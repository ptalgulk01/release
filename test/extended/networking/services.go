package networking

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("networking-services", exutil.KubeConfigPath())
	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-50347-internalTrafficPolicy set Local for pod/node to service access", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create a namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Local",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		g.By("Create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("Create a pod hello-pod2 in second namespace, pod located the same node")
		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod2.name)

		g.By("Create second pod hello-pod3 in second namespace, pod located on the different node")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod3.name)

		g.By("curl from hello-pod2 to service:port")
		CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

		g.By("curl from hello-pod3 to service:port should be failling")
		CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

		g.By("Curl from node0 to service:port")
		//Due to bug 2078691,skip below step for now.
		//CurlNode2SvcPass(oc, pod1.nodename, ns1,"test-service")
		g.By("Curl from node1 to service:port")
		CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		ipStackType := checkIPStackType(oc)

		if ipStackType == "dualstack" {
			g.By("Delete testservice from ns")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Checking pod to svc:port behavior now on with PreferDualStack Service")
			svc.ipFamilyPolicy = "PreferDualStack"
			svc.createServiceFromParams(oc)
			g.By("curl from hello-pod2 to service:port")
			CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

			g.By("curl from hello-pod3 to service:port should be failling")
			CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

			g.By("Curl from node0 to service:port")
			//Due to bug 2078691,skip below step for now.
			//CurlNode2SvcPass(oc, pod1.nodename, ns1,"test-service")
			g.By("Curl from node1 to service:port")
			CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		}
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-50348-internalTrafficPolicy set Local for pod/node to service access with hostnetwork pod backend. [Serial]", func() {
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			hostNetworkPodTemplate = filepath.Join(buildPruningBaseDir, "ping-for-pod-hostnetwork-specific-node-template.yaml")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}
		g.By("Create a namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()
		//Required for hostnetwork pod
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-scc-to-group", "privileged", "system:serviceaccounts:"+ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create 1st hello pod in ns1")

		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns1,
			nodename:  nodeList.Items[0].Name,
			template:  hostNetworkPodTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns1, pod1.name)

		g.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Local",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.ipFamilyPolicy = "SingleStack"
		svc.createServiceFromParams(oc)

		g.By("Create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		g.By("Create a pod hello-pod2 in second namespace, pod located the same node")
		pod2 := pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns2,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod2.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod2.name)

		g.By("Create second pod hello-pod3 in second namespace, pod located on the different node")
		pod3 := pingPodResourceNode{
			name:      "hello-pod3",
			namespace: ns2,
			nodename:  nodeList.Items[1].Name,
			template:  pingPodNodeTemplate,
		}
		pod3.createPingPodNode(oc)
		waitPodReady(oc, ns2, pod3.name)

		g.By("curl from hello-pod2 to service:port")
		CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

		g.By("curl from hello-pod3 to service:port should be failing")
		CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

		g.By("Curl from node1 to service:port")
		CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		ipStackType := checkIPStackType(oc)

		if ipStackType == "dualstack" {
			g.By("Delete testservice from ns")
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "test-service", "-n", ns1).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Checking pod to svc:port behavior now on with PreferDualStack Service")
			svc.ipFamilyPolicy = "PreferDualStack"
			svc.createServiceFromParams(oc)
			g.By("curl from hello-pod2 to service:port")
			CurlPod2SvcPass(oc, ns2, ns1, "hello-pod2", "test-service")

			g.By("curl from hello-pod3 to service:port should be failing")
			CurlPod2SvcFail(oc, ns2, ns1, "hello-pod3", "test-service")

			g.By("Curl from node1 to service:port")
			CurlNode2SvcFail(oc, nodeList.Items[1].Name, ns1, "test-service")

		}
	})

	// author: weliang@redhat.com
	g.It("Author:weliang-Medium-57344-Add support for service session affinity timeout", func() {
		//Bug: https://issues.redhat.com/browse/OCPBUGS-4502
		var (
			buildPruningBaseDir         = exutil.FixturePath("testdata", "networking")
			servicesBaseDir             = exutil.FixturePath("testdata", "networking/services")
			pingPodTemplate             = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			sessionAffinitySvcv4        = filepath.Join(servicesBaseDir, "sessionaffinity-svcv4.yaml")
			sessionAffinitySvcdualstack = filepath.Join(servicesBaseDir, "sessionaffinity-svcdualstack.yaml")
			sessionAffinityPod1         = filepath.Join(servicesBaseDir, "sessionaffinity-pod1.yaml")
			sessionAffinityPod2         = filepath.Join(servicesBaseDir, "sessionaffinity-pod2.yaml")
		)

		ns1 := oc.Namespace()

		g.By("create two pods which will be the endpoints for sessionaffinity service in ns1")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinityPod1, "-n", ns1).Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinityPod2, "-n", ns1).Execute()
		createResourceFromFile(oc, ns1, sessionAffinityPod1)
		waitPodReady(oc, ns1, "blue-pod-1")
		createResourceFromFile(oc, ns1, sessionAffinityPod2)
		waitPodReady(oc, ns1, "blue-pod-2")

		g.By("create a testing pod in ns1")
		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		pod1.createPingPod(oc)
		waitPodReady(oc, ns1, pod1.name)

		ipStackType := checkIPStackType(oc)
		if ipStackType == "ipv4single" {
			g.By("test ipv4 singlestack cluster")
			g.By("create a sessionaffinity service in ns1")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinitySvcv4, "-n", ns1).Execute()
			createsvcerr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sessionAffinitySvcv4, "-n", ns1).Execute()
			o.Expect(createsvcerr).NotTo(o.HaveOccurred())
			svcoutput, svcerr := oc.AsAdmin().Run("get").Args("service", "-n", ns1).Output()
			o.Expect(svcerr).NotTo(o.HaveOccurred())
			o.Expect(svcoutput).To(o.ContainSubstring("sessionaffinitysvcv4"))
			serviceIPv4 := getSvcIPv4(oc, ns1, "sessionaffinitysvcv4")

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			trafficoutput, trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 11; done")
			o.Expect(trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(trafficoutput, "Hello Blue Pod-1") && strings.Contains(trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			trafficoutput1, trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 9; done")
			o.Expect(trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}
		}

		if ipStackType == "dualstack" {
			g.By("test dualstack cluster")
			g.By("create a sessionaffinity service in ns1")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", sessionAffinitySvcdualstack, "-n", ns1).Execute()
			createsvcerr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", sessionAffinitySvcdualstack, "-n", ns1).Execute()
			o.Expect(createsvcerr).NotTo(o.HaveOccurred())
			svcoutput, svcerr := oc.AsAdmin().Run("get").Args("service", "-n", ns1).Output()
			o.Expect(svcerr).NotTo(o.HaveOccurred())
			o.Expect(svcoutput).To(o.ContainSubstring("sessionaffinitysvcdualstack"))
			serviceIPv4 := getSvcIPv4(oc, ns1, "sessionaffinitysvcdualstack")
			serviceIPv6 := getSvcIPv6(oc, ns1, "sessionaffinitysvcdualstack")

			// Test ipv4 traffic in dualstack cluster
			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			trafficoutput, trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 11; done")
			o.Expect(trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(trafficoutput, "Hello Blue Pod-1") && strings.Contains(trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			trafficoutput1, trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl "+serviceIPv4+":8080; sleep 9; done")
			o.Expect(trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}

			// Tes ipv6 traffic in dualstack cluster
			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will LB after curl sleep more than 10s
			g.By("Traffic will LB to two endpoints with sleep 15s in curl")
			v6trafficoutput, v6trafficerr := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl -g -6 ["+serviceIPv6+"]:8080; sleep 11; done")
			o.Expect(v6trafficerr).NotTo(o.HaveOccurred())
			if strings.Contains(v6trafficoutput, "Hello Blue Pod-1") && strings.Contains(v6trafficoutput, "Hello Blue Pod-2") {
				e2e.Logf("Pass : Traffic LB to two endpoints when curl sleep more than 10s")
			} else {
				e2e.Failf("Fail: Traffic does not LB to two endpoints when curl sleep more than 10s")
			}

			// timeoutSeconds in sessionAffinityConfig is set 10s, traffic will not LB after curl sleep less than 10s
			g.By("Traffic will not LB to two endpoints without sleep 15s in curl")
			v6trafficoutput1, v6trafficerr1 := e2eoutput.RunHostCmd(ns1, pod1.name, "for i in 1 2 3 4 5 6 7 8 9 10; do curl -g -6 ["+serviceIPv6+"]:8080; sleep 9; done")
			o.Expect(v6trafficerr1).NotTo(o.HaveOccurred())
			if (strings.Contains(v6trafficoutput1, "Hello Blue Pod-1") && !strings.Contains(v6trafficoutput1, "Hello Blue Pod-2")) || (strings.Contains(v6trafficoutput1, "Hello Blue Pod-2") && !strings.Contains(v6trafficoutput1, "Hello Blue Pod-1")) {
				e2e.Logf("Pass : Traffic does not LB to two endpoints when curl sleep less than 10s")
			} else {
				e2e.Failf("Fail: Traffic LB to two endpoints when curl sleep less than 10s")
			}
		}
	})
	// author: asood@redhat.com
	g.It("Longduration-NonPreRelease-Author:asood-High-62293-Validate all the constructs are created on logical routers and logical switches for a service type loadbalancer. [Disruptive]", func() {
		// Bug: https://issues.redhat.com/browse/OCPBUGS-5930 (Duplicate bug https://issues.redhat.com/browse/OCPBUGS-7000)
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			svcEndpoints           []svcEndpontDetails
			lsConstruct            string
			lrConstruct            string
		)
		platform := exutil.CheckPlatform(oc)
		//vSphere does not have LB service support yet
		e2e.Logf("platform %s", platform)
		if !(strings.Contains(platform, "gcp") || strings.Contains(platform, "aws") || strings.Contains(platform, "azure")) {
			g.Skip("Skip for non-supported auto scaling machineset platforms!!")
		}
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("OVN constructs would not be on the cluster")
		}
		workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get namespace")
		ns := oc.Namespace()

		exutil.By(fmt.Sprintf("create 1st hello pod in %s", ns))
		pod := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns,
			nodename:  workerNodes[0],
			template:  pingPodNodeTemplate,
		}
		pod.createPingPodNode(oc)
		waitPodReady(oc, ns, pod.name)

		exutil.By("Create a test service which is in front of the above pod")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "LoadBalancer",
			ipFamilyPolicy:        "SingleStack",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "Cluster",
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)
		exutil.By("Create a new machineset to add new nodes")
		exutil.SkipConditionally(oc)
		machinesetName := "machineset-62293"
		ms := exutil.MachineSetDescription{machinesetName, 2}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.WaitForMachinesRunning(oc, 2, machinesetName)
		machineName := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)
		nodeName0 := exutil.GetNodeNameFromMachine(oc, machineName[0])
		nodeName1 := exutil.GetNodeNameFromMachine(oc, machineName[1])
		e2e.Logf("The nodes %s and %s added successfully", nodeName0, nodeName1)

		exutil.By(fmt.Sprintf("create 2nd hello pod in %s on newly created node %s", ns, nodeName0))
		pod = pingPodResourceNode{
			name:      "hello-pod2",
			namespace: ns,
			nodename:  nodeName0,
			template:  pingPodNodeTemplate,
		}
		pod.createPingPodNode(oc)
		waitPodReady(oc, ns, pod.name)

		exutil.By("Get backend pod details of user service")
		allPods, getPodErr := exutil.GetAllPodsWithLabel(oc, ns, "name=hello-pod")
		o.Expect(getPodErr).NotTo(o.HaveOccurred())
		o.Expect(len(allPods)).NotTo(o.BeEquivalentTo(0))
		for _, eachPod := range allPods {
			nodeName, nodeNameErr := exutil.GetPodNodeName(oc, ns, eachPod)
			o.Expect(nodeNameErr).NotTo(o.HaveOccurred())
			podIP := getPodIPv4(oc, ns, eachPod)
			ovnkubeNodePod, ovnKubeNodePodErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeName)
			o.Expect(ovnKubeNodePodErr).NotTo(o.HaveOccurred())
			svcEndpoint := svcEndpontDetails{
				ovnKubeNodePod: ovnkubeNodePod,
				nodeName:       nodeName,
				podIP:          podIP,
			}
			svcEndpoints = append(svcEndpoints, svcEndpoint)
		}

		exutil.By("Get logical route and switch on node for endpoints of both services to validate they exist on both new and old node")
		for _, eachEndpoint := range svcEndpoints {
			lsConstruct = eachEndpoint.getOVNConstruct(oc, "ls-list")
			o.Expect(lsConstruct).NotTo(o.BeEmpty())
			e2e.Logf("Logical Switch %s on node %s", lsConstruct, eachEndpoint.nodeName)
			o.Expect(eachEndpoint.getOVNLBContruct(oc, "ls-lb-list", lsConstruct)).To(o.BeTrue())
			lrConstruct = eachEndpoint.getOVNConstruct(oc, "lr-list")
			o.Expect(lrConstruct).NotTo(o.BeEmpty())
			e2e.Logf("Logical Router %s on node %s", lrConstruct, eachEndpoint.nodeName)
			o.Expect(eachEndpoint.getOVNLBContruct(oc, "lr-lb-list", lrConstruct)).To(o.BeTrue())
		}

		exutil.By("Validate kubernetes service is reachable from all nodes including new nodes")
		allNodes, nodeErr := exutil.GetAllNodes(oc)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		o.Expect(len(allNodes)).NotTo(o.BeEquivalentTo(0))
		for i := 0; i < len(allNodes); i++ {
			output, err := exutil.DebugNodeWithChroot(oc, allNodes[i], "bash", "-c", "curl -s -k https://172.30.0.1/healthz --connect-timeout 5")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, "ok")).To(o.BeTrue())
		}

	})
	// author: asood@redhat.com
	g.It("Longduration-NonPreRelease-Author:asood-High-63156-Verify the nodeport is not allocated to VIP based LoadBalancer service type. [Disruptive]", func() {
		// LoadBalancer service implementation are different on cloud provider and bare metal platform
		// https://issues.redhat.com/browse/OCPBUGS-10874 (aws and azure pending support)
		var (
			testDataDir                 = exutil.FixturePath("testdata", "networking/metallb")
			loadBalancerServiceTemplate = filepath.Join(testDataDir, "loadbalancer-svc-template.yaml")
			serviceLabelKey             = "environ"
			serviceLabelValue           = "Test"
			svc_names                   = [2]string{"hello-world-cluster", "hello-world-local"}
			svc_etp                     = [2]string{"Cluster", "Local"}
			namespaces                  []string
		)
		platform := exutil.CheckPlatform(oc)
		e2e.Logf("platform %s", platform)
		if !(strings.Contains(platform, "gcp")) {
			g.Skip("Skip for non-supported platorms!")
		}
		masterNodes, err := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get first namespace and create another")
		ns := oc.Namespace()
		namespaces = append(namespaces, ns)
		oc.SetupProject()
		ns = oc.Namespace()
		namespaces = append(namespaces, ns)
		var desiredMode string
		origMode := getOVNGatewayMode(oc)
		defer switchOVNGatewayMode(oc, origMode)
		g.By("Validate services in original gateway mode " + origMode)
		for j := 0; j < 2; j++ {
			for i := 0; i < 2; i++ {
				svcName := svc_names[i] + "-" + strconv.Itoa(j)
				g.By("Create a service " + svc_names[i] + " with ExternalTrafficPolicy " + svc_etp[i])
				svc := loadBalancerServiceResource{
					name:                          svcName,
					namespace:                     namespaces[i],
					externaltrafficpolicy:         svc_etp[i],
					labelKey:                      serviceLabelKey,
					labelValue:                    serviceLabelValue,
					allocateLoadBalancerNodePorts: false,
					template:                      loadBalancerServiceTemplate,
				}
				result := createLoadBalancerService(oc, svc, loadBalancerServiceTemplate)
				o.Expect(result).To(o.BeTrue())

				g.By("Check LoadBalancer service status")
				err = checkLoadBalancerSvcStatus(oc, svc.namespace, svc.name)
				o.Expect(err).NotTo(o.HaveOccurred())
				g.By("Get LoadBalancer service IP")
				svcIP := getLoadBalancerSvcIP(oc, svc.namespace, svc.name)
				g.By("Validate service")
				result = validateService(oc, masterNodes[0], svcIP)
				o.Expect(result).To(o.BeTrue())
				g.By("Check nodePort is not assigned to service")
				nodePort := getLoadBalancerSvcNodePort(oc, svc.namespace, svc.name)
				o.Expect(nodePort).To(o.BeEmpty())
			}
			if j == 0 {
				g.By("Change the shared gateway mode to local gateway mode")
				if origMode == "local" {
					desiredMode = "shared"
				} else {
					desiredMode = "local"
				}
				e2e.Logf("Cluster is currently on gateway mode %s", origMode)
				e2e.Logf("Desired mode is %s", desiredMode)

				switchOVNGatewayMode(oc, desiredMode)
				g.By("Validate services in modified gateway mode " + desiredMode)
			}
		}

	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:huirwang-Medium-65796-Recreated service should have correct load_balancer nb entries for same name load_balancer. [Serial]", func() {
		// From customer bug https://issues.redhat.com/browse/OCPBUGS-11716
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)

		exutil.By("Get namespace ")
		ns := oc.Namespace()

		exutil.By("create hello pod in namespace")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, ns, pod1.name)
		podIP := getPodIPv4(oc, ns, pod1.name)

		exutil.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "SingleStack",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		svc.createServiceFromParams(oc)

		exutil.By("Check service status")
		svcOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns, svc.servicename).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).Should(o.ContainSubstring(svc.servicename))

		exutil.By("Get service IP")
		svcIP, _ := getSvcIP(oc, svc.namespace, svc.servicename)

		exutil.By("Check nb loadbalancer entries")
		ovnPod := getOVNKMasterOVNkubeNode(oc)
		o.Expect(ovnPod).ShouldNot(o.BeEmpty())
		e2e.Logf("\n ovnKMasterPod: %v\n", ovnPod)
		lbCmd := fmt.Sprintf("ovn-nbctl find load_balancer name=Service_%s/%s_TCP_cluster", ns, svc.servicename)
		lbOutput, err := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lbCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterVIP := fmt.Sprintf("\"%s:%s\"=\"%s:%s\"", svcIP, "27017", podIP, "8080")
		o.Expect(lbOutput).Should(o.ContainSubstring(clusterVIP))

		exutil.By("Delete svc")
		removeResource(oc, true, true, "service", svc.servicename, "-n", ns)

		exutil.By("Manually add load_balancer entry in nb with same name as previous one.")
		// no need to defer to remove, as this will be overrided by following service recreated.
		lbCmdAdd := fmt.Sprintf("ovn-nbctl lb-add \"Service_%s/%s_TCP_cluster\" %s:%s %s:%s", ns, svc.servicename, svcIP, "27017", podIP, "8080")
		_, err = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lbCmdAdd)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Recreate svc")
		svc.createServiceFromParams(oc)
		svcOutput, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns, svc.servicename).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(svcOutput).Should(o.ContainSubstring(svc.servicename))
		exutil.By("Get service IP again")
		svcIP, _ = getSvcIP(oc, svc.namespace, svc.servicename)

		exutil.By("No error logs")
		podlogs, getLogsErr := oc.AsAdmin().Run("logs").Args(ovnPod, "-n", "openshift-ovn-kubernetes", "-c", "ovnkube-controller", "--since", "90s").Output()
		o.Expect(getLogsErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(podlogs, "failed to ensure service")).ShouldNot(o.BeTrue())

		exutil.By("Check nb load_balancer entries again!")
		lbOutput, err = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lbCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterVIP = fmt.Sprintf("\"%s:%s\"=\"%s:%s\"", svcIP, "27017", podIP, "8080")
		o.Expect(lbOutput).Should(o.ContainSubstring(clusterVIP))

		exutil.By("Validate service")
		CurlPod2SvcPass(oc, ns, ns, pod1.name, svc.servicename)
	})

	// author: asood@redhat.com
	g.It("Author:asood-High-46015-Verify traffic to outside the cluster redirected when OVN is used and NodePort service is configured.", func() {
		// Customer bug https://bugzilla.redhat.com/show_bug.cgi?id=1946696
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodNodeTemplate    = filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
		)
		exutil.By("0. Check the network plugin")
		networkType := exutil.CheckNetworkType(oc)
		o.Expect(networkType).NotTo(o.BeEmpty())
		if networkType != "ovnkubernetes" {
			g.Skip("Unsupported network plugin for this test")
		}
		ipStackType := checkIPStackType(oc)
		o.Expect(ipStackType).NotTo(o.BeEmpty())

		exutil.By("1. Get list of worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("Not enough node available, need at least two nodes for the test, skip the case!!")
		}

		exutil.By("2. Get namespace ")
		ns := oc.Namespace()

		exutil.By("3. Create a hello pod in ns")
		pod := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns,
			nodename:  nodeList.Items[0].Name,
			template:  pingPodNodeTemplate,
		}
		pod.createPingPodNode(oc)
		waitPodReady(oc, pod.namespace, pod.name)

		exutil.By("4. Create a nodePort type service fronting the above pod")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "NodePort",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}

		svc.ipFamilyPolicy = "SingleStack"
		defer removeResource(oc, true, true, "service", svc.servicename, "-n", svc.namespace)
		svc.createServiceFromParams(oc)
		exutil.By("5. Get NodePort at which service listens.")
		nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", ns, svc.servicename, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Validate external traffic to node port is redirected.")
		CurlNodePortPass(oc, nodeList.Items[1].Name, nodeList.Items[0].Name, nodePort)
		curlCmd := fmt.Sprintf("curl -4 -v http://www.google.de:%s --connect-timeout 5", nodePort)
		resp, err := exutil.DebugNodeWithChroot(oc, nodeList.Items[1].Name, "/bin/bash", "-c", curlCmd)
		if (err != nil) || (resp != "") {
			o.Expect(strings.Contains(resp, "Hello OpenShift")).To(o.BeFalse())
		}
	})

})
