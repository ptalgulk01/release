package networking

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"

	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-networkpolicy", exutil.KubeConfigPath())

	// author: zzhao@redhat.com
	g.It("Author:zzhao-Critical-49076-service domain can be resolved when egress type is enabled", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloSdnFile        = filepath.Join(buildPruningBaseDir, "hellosdn.yaml")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/egress-allow-all.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/ingress-allow-all.yaml")
		)
		g.By("create new namespace")
		oc.SetupProject()

		g.By("create test pods")
		createResourceFromFile(oc, oc.Namespace(), testPodFile)
		createResourceFromFile(oc, oc.Namespace(), helloSdnFile)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=hellosdn")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=hellosdn not ready")

		g.By("create egress and ingress type networkpolicy")
		createResourceFromFile(oc, oc.Namespace(), egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-all-egress"))
		createResourceFromFile(oc, oc.Namespace(), ingressTypeFile)
		output, err = oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-all-ingress"))

		g.By("check hellosdn pods can reolsve the dns after apply the networkplicy")
		helloSdnName := getPodName(oc, oc.Namespace(), "name=hellosdn")
		digOutput, err := e2e.RunHostCmd(oc.Namespace(), helloSdnName[0], "dig kubernetes.default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(digOutput).Should(o.ContainSubstring("Got answer"))
		o.Expect(digOutput).ShouldNot(o.ContainSubstring("connection timed out"))

		g.By("check test-pods can reolsve the dns after apply the networkplicy")
		testPodName := getPodName(oc, oc.Namespace(), "name=test-pods")
		digOutput, err = e2e.RunHostCmd(oc.Namespace(), testPodName[0], "dig kubernetes.default")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(digOutput).Should(o.ContainSubstring("Got answer"))
		o.Expect(digOutput).ShouldNot(o.ContainSubstring("connection timed out"))

	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-Critical-49186-[Bug 2035336] Networkpolicy egress rule should work for statefulset pods.", func() {
		var (
			buildPruningBaseDir  = exutil.FixturePath("testdata", "networking")
			testPodFile          = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloStatefulsetFile = filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
			egressTypeFile       = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-egress-red.yaml")
		)
		g.By("1. Create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("2. Create a statefulset pod in first namespace.")
		createResourceFromFile(oc, ns1, helloStatefulsetFile)
		err := waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(err, "this pod with label app=hello not ready")
		helloPodName := getPodName(oc, ns1, "app=hello")

		g.By("3. Create networkpolicy with egress rule in first namespace.")
		createResourceFromFile(oc, ns1, egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("allow-egress-to-red"))

		g.By("4. Create second namespace.")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("5. Create test pods in second namespace.")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("6. Add label to first test pod in second namespace.")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "team=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		testPodName := getPodName(oc, ns2, "name=test-pods")
		err = exutil.LabelPod(oc, ns2, testPodName[0], "type=red")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("6. Get IP of the test pods in second namespace.")
		testPodIP1 := getPodIPv4(oc, ns2, testPodName[0])
		testPodIP2 := getPodIPv4(oc, ns2, testPodName[1])

		g.By("7. Check networkpolicy works.")
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		_, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5  -s "+net.JoinHostPort(testPodIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

		g.By("8. Delete statefulset pod for a couple of times.")
		for i := 0; i < 5; i++ {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", helloPodName[0], "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err := waitForPodWithLabelReady(oc, ns1, "app=hello")
			exutil.AssertWaitPollNoErr(err, "this pod with label app=hello not ready")
		}

		g.By("9. Again checking networkpolicy works.")
		output, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		_, err = e2e.RunHostCmd(ns1, helloPodName[0], "curl --connect-timeout 5 -s "+net.JoinHostPort(testPodIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-49437-[BZ 2037647] Ingress network policy shouldn't be overruled by egress network policy on another pod", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/default-allow-egress.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)
		g.By("Create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("create a hello pod in first namespace")
		podns1 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		podns1.createPingPodNode(oc)
		waitPodReady(oc, podns1.namespace, podns1.name)

		g.By("create default allow egress type networkpolicy in first namespace")
		createResourceFromFile(oc, ns1, egressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-allow-egress"))

		g.By("Create Second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		g.By("create a hello-pod on 2nd namesapce on same node as first namespace")
		pod1Ns2 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1Ns2.createPingPodNode(oc)
		waitPodReady(oc, pod1Ns2.namespace, pod1Ns2.name)

		g.By("create another hello-pod on 2nd namesapce but on different node")
		pod2Ns2 := pingPodResourceNode{
			name:      "hello-pod-other-node",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod2Ns2.createPingPodNode(oc)
		waitPodReady(oc, pod2Ns2.namespace, pod2Ns2.name)

		helloPodNameNs2 := getPodName(oc, ns2, "name=hello-pod")

		g.By("create default deny ingress type networkpolicy in 2nd namespace")
		createResourceFromFile(oc, ns2, ingressTypeFile)
		output, err = oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-ingress"))

		g.By("3. Get IP of the test pods in second namespace.")
		hellopodIP1Ns2 := getPodIPv4(oc, ns2, helloPodNameNs2[0])
		hellopodIP2Ns2 := getPodIPv4(oc, ns2, helloPodNameNs2[1])

		g.By("4. Curl both ns2 pods from ns1.")
		_, err = e2e.RunHostCmd(ns1, podns1.name, "curl --connect-timeout 5  -s "+net.JoinHostPort(hellopodIP1Ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))
		_, err = e2e.RunHostCmd(ns1, podns1.name, "curl --connect-timeout 5  -s "+net.JoinHostPort(hellopodIP2Ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))
	})

	// author: anusaxen@redhat.com
	g.It("NonHyperShiftHOST-Author:anusaxen-Medium-49686-network policy with ingress rule with ipBlock", func() {
		var (
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			ipBlockIngressTemplateDual   = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-ingress-dual-CIDRs-template.yaml")
			ipBlockIngressTemplateSingle = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-ingress-single-CIDR-template.yaml")
			pingPodNodeTemplate          = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.Skip("This case requires dualstack or Single Stack Ipv6 cluster")
		}

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("create 1st hello pod in ns1")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("create 2nd hello pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		g.By("create 3rd hello pod in ns1")
		pod3ns1 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3ns1.createPingPodNode(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

		helloPod1ns1IPv6, helloPod1ns1IPv4 := getPodIP(oc, ns1, pod1ns1.name)
		helloPod1ns1IPv4WithCidr := helloPod1ns1IPv4 + "/32"
		helloPod1ns1IPv6WithCidr := helloPod1ns1IPv6 + "/128"

		if ipStackType == "dualstack" {
			g.By("create ipBlock Ingress Dual CIDRs Policy in ns1")
			npIPBlockNS1 := ipBlockIngressDual{
				name:      "ipblock-dual-cidrs-ingress",
				template:  ipBlockIngressTemplateDual,
				cidrIpv4:  helloPod1ns1IPv4WithCidr,
				cidrIpv6:  helloPod1ns1IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1.createipBlockIngressObjectDual(oc)

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-dual-cidrs-ingress"))
		} else {
			npIPBlockNS1 := ipBlockIngressSingle{
				name:      "ipblock-single-cidr-ingress",
				template:  ipBlockIngressTemplateSingle,
				cidr:      helloPod1ns1IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1.createipBlockIngressObjectSingle(oc)

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-single-cidr-ingress"))
		}
		g.By("Checking connectivity from pod1 to pod3")
		CurlPod2PodPass(oc, ns1, "hello-pod1", ns1, "hello-pod3")

		g.By("Checking connectivity from pod2 to pod3")
		CurlPod2PodFail(oc, ns1, "hello-pod2", ns1, "hello-pod3")

		g.By("Create 2nd namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("create 1st hello pod in ns2")
		pod1ns2 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns2.createPingPodNode(oc)
		waitPodReady(oc, pod1ns2.namespace, pod1ns2.name)

		g.By("create 2nd hello pod in ns2")
		pod2ns2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2ns2.createPingPodNode(oc)
		waitPodReady(oc, pod2ns2.namespace, pod2ns2.name)

		g.By("Checking connectivity from pod1ns2 to pod3ns1")
		CurlPod2PodFail(oc, ns2, "hello-pod1", ns1, "hello-pod3")

		g.By("Checking connectivity from pod2ns2 to pod1ns1")
		CurlPod2PodFail(oc, ns2, "hello-pod2", ns1, "hello-pod1")

		if ipStackType == "dualstack" {
			g.By("Delete networkpolicy from ns1")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-dual-cidrs-ingress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			g.By("Delete networkpolicy from ns1")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-single-cidr-ingress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		helloPod2ns2IPv6, helloPod2ns2IPv4 := getPodIP(oc, ns2, pod2ns2.name)
		helloPod2ns2IPv4WithCidr := helloPod2ns2IPv4 + "/32"
		helloPod2ns2IPv6WithCidr := helloPod2ns2IPv6 + "/128"

		if ipStackType == "dualstack" {
			g.By("create ipBlock Ingress Dual CIDRs Policy in ns1 again but with ipblock for pod2 ns2")
			npIPBlockNS1New := ipBlockIngressDual{
				name:      "ipblock-dual-cidrs-ingress",
				template:  ipBlockIngressTemplateDual,
				cidrIpv4:  helloPod2ns2IPv4WithCidr,
				cidrIpv6:  helloPod2ns2IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1New.createipBlockIngressObjectDual(oc)
		} else {
			npIPBlockNS1New := ipBlockIngressSingle{
				name:      "ipblock-single-cidr-ingress",
				template:  ipBlockIngressTemplateSingle,
				cidr:      helloPod2ns2IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1New.createipBlockIngressObjectSingle(oc)
		}
		g.By("Checking connectivity from pod2 ns2 to pod3 ns1")
		CurlPod2PodPass(oc, ns2, "hello-pod2", ns1, "hello-pod3")

		g.By("Checking connectivity from pod1 ns2 to pod3 ns1")
		CurlPod2PodFail(oc, ns2, "hello-pod1", ns1, "hello-pod3")

		if ipStackType == "dualstack" {
			g.By("Delete networkpolicy from ns1 again so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-dual-cidrs-ingress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			g.By("Delete networkpolicy from ns1 again so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-single-cidr-ingress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Check connectivity works fine across all failed ones above to make sure all policy flows are cleared properly")

		g.By("Checking connectivity from pod2ns1 to pod3ns1")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod3")

		g.By("Checking connectivity from pod1ns2 to pod3ns1")
		CurlPod2PodPass(oc, ns2, "hello-pod1", ns1, "hello-pod3")

		g.By("Checking connectivity from pod2ns2 to pod1ns1 on IPv4 interface")
		CurlPod2PodPass(oc, ns2, "hello-pod2", ns1, "hello-pod1")

	})

	// author: zzhao@redhat.com
	g.It("Author:zzhao-Critical-49696-mixed ingress and egress policies can work well", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			helloSdnFile        = filepath.Join(buildPruningBaseDir, "hellosdn.yaml")
			egressTypeFile      = filepath.Join(buildPruningBaseDir, "networkpolicy/egress_49696.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/ingress_49696.yaml")
		)
		g.By("create one namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("create test pods")
		createResourceFromFile(oc, ns1, testPodFile)
		createResourceFromFile(oc, ns1, helloSdnFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		err = waitForPodWithLabelReady(oc, ns1, "name=hellosdn")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=hellosdn not ready")
		hellosdnPodNameNs1 := getPodName(oc, ns1, "name=hellosdn")

		g.By("create egress type networkpolicy in ns1")
		createResourceFromFile(oc, ns1, egressTypeFile)

		g.By("create ingress type networkpolicy in ns1")
		createResourceFromFile(oc, ns1, ingressTypeFile)

		g.By("create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("create test pods in second namespace")
		createResourceFromFile(oc, ns2, helloSdnFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=hellosdn")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=hellosdn not ready")

		g.By("Get IP of the test pods in second namespace.")
		hellosdnPodNameNs2 := getPodName(oc, ns2, "name=hellosdn")
		hellosdnPodIP1Ns2 := getPodIPv4(oc, ns2, hellosdnPodNameNs2[0])

		g.By("curl from ns1 hellosdn pod to ns2 pod")
		_, err = e2e.RunHostCmd(ns1, hellosdnPodNameNs1[0], "curl --connect-timeout 5  -s "+net.JoinHostPort(hellosdnPodIP1Ns2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(err.Error()).Should(o.ContainSubstring("exit status 28"))

	})

	// author: anusaxen@redhat.com
	g.It("Author:anusaxen-High-46246-Network Policies should work with OVNKubernetes when traffic hairpins back to the same source through a service", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			allowfromsameNS        = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-from-same-namespace.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create a namespace")
		oc.SetupProject()
		ns := oc.Namespace()

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns, pod1.name)

		g.By("create 2nd hello pod in same namespace but on different node")

		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns, pod2.name)

		g.By("Create a test service backing up both the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		g.By("create allow-from-same-namespace ingress networkpolicy in ns")
		createResourceFromFile(oc, ns, allowfromsameNS)

		g.By("curl from hello-pod1 to hello-pod2")
		CurlPod2PodPass(oc, ns, "hello-pod1", ns, "hello-pod2")

		g.By("curl from hello-pod2 to hello-pod1")
		CurlPod2PodPass(oc, ns, "hello-pod2", ns, "hello-pod1")

		for i := 0; i < 5; i++ {

			g.By("curl from hello-pod1 to service:port")
			CurlPod2SvcPass(oc, ns, ns, "hello-pod1", "test-service")

			g.By("curl from hello-pod2 to service:port")
			CurlPod2SvcPass(oc, ns, ns, "hello-pod2", "test-service")
		}

		g.By("Make sure pods are curl'able from respective nodes")
		CurlNode2PodPass(oc, pod1.nodename, ns, "hello-pod1")
		CurlNode2PodPass(oc, pod2.nodename, ns, "hello-pod2")

		ipStackType := checkIPStackType(oc)

		if ipStackType == "dualstack" {
			g.By("Delete testservice from ns")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Checking pod to svc:port behavior now on with PreferDualStack Service")
			svc.ipFamilyPolicy = "PreferDualStack"
			svc.createServiceFromParams(oc)
			for i := 0; i < 5; i++ {
				g.By("curl from hello-pod1 to service:port")
				CurlPod2SvcPass(oc, ns, ns, "hello-pod1", "test-service")

				g.By("curl from hello-pod2 to service:port")
				CurlPod2SvcPass(oc, ns, ns, "hello-pod2", "test-service")
			}
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-High-41879-ipBlock should not ignore all other cidr's apart from the last one specified	", func() {
		var (
			buildPruningBaseDir          = exutil.FixturePath("testdata", "networking")
			ipBlockIngressTemplateDual   = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-ingress-dual-multiple-CIDRs-template.yaml")
			ipBlockIngressTemplateSingle = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-ingress-single-multiple-CIDRs-template.yaml")
			testPodFile                  = filepath.Join(buildPruningBaseDir, "testpod.yaml")
		)

		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.Skip("This case requires dualstack or Single Stack IPv6 cluster")
		}

		g.By("Create a namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("create test pods in ns1")
		createResourceFromFile(oc, ns1, testPodFile)
		err := waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("Scale test pods to 5")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=5", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("Get 3 test pods's podname and IPs")
		testPodName := getPodName(oc, ns1, "name=test-pods")
		testPod1IPv6, testPod1IPv4 := getPodIP(oc, ns1, testPodName[0])
		testPod1IPv4WithCidr := testPod1IPv4 + "/32"
		testPod1IPv6WithCidr := testPod1IPv6 + "/128"
		testPod2IPv6, testPod2IPv4 := getPodIP(oc, ns1, testPodName[1])
		testPod2IPv4WithCidr := testPod2IPv4 + "/32"
		testPod2IPv6WithCidr := testPod2IPv6 + "/128"
		testPod3IPv6, testPod3IPv4 := getPodIP(oc, ns1, testPodName[2])
		testPod3IPv4WithCidr := testPod3IPv4 + "/32"
		testPod3IPv6WithCidr := testPod3IPv6 + "/128"

		if ipStackType == "dualstack" {
			g.By("create ipBlock Ingress Dual CIDRs Policy in ns1")
			npIPBlockNS1 := ipBlockIngressDual{
				name:      "ipblock-dual-cidrs-ingress-41879",
				template:  ipBlockIngressTemplateDual,
				cidrIpv4:  testPod1IPv4WithCidr,
				cidrIpv6:  testPod1IPv6WithCidr,
				cidr2Ipv4: testPod2IPv4WithCidr,
				cidr2Ipv6: testPod2IPv6WithCidr,
				cidr3Ipv4: testPod3IPv4WithCidr,
				cidr3Ipv6: testPod3IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1.createipBlockMultipleCidrIngressObjectDual(oc)

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-dual-cidrs-ingress-41879"))
		} else {
			npIPBlockNS1 := ipBlockIngressSingle{
				name:      "ipblock-single-cidr-ingress-41879",
				template:  ipBlockIngressTemplateSingle,
				cidr:      testPod1IPv6WithCidr,
				cidr2:     testPod2IPv6WithCidr,
				cidr3:     testPod3IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1.createipBlockMultipleCidrIngressObjectSingle(oc)

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-single-cidr-ingress-41879"))
		}

		g.By("Checking connectivity from pod1 to pod5")
		CurlPod2PodPass(oc, ns1, testPodName[0], ns1, testPodName[4])

		g.By("Checking connectivity from pod2 to pod5")
		CurlPod2PodPass(oc, ns1, testPodName[1], ns1, testPodName[4])

		g.By("Checking connectivity from pod3 to pod5")
		CurlPod2PodPass(oc, ns1, testPodName[2], ns1, testPodName[4])

		g.By("Checking connectivity from pod4 to pod5")
		CurlPod2PodFail(oc, ns1, testPodName[3], ns1, testPodName[4])

	})

	// author: asood@redhat.com
	g.It("Author:asood-Medium-46807-network policy with egress rule with ipBlock", func() {
		var (
			buildPruningBaseDir         = exutil.FixturePath("testdata", "networking")
			ipBlockEgressTemplateDual   = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-egress-dual-CIDRs-template.yaml")
			ipBlockEgressTemplateSingle = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-egress-single-CIDR-template.yaml")
			pingPodNodeTemplate         = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		ipStackType := checkIPStackType(oc)
		o.Expect(ipStackType).NotTo(o.BeEmpty())

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Obtain the namespace")
		ns1 := oc.Namespace()

		g.By("create 1st hello pod in ns1")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("create 2nd hello pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		g.By("create 3rd hello pod in ns1")
		pod3ns1 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3ns1.createPingPodNode(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

		helloPod1ns1IP1, helloPod1ns1IP2 := getPodIP(oc, ns1, pod1ns1.name)

		if ipStackType == "dualstack" {
			helloPod1ns1IPv6WithCidr := helloPod1ns1IP1 + "/128"
			helloPod1ns1IPv4WithCidr := helloPod1ns1IP2 + "/32"
			g.By("create ipBlock Egress Dual CIDRs Policy in ns1")
			npIPBlockNS1 := ipBlockEgressDual{
				name:      "ipblock-dual-cidrs-egress",
				template:  ipBlockEgressTemplateDual,
				cidrIpv4:  helloPod1ns1IPv4WithCidr,
				cidrIpv6:  helloPod1ns1IPv6WithCidr,
				namespace: ns1,
			}
			npIPBlockNS1.createipBlockEgressObjectDual(oc, false)

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-dual-cidrs-egress"))

		} else {
			if ipStackType == "ipv6single" {
				helloPod1ns1IPv6WithCidr := helloPod1ns1IP1 + "/128"
				npIPBlockNS1 := ipBlockEgressSingle{
					name:      "ipblock-single-cidr-egress",
					template:  ipBlockEgressTemplateSingle,
					cidr:      helloPod1ns1IPv6WithCidr,
					namespace: ns1,
				}
				npIPBlockNS1.createipBlockEgressObjectSingle(oc, false)
			} else {
				helloPod1ns1IPv4WithCidr := helloPod1ns1IP1 + "/32"
				npIPBlockNS1 := ipBlockEgressSingle{
					name:      "ipblock-single-cidr-egress",
					template:  ipBlockEgressTemplateSingle,
					cidr:      helloPod1ns1IPv4WithCidr,
					namespace: ns1,
				}
				npIPBlockNS1.createipBlockEgressObjectSingle(oc, false)
			}

			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-single-cidr-egress"))
		}
		g.By("Checking connectivity from pod2 to pod1")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod1")

		g.By("Checking connectivity from pod2 to pod3")
		CurlPod2PodFail(oc, ns1, "hello-pod2", ns1, "hello-pod3")

		if ipStackType == "dualstack" {
			g.By("Delete networkpolicy from ns1 so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-dual-cidrs-egress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			g.By("Delete networkpolicy from ns1 so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-single-cidr-egress", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Check connectivity works fine across all failed ones above to make sure all policy flows are cleared properly")

		g.By("Checking connectivity from pod2 to pod1")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod1")

		g.By("Checking connectivity from pod2 to pod3")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod3")

	})

	// author: asood@redhat.com
	g.It("Author:asood-Medium-46808-network policy with egress rule with ipBlock and except", func() {
		var (
			buildPruningBaseDir         = exutil.FixturePath("testdata", "networking")
			ipBlockEgressTemplateDual   = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-egress-except-dual-CIDRs-template.yaml")
			ipBlockEgressTemplateSingle = filepath.Join(buildPruningBaseDir, "networkpolicy/ipblock/ipBlock-egress-except-single-CIDR-template.yaml")
			pingPodNodeTemplate         = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		ipStackType := checkIPStackType(oc)
		o.Expect(ipStackType).NotTo(o.BeEmpty())

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Obtain the namespace")
		ns1 := oc.Namespace()

		g.By("create 1st hello pod in ns1 on node[0]")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("create 2nd hello pod in ns1 on node[0]")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		g.By("create 3rd hello pod in ns1 on node[1]")
		pod3ns1 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3ns1.createPingPodNode(oc)
		waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

		g.By("create 4th hello pod in ns1 on node[1]")
		pod4ns1 := pingPodResourceNode{
			name:      "hello-pod4",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod4ns1.createPingPodNode(oc)
		waitPodReady(oc, pod4ns1.namespace, pod4ns1.name)

		helloPod2ns1IP1, helloPod2ns1IP2 := getPodIP(oc, ns1, pod2ns1.name)
		if ipStackType == "dualstack" {
			hostSubnetCIDRIPv4, hostSubnetCIDRIPv6 := getNodeSubnetDualStack(oc, nodeList.Items[0].Name)
			o.Expect(hostSubnetCIDRIPv6).NotTo(o.BeEmpty())
			o.Expect(hostSubnetCIDRIPv4).NotTo(o.BeEmpty())
			helloPod2ns1IPv6WithCidr := helloPod2ns1IP1 + "/128"
			helloPod2ns1IPv4WithCidr := helloPod2ns1IP2 + "/32"
			g.By("create ipBlock Egress CIDRs with except rule Policy in ns1 on dualstack")
			npIPBlockNS1 := ipBlockEgressDual{
				name:           "ipblock-dual-cidrs-egress-except",
				template:       ipBlockEgressTemplateDual,
				cidrIpv4:       hostSubnetCIDRIPv4,
				cidrIpv4Except: helloPod2ns1IPv4WithCidr,
				cidrIpv6:       hostSubnetCIDRIPv6,
				cidrIpv6Except: helloPod2ns1IPv6WithCidr,
				namespace:      ns1,
			}
			npIPBlockNS1.createipBlockEgressObjectDual(oc, true)
			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-dual-cidrs-egress-except"))
		} else {
			if ipStackType == "ipv6single" {
				hostSubnetCIDRIPv6 := getNodeSubnet(oc, nodeList.Items[0].Name)
				o.Expect(hostSubnetCIDRIPv6).NotTo(o.BeEmpty())
				helloPod2ns1IPv6WithCidr := helloPod2ns1IP1 + "/128"
				g.By("create ipBlock Egress CIDRs with except rule Policy in ns1 on IPv6 singlestack")
				npIPBlockNS1 := ipBlockEgressSingle{
					name:      "ipblock-single-cidr-egress-except",
					template:  ipBlockEgressTemplateSingle,
					cidr:      hostSubnetCIDRIPv6,
					except:    helloPod2ns1IPv6WithCidr,
					namespace: ns1,
				}
				npIPBlockNS1.createipBlockEgressObjectSingle(oc, true)
			} else {
				hostSubnetCIDRIPv4 := getNodeSubnet(oc, nodeList.Items[0].Name)
				o.Expect(hostSubnetCIDRIPv4).NotTo(o.BeEmpty())
				helloPod2ns1IPv4WithCidr := helloPod2ns1IP1 + "/32"
				g.By("create ipBlock Egress CIDRs with except rule Policy in ns1 on IPv4 singlestack")
				npIPBlockNS1 := ipBlockEgressSingle{
					name:      "ipblock-single-cidr-egress-except",
					template:  ipBlockEgressTemplateSingle,
					cidr:      hostSubnetCIDRIPv4,
					except:    helloPod2ns1IPv4WithCidr,
					namespace: ns1,
				}
				npIPBlockNS1.createipBlockEgressObjectSingle(oc, true)
			}
			output, err := oc.Run("get").Args("networkpolicy").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("ipblock-single-cidr-egress-except"))
		}
		g.By("Checking connectivity from pod3 to pod1")
		CurlPod2PodPass(oc, ns1, "hello-pod3", ns1, "hello-pod1")

		g.By("Checking connectivity from pod3 to pod2")
		CurlPod2PodFail(oc, ns1, "hello-pod3", ns1, "hello-pod2")

		g.By("Checking connectivity from pod3 to pod4")
		CurlPod2PodFail(oc, ns1, "hello-pod3", ns1, "hello-pod4")
		if ipStackType == "dualstack" {
			g.By("Delete networkpolicy from ns1 so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-dual-cidrs-egress-except", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			g.By("Delete networkpolicy from ns1 so no networkpolicy in namespace")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("networkpolicy", "ipblock-single-cidr-egress-except", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Check connectivity works fine across all failed ones above to make sure all policy flows are cleared properly")

		g.By("Checking connectivity from pod3 to pod1")
		CurlPod2PodPass(oc, ns1, "hello-pod3", ns1, "hello-pod1")

		g.By("Checking connectivity from pod3 to pod2")
		CurlPod2PodPass(oc, ns1, "hello-pod3", ns1, "hello-pod2")

		g.By("Checking connectivity from pod3 to pod4")
		CurlPod2PodPass(oc, ns1, "hello-pod3", ns1, "hello-pod4")

	})

	// author: asood@redhat.com
	g.It("Author:asood-Medium-41082-Check ACL audit logs can be extracted", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			allowFromSameNS     = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-from-same-namespace.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("Network policy ACL auditing enabled on OVN network plugin")
		}

		g.By("Obtain the namespace")
		ns1 := oc.Namespace()

		g.By("Enable ACL looging on the namespace ns1")
		aclSettings := aclSettings{DenySetting: "alert", AllowSetting: "alert"}
		err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", ns1, aclSettings.getJSONString()).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())

		g.By("create default deny ingress networkpolicy in ns1")
		createResourceFromFile(oc, ns1, ingressTypeFile)

		g.By("create allow same namespace networkpolicy in ns1")
		createResourceFromFile(oc, ns1, allowFromSameNS)

		g.By("create 1st hello pod in ns1")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("create 2nd hello pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}

		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		g.By("Checking connectivity from pod2 to pod1 to generate messages")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod1")

		output, err2 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "verdict=allow")).To(o.BeTrue())

	})
	// author: asood@redhat.com
	g.It("Author:asood-Medium-41407-Check networkpolicy ACL audit message is logged with correct policy name", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			allowFromSameNS     = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-from-same-namespace.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("Network policy ACL auditing enabled on OVN network plugin")
		}

		var namespaces [2]string
		for i := 0; i < 2; i++ {

			g.By("Obtain and create the namespace")
			oc.SetupProject()
			ns := oc.Namespace()
			namespaces[i] = ns

			g.By(fmt.Sprintf("Enable ACL looging on the namespace %s", namespaces[i]))
			aclSettings := aclSettings{DenySetting: "alert", AllowSetting: "alert"}
			err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", namespaces[i], aclSettings.getJSONString()).Execute()
			o.Expect(err1).NotTo(o.HaveOccurred())

			g.By(fmt.Sprintf("create default deny ingress networkpolicy in %s", namespaces[i]))
			createResourceFromFile(oc, namespaces[i], ingressTypeFile)

			g.By(fmt.Sprintf("create allow same namespace networkpolicy in %s", namespaces[i]))
			createResourceFromFile(oc, namespaces[i], allowFromSameNS)

			g.By(fmt.Sprintf("create 1st hello pod in %s", namespaces[i]))
			pod1ns := pingPodResourceNode{
				name:      "hello-pod1",
				namespace: namespaces[i],
				nodename:  nodeList.Items[0].Name,
				template:  pingPodNodeTemplate,
			}
			pod1ns.createPingPodNode(oc)
			waitPodReady(oc, pod1ns.namespace, pod1ns.name)

			g.By(fmt.Sprintf("create 2nd hello pod in %s", namespaces[i]))
			pod2ns := pingPodResourceNode{
				name:      "hello-pod2",
				namespace: namespaces[i],
				nodename:  nodeList.Items[1].Name,
				template:  pingPodNodeTemplate,
			}
			pod2ns.createPingPodNode(oc)
			waitPodReady(oc, pod2ns.namespace, pod2ns.name)

			g.By(fmt.Sprintf("Checking connectivity from pod2 to pod1 to generate messages in %s", namespaces[i]))
			CurlPod2PodPass(oc, namespaces[i], "hello-pod2", namespaces[i], "hello-pod1")
		}

		output, err3 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", nodeList.Items[0].Name, "--path=ovn/acl-audit-log.log").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		for i := 0; i < len(namespaces); i++ {
			o.Expect(strings.Contains(output, "verdict=allow")).To(o.BeTrue())
			o.Expect(strings.Contains(output, namespaces[i])).To(o.BeTrue())

		}

	})
	// author: asood@redhat.com
	g.It("Author:asood-Medium-41080-Check network policy ACL audit messages are logged to journald", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			allowFromSameNS     = filepath.Join(buildPruningBaseDir, "networkpolicy/allow-from-same-namespace.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
			pingPodNodeTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("Network policy ACL auditing enabled on OVN network plugin")
		}

		g.By("Configure audit message logging destination to journald")
		patchSResource := "networks.operator.openshift.io/cluster"
		patchInfo := `{"spec":{"defaultNetwork":{"ovnKubernetesConfig":{"policyAuditConfig": {"destination": "libc"}}}}}`
		undoPatchInfo := `{"spec":{"defaultNetwork":{"ovnKubernetesConfig":{"policyAuditConfig": {"destination": ""}}}}}`
		defer oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", undoPatchInfo, "--type=merge").Output()
		_, patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchSResource, "-p", patchInfo, "--type=merge").Output()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		//Network operator needs to recreate the pods on a merge request, therefore give it enough time.
		checkNetworkOperatorState(oc, 400, 400)

		g.By("Obtain the namespace")
		ns1 := oc.Namespace()

		g.By("Enable ACL looging on the namespace ns1")
		aclSettings := aclSettings{DenySetting: "alert", AllowSetting: "alert"}
		err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", ns1, aclSettings.getJSONString()).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())

		g.By("create default deny ingress networkpolicy in ns1")
		createResourceFromFile(oc, ns1, ingressTypeFile)

		g.By("create allow same namespace networkpolicy in ns1")
		createResourceFromFile(oc, ns1, allowFromSameNS)

		g.By("create 1st hello pod in ns1")
		pod1ns1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1ns1.createPingPodNode(oc)
		waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

		g.By("create 2nd hello pod in ns1")
		pod2ns1 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns1,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}

		pod2ns1.createPingPodNode(oc)
		waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

		g.By("Checking connectivity from pod2 to pod1 to generate messages")
		CurlPod2PodPass(oc, ns1, "hello-pod2", ns1, "hello-pod1")

		g.By("Checking messages are logged to journald")
		cmd := fmt.Sprintf("journalctl -t ovn-controller --since '1min ago'| grep 'verdict=allow'")
		output, journalctlErr := exutil.DebugNodeWithOptionsAndChroot(oc, nodeList.Items[0].Name, []string{"-q"}, "bin/sh", "-c", cmd)
		e2e.Logf("Output %s", output)
		o.Expect(journalctlErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "verdict=allow")).To(o.BeTrue())

	})

	// author: anusaxen@redhat.com
	g.It("NonHyperShiftHOST-Author:anusaxen-Medium-55287-Default network policy ACLs to a namespace should not be present with arp but arp||nd for ARPAllowPolicies", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "networking")
			testPodFile         = filepath.Join(buildPruningBaseDir, "testpod.yaml")
			ingressTypeFile     = filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
		)
		g.By("This is for BZ 2095852")
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("This case requires OVNKubernetes as network backend")
		}
		g.By("create new namespace")
		oc.SetupProject()

		g.By("create test pods")
		createResourceFromFile(oc, oc.Namespace(), testPodFile)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		g.By("create ingress default-deny type networkpolicy")
		createResourceFromFile(oc, oc.Namespace(), ingressTypeFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny"))

		ovnMasterPodName := getOVNLeaderPod(oc, "north")
		g.By("get ACLs related to ns")
		//list ACLs only related namespace in test
		listACLCmd := "ovn-nbctl list ACL | grep -C 5 " + oc.Namespace() + "_ARPallowPolicy"
		listOutput, listErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnMasterPodName, listACLCmd)
		o.Expect(listErr).NotTo(o.HaveOccurred())
		e2e.Logf("Output %s", listOutput)
		o.Expect(listOutput).To(o.ContainSubstring("&& (arp || nd)"))
		o.Expect(listOutput).ShouldNot(o.ContainSubstring("&& arp"))
	})

})
