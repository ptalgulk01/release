package networking

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/vmware/govmomi"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var (
		ipEchoURL       string
		a               *exutil.AwsClient
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		flag            string
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "none") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS/GCP/Azure/Openstack/Vsphere/BareMetal cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}

		if !strings.Contains(platform, "none") && checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if ipEchoURL == "" {
				creErr := getAwsCredentialFromCluster(oc)
				if creErr != nil {
					e2e.Logf("Cannot get AWS credential, will use tcpdump tool to verify egressIP,%v", creErr)
					flag = "tcpdump"
				} else {
					a = exutil.InitAwsSession()
					_, err := getAwsIntSvcInstanceID(a, oc)
					if err != nil {
						flag = "tcpdump"
						e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
					} else {
						ipEchoURL, err = installIPEchoServiceOnAWS(a, oc)
						if ipEchoURL != "" && err == nil {
							flag = "ipecho"
							e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
						} else {
							flag = "tcpdump"
							e2e.Logf("No ip-echo service installed on the bastion host, change to use tcpdump way %v", err)
						}
					}
				}
			}
		case "gcp":
			e2e.Logf("\n GCP is detected, running the case on GCP\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, use tcpdump to verify egressIP
				infraID, err := exutil.GetInfraID(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIPFromGcp(oc, infraID)
				if host == "" || err != nil {
					flag = "tcpdump"
					e2e.Logf("There is no int svc instance in this cluster: %v, try tcpdump way", err)
				} else {
					ipEchoURL, err = installIPEchoServiceOnGCP(oc, infraID, host)
					if ipEchoURL != "" && err == nil {
						flag = "ipecho"
						e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
					} else {
						e2e.Logf("No ip-echo service installed on the bastion host, %v, change to use tcpdump to verify", err)
						flag = "tcpdump"
					}
				}
			}
		case "azure":
			e2e.Logf("\n Azure is detected, running the case on Azure\n")
			if ipEchoURL == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, use tcpdump to verify egressIP
				creErr := getAzureCredentialFromCluster(oc)
				if creErr != nil {
					e2e.Logf("Cannot get azure credential, will use tcpdump tool to verify egressIP,%v", creErr)
					flag = "tcpdump"
				} else {
					rg, azGroupErr := getAzureResourceGroup(oc)
					if azGroupErr != nil {
						e2e.Logf("Cannot get azure resource group, will use tcpdump tool to verify egressIP,%v", azGroupErr)
						flag = "tcpdump"
					} else {
						az, err := exutil.NewAzureSessionFromEnv()
						if err != nil {
							e2e.Logf("Cannot get new azure session, will use tcpdump tool to verify egressIP,%v", err)
							flag = "tcpdump"
						} else {
							_, intSvcErr := getAzureIntSvcVMPublicIP(oc, az, rg)
							if intSvcErr != nil {
								e2e.Logf("There is no int svc instance in this cluster, %v. Will use tcpdump tool to verify egressIP", intSvcErr)
								flag = "tcpdump"
							} else {
								ipEchoURL, intSvcErr = installIPEchoServiceOnAzure(oc, az, rg)
								if intSvcErr != nil && ipEchoURL != "" {
									e2e.Logf("No ip-echo service installed on the bastion host, %v. Will use tcpdump tool to verify egressIP", intSvcErr)
									flag = "tcpdump"
								} else {
									e2e.Logf("bastion host and ip-echo service instaled successfully, use ip-echo service to verify")
									flag = "ipecho"
								}
							}
						}
					}
				}
			}
		case "openstack":
			e2e.Logf("\n OpenStack is detected, running the case on OpenStack\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on OpenStack")
		case "vsphere":
			e2e.Logf("\n Vsphere is detected, running the case on Vsphere\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on Vsphere")
		case "baremetal":
			e2e.Logf("\n BareMetal is detected, running the case on BareMetal\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on BareMetal")
		case "none":
			e2e.Logf("\n UPI BareMetal is detected, running the case on UPI BareMetal\n")
			ipEchoURL = getIPechoURLFromUPIPrivateVlanBM(oc)
			e2e.Logf("IP echo URL is %s", ipEchoURL)
			if ipEchoURL == "" {
				g.Skip("This UPI Baremetal cluster did not fulfill the prequiste of testing egressIP cases, skip the test!!")
			}
			flag = "ipecho"
			e2e.Logf("Use IP echo way to verify egressIP on UPI BareMetal")
		case "nutanix":
			e2e.Logf("\n Nutanix is detected, running the case on Nutanix\n")
			flag = "tcpdump"
			e2e.Logf("Use tcpdump way to verify egressIP on Nutanix")
		default:
			e2e.Logf("Not support cloud provider for  egressip cases for now.")
			g.Skip("Not support cloud provider for  egressip cases for now.")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47272-Pods will not be affected by the egressIP set on other netnamespace. [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1.1 Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		exutil.By("1.2 Apply EgressLabel Key to one node.")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)

		exutil.By("2.1 Create first egressip object")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47272-1",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("2.2 Create second egressip object")
		egressip2 := egressIPResource1{
			name:          "egressip-47272-2",
			template:      egressIP2Template,
			egressIP1:     freeIPs[1],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "blue",
		}
		egressip2.createEgressIPObject2(oc)
		defer egressip2.deleteEgressIPObject1(oc)
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip2.name)
		o.Expect(len(egressIPMaps2)).Should(o.Equal(1))

		exutil.By("3.1 create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("3.2 Apply a label to first namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.3 Create a pod in first namespace. ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("3.4 Apply label to pod in first namespace")
		err = exutil.LabelPod(oc, ns1, pod1.name, "color=pink")
		defer exutil.LabelPod(oc, ns1, pod1.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4.1 create second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("4.2 Apply a label to second namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4.3 Create a pod in second namespace ")
		pod2 := pingPodResource{
			name:      "hello-pod",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2.name, "-n", pod2.namespace).Execute()
		waitPodReady(oc, pod2.namespace, pod2.name)

		exutil.By("4.4 Apply label to pod in second namespace")
		err = exutil.LabelPod(oc, ns2, pod2.name, "color=blue")
		defer exutil.LabelPod(oc, ns2, pod2.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5.1 Check source IP in first namespace using first egressip object")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[0])

			exutil.By("5.2 Check source IP in second namespace using second egressip object")
			verifyEgressIPWithIPEcho(oc, pod2.namespace, pod2.name, ipEchoURL, true, freeIPs[1])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47272", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47272", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[0], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			exutil.By("5.2 Check source IP in second namespace using second egressip object")
			egressErr2 := verifyEgressIPinTCPDump(oc, pod2.name, pod2.namespace, freeIPs[1], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr2).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		exutil.By("Pods will not be affected by the egressIP set on other netnamespace.!!! ")
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47164-Medium-47025-Be able to update egressip object,The pods removed matched labels will not use EgressIP [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1.1 Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		exutil.By("1.2 Apply EgressLabel Key to one node.")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)

		exutil.By("2.1 Create first egressip object")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47164",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("3.1 create first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("3.2 Apply a label to first namespace")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.3 Create a pod in first namespace. ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("3.4 Apply label to pod in first namespace")
		err = exutil.LabelPod(oc, ns1, pod1.name, "color=pink")
		defer exutil.LabelPod(oc, ns1, pod1.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Update the egressip in egressip object")
		updateEgressIPObject(oc, egressip1.name, freeIPs[1])

		exutil.By("5. Check source IP is updated IP")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, freeIPs[1])

			exutil.By("6. Remove labels from test pod.")
			err = exutil.LabelPod(oc, ns1, pod1.name, "color-")
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("7. Check source IP is not EgressIP")
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, false, freeIPs[1])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47164", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47164", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", freeIPs[1]))

			exutil.By("6. Remove labels from test pod.")
			err = exutil.LabelPod(oc, ns1, pod1.name, "color-")
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("7. Check source IP is not EgressIP")
			egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, freeIPs[1], dstHost, ns1, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Should not get egressip:%s", freeIPs[1]))

		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47030-An EgressIP object can not have multiple egress IP assignments on the same node. [Serial]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("1. Get two worker nodes with same subnets")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("2. Apply EgressLabel Key for this test on one node.")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)

		exutil.By("3. Create an egressip object")
		freeIPs := findFreeIPs(oc, egressNodes[0], 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47030",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("4. Check only one EgressIP assigned in the object.")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("5. Apply EgressLabel Key for this test on second node.")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		exutil.By("6. Check two EgressIP assigned in the object.")
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Medium-47028-After remove EgressIP node tag, EgressIP will failover to other availabel egress nodes. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("1. Get list of nodes, get subnet from two worker nodes that have same subnet \n")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		exutil.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)

		exutil.By("3.1 Create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()
		exutil.By("3.2 Apply label to namespace\n")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Output()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create a pod in first namespace. \n")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode1, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47028",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("4. Check EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("5. Update Egress node to egressNode2.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)

		exutil.By("6. Check the egress node was updated in the egressip object.\n")
		egressipErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			egressIPMaps = getAssignedEIPInEIPObject(oc, egressip1.name)
			if len(egressIPMaps) != 1 || egressIPMaps[0]["node"] == egressNode1 {
				e2e.Logf("Wait for new egress node applied,try next round.")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to update egress node:%s", egressipErr))
		o.Expect(egressIPMaps[0]["node"]).Should(o.ContainSubstring(egressNode2))

		exutil.By("7. Check the source ip.\n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, egressIPMaps[0]["egressIP"])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47028", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47028", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-High-47031-After reboot egress node EgressIP still work.  [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1.1 Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		exutil.By("1.2 Apply EgressLabel Key to one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("2.1 Create first egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-47031",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("3.1 create first namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("3.2 Apply a label to test namespace.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.3 Create pods in test namespace. \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		exutil.By("3.4 Apply label to one pod in test namespace\n")
		testPodName := getPodName(oc, ns1, "name=test-pods")
		err = exutil.LabelPod(oc, ns1, testPodName[0], "color=pink")
		defer exutil.LabelPod(oc, ns1, testPodName[0], "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Check only one EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("5.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNode, "Ready")
		rebootNode(oc, egressNode)
		checkNodeStatus(oc, egressNode, "NotReady")
		checkNodeStatus(oc, egressNode, "Ready")
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodName = getPodName(oc, ns1, "name=test-pods")
		_, err = exutil.AddLabelsToSpecificResource(oc, "pod/"+testPodName[0], ns1, "color=pink")
		defer exutil.LabelPod(oc, ns1, testPodName[0], "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7. Check EgressIP assigned in the object.\n")
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 1)

		exutil.By("8. Check source IP is egressIP \n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf(" ipEchoURL is %v", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, ns1, testPodName[0], ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47031", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47031", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, testPodName[0], ns1, freeIPs[0], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-Critical-47032-High-47034-Traffic is load balanced between egress nodes,multiple EgressIP objects can have multiple egress IPs.[Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("Apply EgressLabel Key for this test on one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		exutil.By("Apply label to namespace\n")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNodes[0], 4)
		o.Expect(len(freeIPs)).Should(o.Equal(4))
		egressip1 := egressIPResource1{
			name:      "egressip-47032",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		//Replce matchLabel with matchExpressions
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47032", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"name\", \"operator\": \"In\", \"values\": [\"test\"]}]}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47032", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchLabels\":null}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)

		exutil.By("Create another egressip object\n")
		egressip2 := egressIPResource1{
			name:      "egressip-47034",
			template:  egressIPTemplate,
			egressIP1: freeIPs[2],
			egressIP2: freeIPs[3],
		}
		egressip2.createEgressIPObject1(oc)
		defer egressip2.deleteEgressIPObject1(oc)
		//Replce matchLabel with matchExpressions
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47034", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchExpressions\":[{\"key\": \"name\", \"operator\": \"In\", \"values\": [\"qe\"]}]}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47034", "-p", "{\"spec\":{\"namespaceSelector\":{\"matchLabels\":null}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		verifyExpectedEIPNumInEIPObject(oc, egressip2.name, 2)

		exutil.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("Create sencond namespace.")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("Apply label to second namespace\n")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a pod in second namespace")
		pod2 := pingPodResource{
			name:      "hello-pod",
			namespace: ns2,
			template:  pingPodTemplate,
		}
		pod2.createPingPod(oc)
		waitPodReady(oc, pod2.namespace, pod2.name)

		exutil.By("Check source IP is randomly one of egress ips.\n")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := execCommandInSpecificPod(oc, pod2.namespace, pod2.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[2]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[3]))
			sourceIP, err = execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..10}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIPs[1]))
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNodes[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47032", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47032", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("Check source IP is randomly one of egress ips for both namespaces.")
			egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, err := execCommandInSpecificPod(oc, pod2.namespace, pod2.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				if checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[2], true) != nil || checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[3], true) != nil || err != nil {
					e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump", freeIPs[2], freeIPs[3]))

			egressipErr2 := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..10}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				if checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[0], true) != nil || checkMatchedIPs(oc, ns2, tcpdumpDS.name, randomStr, freeIPs[1], true) != nil || err != nil {
					e2e.Logf("No matched egressIPs in tcpdump log, try next round.")
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr2, fmt.Sprintf("Failed to get both EgressIPs %s,%s in tcpdump", freeIPs[0], freeIPs[1]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-47019-High-47023-EgressIP works well with networkpolicy and egressFirewall. [Serial]", func() {
		//EgressFirewall case cannot run in proxy cluster, skip if proxy cluster.
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		networkPolicyFile := filepath.Join(buildPruningBaseDir, "networkpolicy/default-deny-ingress.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		egressFWTemplate := filepath.Join(buildPruningBaseDir, "egressfirewall2-template.yaml")

		exutil.By("1. Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		egressNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("3. create new namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		exutil.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, nodeList.Items[0].Name, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47019",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("6. Create test pods \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, oc.Namespace(), "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		exutil.By("7. Create default deny ingress type networkpolicy in test namespace\n")
		createResourceFromFile(oc, ns1, networkPolicyFile)
		output, err := oc.Run("get").Args("networkpolicy").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("default-deny-ingress"))

		exutil.By("8. Create an EgressFirewall object with rule deny.")
		egressFW2 := egressFirewall2{
			name:      "default",
			namespace: ns1,
			ruletype:  "Deny",
			cidr:      "0.0.0.0/0",
			template:  egressFWTemplate,
		}
		egressFW2.createEgressFW2Object(oc)
		defer egressFW2.deleteEgressFW2Object(oc)

		exutil.By("9. Get test pods IP and test pod name in test namespace\n")
		testPodName := getPodName(oc, oc.Namespace(), "name=test-pods")

		exutil.By("10. Check network policy works. \n")
		CurlPod2PodFail(oc, ns1, testPodName[0], ns1, testPodName[1])

		exutil.By("11. Check EgressFirewall policy works. \n")
		_, err = e2eoutput.RunHostCmd(ns1, testPodName[0], "curl -s ifconfig.me --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("12.Update EgressFirewall to allow")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressfirewall.k8s.ovn.org/default", "-n", ns1, "-p", "{\"spec\":{\"egress\":[{\"type\":\"Allow\",\"to\":{\"cidrSelector\":\"0.0.0.0/0\"}}]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By("13. Check EgressFirewall Allow rule works and EgressIP works.\n")
			egressipErr := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
				sourceIP, err := e2eoutput.RunHostCmd(ns1, testPodName[0], "curl -s "+ipEchoURL+" --connect-timeout 5")
				if err != nil {
					e2e.Logf("Wait for EgressFirewall taking effect. %v", err)
					return false, nil
				}
				if !contains(freeIPs, sourceIP) {
					eip, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", "-o=jsonpath={.}").Output()
					e2e.Logf(eip)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("The source Ip is not same as the egressIP expected!"))
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47023", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47023", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("13. Verify from tcpdump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, testPodName[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-Medium-47018-Medium-47017-Multiple projects use same EgressIP,EgressIP works for all pods in the namespace with matched namespaceSelector. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("1. Label EgressIP node\n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		egressNode := nodeList.Items[0].Name
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("3. create first namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("4. Create test pods in first namespace. \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNs1Name := getPodName(oc, ns1, "name=test-pods")

		exutil.By("5. Apply label to ns1 namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		exutil.By("6. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47018",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("7. create new namespace\n")
		oc.SetupProject()
		ns2 := oc.Namespace()

		exutil.By("8. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "name-").Execute()

		exutil.By("9. Create test pods in second namespace  \n")
		createResourceFromFile(oc, ns2, testPodFile)
		err = waitForPodWithLabelReady(oc, ns2, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNs2Name := getPodName(oc, ns2, "name=test-pods")

		exutil.By("create new namespace\n")
		oc.SetupProject()
		ns3 := oc.Namespace()

		exutil.By("Create test pods in third namespace  \n")
		createResourceFromFile(oc, ns3, testPodFile)
		err = waitForPodWithLabelReady(oc, ns3, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")
		testPodNs3Name := getPodName(oc, ns3, "name=test-pods")

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By("10. Check source IP from both namespace, should be egressip.  \n")
			verifyEgressIPWithIPEcho(oc, ns1, testPodNs1Name[0], ipEchoURL, true, freeIPs...)
			verifyEgressIPWithIPEcho(oc, ns1, testPodNs1Name[1], ipEchoURL, true, freeIPs...)
			verifyEgressIPWithIPEcho(oc, ns2, testPodNs2Name[0], ipEchoURL, true, freeIPs...)
			verifyEgressIPWithIPEcho(oc, ns2, testPodNs2Name[1], ipEchoURL, true, freeIPs...)
			verifyEgressIPWithIPEcho(oc, ns3, testPodNs3Name[0], ipEchoURL, false, freeIPs...)

			exutil.By("11. Remove matched labels from namespace ns1  \n")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("12.  Check source IP from namespace ns1, should not be egressip. \n")
			verifyEgressIPWithIPEcho(oc, ns1, testPodNs1Name[0], ipEchoURL, false, freeIPs...)
			verifyEgressIPWithIPEcho(oc, ns1, testPodNs1Name[1], ipEchoURL, false, freeIPs...)
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47017", ns2)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns2, "tcpdump-47017", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("10.Check source IP from both namespace, should be egressip. ")
			egressErr := verifyEgressIPinTCPDump(oc, testPodNs1Name[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[1], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs2Name[0], ns2, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs2Name[0], ns2, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs3Name[0], ns3, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			exutil.By("11. Remove matched labels from namespace ns1  \n")
			err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			exutil.By("12.  Check source IP from namespace ns1, should not be egressip. \n")
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[0], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
			egressErr = verifyEgressIPinTCPDump(oc, testPodNs1Name[1], ns1, egressIPMaps[0]["egressIP"], dstHost, ns2, tcpdumpDS.name, false)
			o.Expect(egressErr).NotTo(o.HaveOccurred())
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:huirwang-Longduration-NonPreRelease-Medium-47033-If an egress node is NotReady traffic is still load balanced between available egress nodes. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		timer := estimateTimeoutForEgressIP(oc) * 2

		// This test case is not supposed to run on some special AWS/GCP cluster with STS, use specialPlatformCheck function to identify such a cluster
		// For Azure cluster, if it has special credential type, this test case should be skipped as well
		isSpecialSTSorCredCluster := specialPlatformCheck(oc)
		if isSpecialSTSorCredCluster {
			g.Skip("Skipped: This test case is not suitable for special AWS/GCP STS cluster or Azure with special credential type!!")
		}

		exutil.By("1. create new namespace\n")
		ns1 := oc.Namespace()

		exutil.By("2. Label EgressIP node\n")
		// As in rdu1 cluster, sriov nodes have different primary NIC name from common node, we need uniq nic name for multiple tcpdump pods to capture packets, so filter out sriov nodes
		platform := exutil.CheckPlatform(oc)
		var workers []string
		if strings.Contains(platform, "baremetal") {
			workers = excludeSriovNodes(oc)
			if len(workers) < 3 {
				g.Skip("Not enough worker nodes for this test, skip the case!!")
			}
		} else {
			nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(nodeList.Items) < 3 {
				g.Skip("Not enough worker nodes for this test, skip the case!!")
			}
			for _, node := range nodeList.Items {
				workers = append(workers, node.Name)
			}

		}

		exutil.By("3. Apply EgressLabel Key for this test on 3 nodes.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[1], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[2], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[2], egressNodeLabel)

		exutil.By("4. Apply label to namespace\n")
		err := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name-").Execute()

		exutil.By("5. Create an egressip object\n")
		sub1 := getEgressCIDRsForNode(oc, workers[0])
		freeIP1 := findUnUsedIPsOnNode(oc, workers[0], sub1, 1)
		o.Expect(len(freeIP1) == 1).Should(o.BeTrue())
		sub2 := getEgressCIDRsForNode(oc, workers[1])
		freeIP2 := findUnUsedIPsOnNode(oc, workers[1], sub2, 1)
		o.Expect(len(freeIP2) == 1).Should(o.BeTrue())
		sub3 := getEgressCIDRsForNode(oc, workers[2])
		freeIP3 := findUnUsedIPsOnNode(oc, workers[2], sub3, 1)
		o.Expect(len(freeIP3) == 1).Should(o.BeTrue())

		egressip1 := egressIPResource1{
			name:      "egressip-47033",
			template:  egressIPTemplate,
			egressIP1: freeIP1[0],
			egressIP2: freeIP2[0],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("6. Update an egressip object with three egressips.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/egressip-47033", "-p", "{\"spec\":{\"egressIPs\":[\""+freeIP1[0]+"\",\""+freeIP2[0]+"\",\""+freeIP3[0]+"\"]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7. Create a pod \n")
		pod1 := pingPodResourceNode{
			name:      "hello-pod",
			namespace: ns1,
			nodename:  workers[0],
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("8. Check source IP is randomly one of egress ips.\n")
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 3)

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		errMsgFmt := "Any error in finding %v in tcpdump?: %v\n\n\n"
		switch flag {
		case "ipecho":
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			sourceIP, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..15}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(sourceIP)
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIP1[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIP2[0]))
			o.Expect(sourceIP).Should(o.ContainSubstring(freeIP3[0]))
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[0], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[0], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[1], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[1], "tcpdump", "true")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workers[2], "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workers[2], "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, workers[0])
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-47033", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-47033", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("Verify all egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(30*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..30}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP1[0], true)
				e2e.Logf(errMsgFmt, freeIP1[0], egressIPCheck1)
				egressIPCheck2 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP2[0], true)
				e2e.Logf(errMsgFmt, freeIP2[0], egressIPCheck2)
				egressIPCheck3 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP3[0], true)
				e2e.Logf(errMsgFmt, freeIP3[0], egressIPCheck3)
				e2e.Logf("Any cmdErr when running curl?: %v\n\n\n", cmdErr)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Did not find egressIPs %s or %s or %s in tcpdump log, try next round.", freeIP1[0], freeIP2[0], freeIP3[0])
					return false, nil
				}
				e2e.Logf("Found all other 3 egressIP in tcpdump log as expected")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get all EgressIPs %s,%s, %s in tcpdump", freeIP1[0], freeIP2[0], freeIP3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)

		// Choose one egress node and shut it down
		nodeToBeShutdown := egressIPMaps1[2]["node"]
		e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", nodeToBeShutdown)
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeToBeShutdown, "tcpdump")

		exutil.By("9. Stop one egress node.\n")
		var instance []string
		var zone string
		var az *exutil.AzureSession
		var rg string
		var infraID string
		var ospObj exutil.Osp
		var vspObj *exutil.Vmware
		var vspClient *govmomi.Client
		var nutanixClient *exutil.NutanixClient
		switch exutil.CheckPlatform(oc) {
		case "aws":
			e2e.Logf("\n AWS is detected \n")
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer startInstanceOnAWS(a, nodeToBeShutdown)
			stopInstanceOnAWS(a, nodeToBeShutdown)
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "gcp":
			// for gcp, remove the postfix "c.openshift-qe.internal" to get its instance name
			instance = strings.Split(nodeToBeShutdown, ".")
			e2e.Logf("\n\n\n the worker node to be shutdown is: %v\n\n\n", instance[0])
			infraID, err = exutil.GetInfraID(oc)
			zone, err = getZoneOfInstanceFromGcp(oc, infraID, instance[0])
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer startInstanceOnGcp(oc, instance[0], zone)
			err = stopInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "azure":
			e2e.Logf("\n Azure is detected \n")
			err := getAzureCredentialFromCluster(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			rg, err = getAzureResourceGroup(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			az, err = exutil.NewAzureSessionFromEnv()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer startVMOnAzure(az, nodeToBeShutdown, rg)
			stopVMOnAzure(az, nodeToBeShutdown, rg)
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "openstack":
			e2e.Logf("\n OpenStack is detected, stop the instance %v on OSP now \n", nodeToBeShutdown)
			ospObj = exutil.Osp{}
			OspCredentials(oc)
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer ospObj.GetStartOspInstance(nodeToBeShutdown)
			err = ospObj.GetStopOspInstance(nodeToBeShutdown)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "vsphere":
			e2e.Logf("\n vSphere is detected, stop the instance %v on OSP now \n", nodeToBeShutdown)
			vspObj, vspClient = VsphereCloudClient(oc)
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer vspObj.StartVsphereInstance(vspClient, nodeToBeShutdown)
			err = vspObj.StopVsphereInstance(vspClient, nodeToBeShutdown)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "baremetal":
			e2e.Logf("\n IPI baremetal is detected \n")
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer startVMOnIPIBM(oc, nodeToBeShutdown)
			stopErr := stopVMOnIPIBM(oc, nodeToBeShutdown)
			o.Expect(stopErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		case "nutanix":
			e2e.Logf("\n Nutanix is detected, stop the instance %v on nutanix now \n", nodeToBeShutdown)
			nutanixClient, err = exutil.InitNutanixClient(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			defer startInstanceOnNutanix(nutanixClient, nodeToBeShutdown)
			stopInstanceOnNutanix(nutanixClient, nodeToBeShutdown)
			checkNodeStatus(oc, nodeToBeShutdown, "NotReady")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		exutil.By("10. Check EgressIP updated in EIP object, sourceIP contains 2 IPs. \n")
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)

		switch flag {
		case "ipecho":
			egressipErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
				sourceIP, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..30}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
				e2e.Logf(sourceIP)
				if err != nil {
					e2e.Logf("Getting error: %v while curl %s from the pod.", err, ipEchoURL)
					return false, nil
				}
				if strings.Contains(sourceIP, egressIPMaps1[0]["egressIP"]) && strings.Contains(sourceIP, egressIPMaps1[1]["egressIP"]) {
					sourceIPSlice := findIP(sourceIP)
					if len(unique(sourceIPSlice)) == 2 {
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("The source Ip is not same as the egressIP expected!"))
		case "tcpdump":
			exutil.By("Verify other available egressIP is randomly used as sourceIP.")
			egressipErr := wait.Poll(30*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..30}; do curl -s "+url+" --connect-timeout 5 ; sleep 3;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())

				egressIPCheck1 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, egressIPMaps1[0]["egressIP"], true)
				e2e.Logf(errMsgFmt, egressIPMaps1[0]["egressIP"], egressIPCheck1)
				egressIPCheck2 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, egressIPMaps1[1]["egressIP"], true)
				e2e.Logf(errMsgFmt, egressIPMaps1[1]["egressIP"], egressIPCheck2)
				egressIPCheck3 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, egressIPMaps1[2]["egressIP"], false)
				e2e.Logf("Any error in finding %v in tcpdump when it is not expected to be in tcpdump log?: %v\n\n\n", egressIPMaps1[2]["egressIP"], egressIPCheck3)
				e2e.Logf("Any cmdErr when running curl?: %v\n\n\n", cmdErr)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Did not find egressIPs %v or %v in tcpdump log, or found %v unexpected, try next round.", egressIPMaps1[0]["egressIP"], egressIPMaps1[1]["egressIP"], egressIPMaps1[2]["egressIP"])
					return false, nil
				}
				e2e.Logf("After the egress node is shut down, found all other 2 egressIP in tcpdump log!as expected")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get all expected EgressIPs in tcpdump log"))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		exutil.By("11. Start the stopped egress node \n")
		switch exutil.CheckPlatform(oc) {
		case "aws":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			startInstanceOnAWS(a, nodeToBeShutdown)
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "gcp":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			err = startInstanceOnGcp(oc, instance[0], zone)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "azure":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			startVMOnAzure(az, nodeToBeShutdown, rg)
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "openstack":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			err = ospObj.GetStartOspInstance(nodeToBeShutdown)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "vsphere":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			err = vspObj.StartVsphereInstance(vspClient, nodeToBeShutdown)
			o.Expect(err).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "baremetal":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			startErr := startVMOnIPIBM(oc, nodeToBeShutdown)
			o.Expect(startErr).NotTo(o.HaveOccurred())
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		case "nutanix":
			defer checkNodeStatus(oc, nodeToBeShutdown, "Ready")
			startInstanceOnNutanix(nutanixClient, nodeToBeShutdown)
			checkNodeStatus(oc, nodeToBeShutdown, "Ready")
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

		exutil.By("12. Check source IP is randomly one of 3 egress IPs.\n")
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 3)

		switch flag {
		case "ipecho":
			egressipErr := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
				sourceIP, err := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..30}; do curl -s "+ipEchoURL+" --connect-timeout 5 ; sleep 2;echo ;done")
				e2e.Logf(sourceIP)
				if err != nil {
					e2e.Logf("Getting error: %v while curl %s from the pod.", err, ipEchoURL)
					return false, nil
				}
				if strings.Contains(sourceIP, freeIP1[0]) && strings.Contains(sourceIP, freeIP2[0]) && strings.Contains(sourceIP, freeIP3[0]) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("The source Ip is not same as the egressIP expected!"))
		case "tcpdump":
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeToBeShutdown, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeToBeShutdown, "tcpdump", "true")
			egressipErr := wait.Poll(30*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, pod1.namespace, pod1.name, "for i in {1..30}; do curl -s "+url+" --connect-timeout 5 ; sleep 3;echo ;done")
				o.Expect(err).NotTo(o.HaveOccurred())
				egressIPCheck1 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP1[0], true)
				e2e.Logf(errMsgFmt, freeIP1[0], egressIPCheck1)

				egressIPCheck2 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP2[0], true)
				e2e.Logf(errMsgFmt, freeIP2[0], egressIPCheck2)

				egressIPCheck3 := checkMatchedIPs(oc, ns1, tcpdumpDS.name, randomStr, freeIP3[0], true)
				e2e.Logf(errMsgFmt, freeIP3[0], egressIPCheck3)

				e2e.Logf("Any cmdErr when running curl?: %v\n\n\n", cmdErr)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || egressIPCheck3 != nil || cmdErr != nil {
					e2e.Logf("Did not find egressIPs %s or %s or %s in tcpdump log, try next round.", freeIP1[0], freeIP2[0], freeIP3[0])
					return false, nil
				}
				e2e.Logf("After the egress node is brought back up, found all 3 egressIP in tcpdump log!as expected")
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get all EgressIPs %s,%s, %s in tcpdump", freeIP1[0], freeIP2[0], freeIP3[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-Longduration-NonPreRelease-53069-EgressIP should work for recreated same name pod. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		exutil.By("1. Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		exutil.By("2. Apply EgressLabel Key for this test on one node.\n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("3.1 Get temp namespace\n")
		oc.SetupProject()
		ns1 := oc.Namespace()

		exutil.By("3.2 Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("4. Create a pod in temp namespace. \n")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("5. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-53069",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("4. Check EgressIP assigned in the object.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By("5. Check the source ip.\n")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, egressIPMaps[0]["egressIP"])

			exutil.By("6. Delete the test pod and recreate it. \n")
			// Add more times to delete pod and recreate pod. This is to cover bug https://bugzilla.redhat.com/show_bug.cgi?id=2117310
			exutil.By("6. Delete the test pod and recreate it. \n")
			for i := 0; i < 15; i++ {
				e2e.Logf("Delete and recreate pod for the %v time", i)
				pod1.deletePingPod(oc)
				pod1.createPingPod(oc)
				waitPodReady(oc, pod1.namespace, pod1.name)

				exutil.By("7. Check the source ip.\n")
				verifyEgressIPWithIPEcho(oc, pod1.namespace, pod1.name, ipEchoURL, true, egressIPMaps[0]["egressIP"])
			}
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-53069", ns1)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns1, "tcpdump-53069", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())

			exutil.By("5. Verify from tcpdump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred())

			exutil.By("6. Delete the test pod and recreate it for. \n")
			for i := 0; i < 15; i++ {
				e2e.Logf("Delete and recreate pod for the %v time", i)
				pod1.deletePingPod(oc)
				pod1.createPingPod(oc)
				waitPodReady(oc, pod1.namespace, pod1.name)

				exutil.By("7. Verify from tcpdump that source IP is EgressIP")
				egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps[0]["egressIP"], dstHost, ns1, tcpdumpDS.name, true)
				o.Expect(egressErr).NotTo(o.HaveOccurred())
			}

		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-PreChkUpgrade-Author:jechen-High-56875-OVN egressIP should still be functional post upgrade. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		statefulSetHelloPod := filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
		ns := "56875-upgrade-ns"

		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())

		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 := egressNodes[0]

		exutil.By("1. Create a namespace, apply namespace label to it that matches the one defined in egressip object.")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		exutil.SetNamespacePrivileged(oc, ns)

		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2. Choose a node as EgressIP node, label the node to be egress assignable")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("3. Create an egressip object")
		freeIPs := findFreeIPs(oc, egressNode1, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-56875",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("4. Create a pod in the namespace and apply pod label to the pod that matches the podLabel defined in egressip object created in step 2.")
		createResourceFromFile(oc, ns, statefulSetHelloPod)
		podErr := waitForPodWithLabelReady(oc, ns, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
		helloPodname := getPodName(oc, ns, "app=hello")

		err = exutil.LabelPod(oc, ns, helloPodname[0], "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Check source IP is the assigned egress IP address")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, freeIPs[0])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-56875", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-56875", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, helloPodname[0], ns, freeIPs[0], dstHost, ns, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", freeIPs[0]))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-PstChkUpgrade-Author:jechen-High-56875-OVN egressIP should still be functional post upgrade. [Disruptive]", func() {

		ns := "56875-upgrade-ns"
		nsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", ns).Execute()
		if nsErr != nil {
			g.Skip("Skip the PstChkUpgrade test as 56875-upgrade-ns namespace does not exist, PreChkUpgrade test did not run")
		}

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns, "--ignore-not-found=true").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", "hello-", "-n", ns, "--ignore-not-found=true").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("egressip", "--all").Execute()

		egressNodeList := exutil.GetNodeListByLabel(oc, egressNodeLabel)
		for _, labelledEgressNode := range egressNodeList {
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, labelledEgressNode, egressNodeLabel)
		}

		nodeNum := 2
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		exutil.By("1. Check EgressIP in EIP object, sourceIP contains one IP. \n")
		EIPObjects := getOVNEgressIPObject(oc)
		o.Expect(len(EIPObjects) == 1).Should(o.BeTrue())
		EIPObjectName := EIPObjects[0]
		egressIPMaps := getAssignedEIPInEIPObject(oc, EIPObjectName)
		o.Expect(len(egressIPMaps) == 1).Should(o.BeTrue())
		egressNode1 := egressIPMaps[0]["node"]
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		exutil.SetNamespacePrivileged(oc, ns)
		helloPodname := getPodName(oc, ns, "app=hello")

		// pod needs to be re-labelled after upgrade
		podErr := exutil.LabelPod(oc, ns, helloPodname[0], "color=pink")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")

		exutil.By("2. Check source IP from the test pod of the namespace is the assigned egress IP address")
		var dstHost, primaryInf string
		var infErr, snifErr error
		var tcpdumpDS, tcpdumpDS2, tcpdumpDS3 *tcpdumpDaemonSet
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			e2e.Logf("\n ipEchoURL is %v\n", ipEchoURL)
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, egressIPMaps[0]["egressIP"])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode1)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-56875", ns)
			tcpdumpDS, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-56875", "tcpdump", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, helloPodname[0], ns, egressIPMaps[0]["egressIP"], dstHost, ns, tcpdumpDS.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("Failed to get expected egressip:%s", egressIPMaps[0]["egressIP"]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		exutil.By("3. Find another scheduleable node that is in the same subnet of first egress node, label it as the second egress node")
		var egressNode2 string
		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "azure") || strings.Contains(platform, "openstack") {
			firstSub := getIfaddrFromNode(egressNode1, oc)
			for _, v := range nodeList.Items {
				secondSub := getIfaddrFromNode(v.Name, oc)
				if v.Name == egressNode1 || secondSub != firstSub {
					continue
				} else {
					egressNode2 = v.Name
					break
				}
			}
		} else { // On other BM, vSphere platforms, worker nodes are on same subnet
			for _, v := range nodeList.Items {
				if v.Name == egressNode1 {
					continue
				} else {
					egressNode2 = v.Name
					break
				}
			}
		}

		if egressNode2 == "" {
			g.Skip("Did not find a scheduleable second node that is on same subnet as the first egress node, skip the rest of the test!!")
		}
		e2e.Logf("\n secondEgressNode is %v\n", egressNode2)
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")

		egressNodeList = exutil.GetNodeListByLabel(oc, egressNodeLabel)
		o.Expect(len(egressNodeList) == 2).Should(o.BeTrue())

		exutil.By("4. Unlabel the first egress node to cause egressIP failover to the second egress node")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)

		// stateful test pod would be recreated during failover, wait for pod to be ready and relabel the pod
		podErr = waitForPodWithLabelReady(oc, ns, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
		helloPodname = getPodName(oc, ns, "app=hello")
		err = exutil.LabelPod(oc, ns, helloPodname[0], "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5. Check EgressIP assigned to the second egress node.\n")
		o.Eventually(func() bool {
			egressIPMaps = getAssignedEIPInEIPObject(oc, EIPObjectName)
			return len(egressIPMaps) == 1 && egressIPMaps[0]["node"] == egressNode2
		}, "300s", "10s").Should(o.BeTrue(), "egressIP was not migrated to second egress node after unlabel first egress node!!")

		exutil.By("6. Check source IP from the test pod of the namespace is still the egressIP after egressIP failover \n")
		switch flag {
		case "ipecho":
			exutil.By(" Use IP-echo service to verify egressIP.")
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, egressIPMaps[0]["egressIP"])
		case "tcpdump":
			exutil.By(" Use tcpdump to verify egressIP, create tcpdump sniffer Daemonset first.")
			e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, "tcpdump")
			deleteTcpdumpDS(oc, "tcpdump-56875", ns)
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2", "true")
			primaryInf, infErr = getSnifPhyInf(oc, egressNode2)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-56875-2", ns)
			tcpdumpDS2, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-56875-2", "tcpdump2", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			exutil.By("Verify from tcpDump that source IP is EgressIP")
			egressErr := verifyEgressIPinTCPDump(oc, helloPodname[0], ns, egressIPMaps[0]["egressIP"], dstHost, ns, tcpdumpDS2.name, true)
			o.Expect(egressErr).NotTo(o.HaveOccurred(), fmt.Sprintf("After failover, failed to get expected egressip:%s in tcpdump", egressIPMaps[0]["egressIP"]))
		default:
			g.Skip("Skip for not support scenarios!")
		}

		exutil.By("7. Delete egressIP object, verify egressip is cleared\n")
		o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("egressip", EIPObjectName).Execute()).NotTo(o.HaveOccurred())
		waitCloudPrivateIPconfigUpdate(oc, egressIPMaps[0]["egressIP"], false)

		exutil.By("8. Verify node's IP will be used as source IP from the namespace and test pod\n")
		// Find the node that the hello-pod resides on
		PodNodeName, nodeErr := exutil.GetPodNodeName(oc, ns, helloPodname[0])
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		nodeIP := getNodeIPv4(oc, ns, PodNodeName)
		timer := estimateTimeoutForEgressIP(oc)

		switch flag {
		case "ipecho":
			verifyEgressIPWithIPEcho(oc, ns, helloPodname[0], ipEchoURL, true, nodeIP)
		case "tcpdump":
			e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, "tcpdump2")
			deleteTcpdumpDS(oc, "tcpdump-56875-2", ns)
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump3")
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, PodNodeName, "tcpdump3", "true")
			primaryInf, infErr = getSnifPhyInf(oc, PodNodeName)
			o.Expect(infErr).NotTo(o.HaveOccurred())
			dstHost = nslookDomainName("ifconfig.me")
			defer deleteTcpdumpDS(oc, "tcpdump-56875-3", ns)
			tcpdumpDS3, snifErr = createSnifferDaemonset(oc, ns, "tcpdump-56875-3", "tcpdump3", "true", dstHost, primaryInf, 80)
			o.Expect(snifErr).NotTo(o.HaveOccurred())
			egressipErr := wait.Poll(10*time.Second, timer, func() (bool, error) {
				randomStr, url := getRequestURL(dstHost)
				_, cmdErr := execCommandInSpecificPod(oc, ns, helloPodname[0], "for i in {1..15}; do curl -s "+url+" --connect-timeout 5 ; sleep 2;echo ;done")
				o.Expect(cmdErr).NotTo(o.HaveOccurred())
				e2e.Logf("\n Check1: if source IP is IP of the node where test pod resides\n")
				egressIPCheck1 := checkMatchedIPs(oc, ns, tcpdumpDS3.name, randomStr, nodeIP, true)
				e2e.Logf("\n Check2: original egressip is not in tcpdump log as source IP\n")
				egressIPCheck2 := checkMatchedIPs(oc, ns, tcpdumpDS3.name, randomStr, egressIPMaps[0]["egressIP"], false)
				e2e.Logf("\n egressIPCheck1: %v,   egressIPCheck2: %v\n", egressIPCheck1, egressIPCheck2)
				if egressIPCheck1 != nil || egressIPCheck2 != nil || cmdErr != nil {
					e2e.Logf("Got %v unexpectedly, or did not find %s as expected in tcpdump log, try next round.", egressIPMaps[0]["egressIP"], nodeIP)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to get %s as source IP in tcpdump", nodeIP))
		default:
			g.Skip("Skip for not support scenarios!")
		}
	})
})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP Basic", func() {
	//Cases in this function, do not need curl ip-echo
	defer g.GinkgoRecover()

	var (
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "none") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS/GCP/Azure/Openstack/Vsphere/Baremetal/Nutanix cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
		if strings.Contains(platform, "none") {
			// For UPI baremetal, egressIP cases only can be tested on clusters from upi-on-baremetal/versioned-installer-packet-http_proxy-private-vlan as some limitations on other clusters.
			e2e.Logf("\n UPI BareMetal is detected, running the case on UPI BareMetal\n")
			ipEchoURL := getIPechoURLFromUPIPrivateVlanBM(oc)
			e2e.Logf("IP echo URL is %s", ipEchoURL)
			if ipEchoURL == "" {
				g.Skip("This UPI Baremetal cluster did not fulfill the prequiste of testing egressIP cases, skip the test!!")
			}
		}

	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-NonPreRelease-Longduration-Medium-47029-Low-47024-Any egress IP can only be assigned to one node only. Warning event will be triggered if applying EgressIP object but no EgressIP nodes. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("2 Create first egressip object \n")
		freeIPs := findFreeIPs(oc, egressNodes[0], 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:          "egressip-47029",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)

		exutil.By("3. Check warning event. \n")
		warnErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			warningEvent, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", "default").Output()
			if err != nil {
				e2e.Logf("Wait for waring event generated.%v", err)
				return false, nil
			}
			if !strings.Contains(warningEvent, "NoMatchingNodeFound") {
				e2e.Logf("Wait for waring event generated. ")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(warnErr, "Warning event doesn't conclude: NoMatchingNodeFound.")

		exutil.By("4 Apply EgressLabel Key to nodes. \n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)

		exutil.By("5. Check EgressIP assigned in the object.\n")
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1), "EgressIP object should get one applied item, but actually not.")

		exutil.By("6 Create second egressip object with same egressIP \n")
		egressip2 := egressIPResource1{
			name:          "egressip-47024",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip2.createEgressIPObject2(oc)
		defer egressip2.deleteEgressIPObject1(oc)

		exutil.By("7 Check the second egressIP object, no egressIP assigned  .\n")
		egressIPStatus, egressIPerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", egressip2.name, "-ojsonpath={.status.items}").Output()
		o.Expect(egressIPerr).NotTo(o.HaveOccurred())
		o.Expect(egressIPStatus).To(o.Equal(""))

		exutil.By("8. Edit the second egressIP object to another IP\n")
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/"+egressip2.name, "-p", "{\"spec\":{\"egressIPs\":[\""+freeIPs[1]+"\"]}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("9. Check egressIP assigned in the second object.\n")
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip2.name)
		o.Expect(len(egressIPMaps2)).Should(o.Equal(1), "EgressIP object should get one applied item, but actually not.")

	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-High-47021-lr-policy-list and snat should be updated correctly after remove pods. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")

		exutil.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		exutil.By("2 Apply EgressLabel Key to one node. \n")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")

		exutil.By("3. create new namespace\n")
		ns1 := oc.Namespace()

		exutil.By("4. Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("5. Create test pods and scale test pods to 10 \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=10", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		exutil.By("6. Create an egressip object\n")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47021",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue())

		exutil.By("6. Restart ovnkube-node pod which is on egress node\n")
		ovnPod := ovnkubeNodePod(oc, egressNode)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", ovnPod, "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("9. Scale test pods to 1 \n")
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=1", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podsErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			podsOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns1).Output()
			e2e.Logf(podsOutput)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Count(podsOutput, "test") == 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(podsErr, fmt.Sprintf("The pods were not scaled to the expected number!"))
		testPodName := getPodName(oc, ns1, "name=test-pods")
		_, testPodIPv4 := getPodIP(oc, ns1, testPodName[0])
		testPodNode, err := exutil.GetPodNodeName(oc, ns1, testPodName[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("test pod %s is on node %s", testPodName, testPodNode)

		exutil.By("11. Check lr-policy-list and snat in northdb. \n")
		ovnPod = ovnkubeNodePod(oc, testPodNode)
		o.Expect(ovnPod != "").Should(o.BeTrue())
		lspCmd := "ovn-nbctl lr-policy-list ovn_cluster_router | grep -v inport"
		checkLspErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			lspOutput, lspErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lspCmd)
			if lspErr != nil {
				e2e.Logf("%v,Waiting for lr-policy-list to be synced, try next ...,", lspErr)
				return false, nil
			}
			e2e.Logf(lspOutput)
			if strings.Contains(lspOutput, testPodIPv4) && strings.Count(lspOutput, "100 ") == 1 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkLspErr, fmt.Sprintf("lr-policy-list was not synced correctly!"))

		ovnPod = ovnkubeNodePod(oc, egressNode)
		snatCmd := "ovn-nbctl --format=csv --no-heading find nat external_ids:name=" + egressip1.name
		checkSnatErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			snatOutput, snatErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, snatCmd)
			if snatErr != nil {
				e2e.Logf("%v,Waiting for snat to be synced, try next ...,", snatErr)
				return false, nil
			}
			e2e.Logf(snatOutput)
			if strings.Contains(snatOutput, testPodIPv4) && strings.Count(snatOutput, egressip1.name) == 1 {
				e2e.Logf("The snat for egressip is as expected!")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(checkSnatErr, fmt.Sprintf("snat was not synced correctly!"))
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-Longduration-NonPreRelease-Medium-47208-The configured EgressIPs exceeds IP capacity. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name

		exutil.By("2 Apply EgressLabel Key to one node. \n")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("3 Get IP capacity of the node. \n")
		ipCapacity := getIPv4Capacity(oc, egressNode)
		o.Expect(ipCapacity != "").Should(o.BeTrue())
		ipCap, _ := strconv.Atoi(ipCapacity)
		if ipCap > 14 {
			g.Skip("This is not the general IP capacity, will skip it.")
		}
		exceedNum := ipCap + 1

		exutil.By("4 Create egressip objects \n")
		sub1 := getIfaddrFromNode(egressNode, oc)
		freeIPs := findUnUsedIPsOnNode(oc, egressNode, sub1, exceedNum)
		o.Expect(len(freeIPs) == exceedNum).Should(o.BeTrue())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("egressip", "--all").Execute()
		egressIPConfig := make([]egressIPResource1, exceedNum)
		for i := 0; i <= ipCap; i++ {
			iVar := strconv.Itoa(i)
			egressIPConfig[i] = egressIPResource1{
				name:          "egressip-47208-" + iVar,
				template:      egressIP2Template,
				egressIP1:     freeIPs[i],
				nsLabelKey:    "org",
				nsLabelValue:  "qe",
				podLabelKey:   "color",
				podLabelValue: "pink",
			}
			egressIPConfig[i].createEgressIPObject2(oc)
		}

		exutil.By("5 Check ipCapacity+1 number egressIP created,but one is not assigned egress node \n")
		egressIPErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			egressIPOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip").Output()
			e2e.Logf(egressIPOutput)
			if err != nil {
				e2e.Logf("Wait for egressip assigned.%v", err)
				return false, nil
			}
			if strings.Count(egressIPOutput, "egressip-47208") == exceedNum {
				e2e.Logf("The %v number egressIP object created.", exceedNum)
				if strings.Count(egressIPOutput, egressNode) == ipCap {
					e2e.Logf("The %v number egressIPs were assigned.", ipCap)
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(egressIPErr, fmt.Sprintf(" Error at getting EgressIPs or EgressIPs were not assigned corrently."))

		exutil.By("6. Check warning event. \n")
		warnErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			warningEvent, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", "default").Output()
			if err != nil {
				e2e.Logf("Wait for warning event generated.%v", err)
				return false, nil
			}
			if !strings.Contains(warningEvent, "NoMatchingNodeFound") {
				e2e.Logf("Expected warning message is not found, try again ")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(warnErr, fmt.Sprintf("Warning event doesn't conclude: NoMatchingNodeFound."))

	})

	// author: jechen@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-54045-EgressIP health check through monitoring port over GRPC on OCP OVN cluster. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		ipStackType := checkIPStackType(oc)
		if ipStackType != "ipv4single" {
			g.Skip("This case requires IPv4 cluster only")
		}

		exutil.By("1 check ovnkube-config configmap if egressip-node-healthcheck-port=9107 is in it \n")
		configmapName := "ovnkube-config"
		envString := " egressip-node-healthcheck-port=9107"
		cmCheckErr := checkEnvInConfigMap(oc, "openshift-ovn-kubernetes", configmapName, envString)
		o.Expect(cmCheckErr).NotTo(o.HaveOccurred())

		exutil.By("2 get leader OVNK control plane pod and ovnkube-node pods \n")
		readyErr := waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-control-plane")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-control-plane pods are not ready")
		OVNKCtrlPlaneLeadPodName := getOVNKMasterPod(oc)

		readyErr = waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-node pods are not ready")
		ovnkubeNodePods := getPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("3 Check each ovnkube-node pod's log that health check server is started on it \n")
		expectedString := "Starting Egress IP Health Server on "
		for _, ovnkubeNodePod := range ovnkubeNodePods {
			podLogs, LogErr := checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-controller", ovnkubeNodePod, "'egress ip'")
			o.Expect(LogErr).NotTo(o.HaveOccurred())
			o.Expect(podLogs).To(o.ContainSubstring(expectedString))
		}

		exutil.By("4 Get list of nodes, pick one as egressNode, apply EgressLabel Key to it \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		nodeOVNK8sMgmtIP := getOVNK8sNodeMgmtIPv4(oc, egressNode)

		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("5 Check leader OVNK control plane pod's log that health check connection has been made to the egressNode on port 9107 \n")
		expectedString = "Connected to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr := checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-cluster-manager", OVNKCtrlPlaneLeadPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))

		exutil.By("6. Create an egressip object, verify egressIP is assigned to the egressNode")
		freeIPs := findFreeIPs(oc, egressNode, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-54045",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "red",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("7. Add iptables on to block port 9107 on egressNode, verify from log of ovnkube-control-plane pod that the health check connection is closed.\n")
		defer exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-D", "INPUT", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		_, debugNodeErr := exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-I", "INPUT", "1", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		expectedString1 := "Closing connection with " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-cluster-manager", OVNKCtrlPlaneLeadPodName, "'"+expectedString1+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString1))
		expectedString2 := "Could not connect to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-cluster-manager", OVNKCtrlPlaneLeadPodName, "'"+expectedString2+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString2))

		exutil.By("8. Verify egressIP is not assigned after blocking iptable rule on port 9170 is added.\n")
		o.Eventually(func() bool {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			return len(egressIPMaps1) == 0
		}, "300s", "10s").Should(o.BeTrue(), "egressIP is not unassigned after blocking iptable rule on port 9170 is added!!")

		exutil.By("9. Delete the iptables rule, verify from log of ovnkube-control-plane pod that the health check connection is re-established.\n")
		_, debugNodeErr = exutil.DebugNodeWithChroot(oc, egressNode, "iptables", "-D", "INPUT", "-p", "tcp", "--destination-port", "9107", "-j", "DROP")
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		expectedString = "Connected to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-cluster-manager", OVNKCtrlPlaneLeadPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))

		exutil.By("10. Verify egressIP is re-applied after blocking iptable rule on port 9170 is deleted.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("11. Unlabel the egressNoe egressip-assignable, verify from log of ovnkube-control-plane pod that the health check connection is closed.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[0].Name, egressNodeLabel)
		expectedString = "Closing connection with " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"

		podLogs, LogErr = checkLogMessageInPod(oc, "openshift-ovn-kubernetes", "ovnkube-cluster-manager", OVNKCtrlPlaneLeadPodName, "'"+expectedString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedString))
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-High-Longduration-NonPreRelease-55030-After reboot egress node, lr-policy-list and snat should keep correct. [Disruptive]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		testPodFile := filepath.Join(buildPruningBaseDir, "testpod.yaml")

		exutil.By("1 Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}

		exutil.By("2 Apply EgressLabel Key to one node. \n")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[0], egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNodes[1], egressNodeLabel, "true")

		exutil.By("3. create new namespace\n")
		ns1 := oc.Namespace()

		exutil.By("4. Apply label to namespace\n")
		worker1 := nodeList.Items[0].Name
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("5. Create test pods and scale test pods to 5 \n")
		createResourceFromFile(oc, ns1, testPodFile)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("replicationcontroller/test-rc", "-n", ns1, "-p", "{\"spec\":{\"template\":{\"spec\":{\"nodeName\":\""+worker1+"\"}}}}", "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=0", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("rc", "test-rc", "--replicas=5", "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		exutil.By("6. Create an egressip object\n")
		ipStackType := checkIPStackType(oc)
		var freeIPs []string
		lspExpNum := 5
		switch ipStackType {
		case "ipv4single":
			freeIPs = findFreeIPs(oc, egressNodes[0], 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		case "dualstack":
			//Get one IPv6 address for second node
			freeIPv6s := findFreeIPv6s(oc, egressNodes[1], 1)
			o.Expect(len(freeIPv6s)).Should(o.Equal(1))
			//Get one IPv4 address
			freeIPs = findFreeIPs(oc, egressNodes[0], 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPs = append(freeIPs, freeIPv6s[0])
			lspExpNum = 10
		case "ipv6single":
			freeIPs = findFreeIPv6s(oc, egressNodes[0], 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		}
		egressip1 := egressIPResource1{
			name:      "egressip-55030",
			template:  egressIP1Template,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)

		exutil.By("5.Reboot egress node.\n")
		defer checkNodeStatus(oc, egressNodes[0], "Ready")
		rebootNode(oc, egressNodes[0])
		checkNodeStatus(oc, egressNodes[0], "NotReady")
		checkNodeStatus(oc, egressNodes[0], "Ready")
		err = waitForPodWithLabelReady(oc, ns1, "name=test-pods")
		exutil.AssertWaitPollNoErr(err, "this pod with label name=test-pods not ready")

		exutil.By("6. Check lr-policy-list and snat in northdb. \n")
		ovnPod := ovnkubeNodePod(oc, worker1)
		o.Expect(ovnPod).ShouldNot(o.Equal(""))
		lspCmd := "ovn-nbctl lr-policy-list ovn_cluster_router | grep -v inport"
		o.Eventually(func() bool {
			output, cmdErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, lspCmd)
			return cmdErr == nil && strings.Count(output, "100 ") == lspExpNum
		}, "120s", "10s").Should(o.BeTrue(), "The command check result in ovndb is not expected!")
		ovnPod = ovnkubeNodePod(oc, egressNodes[0])
		snatCmd := "ovn-nbctl --format=csv --no-heading find nat external_ids:name=" + egressip1.name
		o.Eventually(func() bool {
			output, cmdErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, snatCmd)
			return cmdErr == nil && strings.Count(output, egressip1.name) == 5
		}, "120s", "10s").Should(o.BeTrue(), "The command check result in ovndb is not expected!")
	})

	// author: huirwang@redhat.com
	g.It("Author:huirwang-High-55632-After enable egress node, egress node shouldn't generate broadcast ARP for service IPs. [Serial]", func() {
		e2e.Logf("This case is from customer bug: https://bugzilla.redhat.com/show_bug.cgi?id=2052975")
		exutil.By("1 Get list of nodes \n")
		nodeList, nodeErr := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(nodeErr).NotTo(o.HaveOccurred())
		egessNode := nodeList.Items[0].Name

		exutil.By("2 Apply EgressLabel Key to one node. \n")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel, "true")

		exutil.By("3. Check no ARP broadcast for service IPs\n")
		e2e.Logf("Trying to get physical interface on the node,%s", egessNode)
		phyInf, nicError := getSnifPhyInf(oc, egessNode)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 10 -nni %s arp", phyInf)
		outPut, _ := exutil.DebugNode(oc, egessNode, "bash", "-c", tcpdumpCmd)
		o.Expect(outPut).NotTo(o.ContainSubstring("172.30"), fmt.Sprintf("The output of tcpdump is %s", outPut))
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-High-49161-High-43465-Service IP should be reachable when egressIP set to the namespace. [Serial]", func() {
		e2e.Logf("This case is from customer bug: https://bugzilla.redhat.com/show_bug.cgi?id=2014202")
		var (
			buildPruningBaseDir    = exutil.FixturePath("testdata", "networking")
			pingPodTemplate        = filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
			genericServiceTemplate = filepath.Join(buildPruningBaseDir, "service-generic-template.yaml")
			egressIPTemplate       = filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")
		)

		exutil.By(" Get list of nodes \n")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Apply EgressLabel Key to one node. \n")
		egessNode := nodeList.Items[0].Name
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel, "true")
		ipStackType := checkIPStackType(oc)
		// For dual stack cluster, it needs two nodes holding IPv4 and IPv6 seperately.
		if ipStackType == "dualstack" {
			if len(nodeList.Items) < 2 {
				g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
			}
			defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, egressNodeLabel)
			e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeList.Items[1].Name, egressNodeLabel, "true")
		}

		exutil.By("Get namespace\n")
		ns1 := oc.Namespace()

		exutil.By("create 1st hello pod in ns1")
		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, ns1, pod1.name)

		exutil.By("Create a test service which is in front of the above pods")
		svc := genericServiceResource{
			servicename:           "test-service",
			namespace:             ns1,
			protocol:              "TCP",
			selector:              "hello-pod",
			serviceType:           "ClusterIP",
			ipFamilyPolicy:        "",
			internalTrafficPolicy: "Cluster",
			externalTrafficPolicy: "", //This no value parameter will be ignored
			template:              genericServiceTemplate,
		}
		if ipStackType == "dualstack" {
			svc.ipFamilyPolicy = "PreferDualStack"
		} else {
			svc.ipFamilyPolicy = "SingleStack"
		}
		svc.createServiceFromParams(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", svc.servicename, "-n", svc.namespace).Execute()

		exutil.By("Apply label to namespace\n")
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "name=test").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6. Create an egressip object\n")
		var freeIPs []string
		switch ipStackType {
		case "ipv4single":
			freeIPs = findFreeIPs(oc, egessNode, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		case "dualstack":
			//Get one IPv6 address for second node
			freeIPv6s := findFreeIPv6s(oc, nodeList.Items[1].Name, 1)
			o.Expect(len(freeIPv6s)).Should(o.Equal(1))
			//Get one IPv4 address
			freeIPs = findFreeIPs(oc, egessNode, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
			freeIPs = append(freeIPs, freeIPv6s[0])
		case "ipv6single":
			freeIPs = findFreeIPv6s(oc, egessNode, 2)
			o.Expect(len(freeIPs)).Should(o.Equal(2))
		default:
			e2e.Logf("Get ipStackType as %s", ipStackType)
			g.Skip("Skip for not supported IP stack type!! ")
		}

		egressip1 := egressIPResource1{
			name:      "egressip-49161",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject1(oc)

		//Get one non-egress node
		masterNode, errNode := exutil.GetFirstMasterNode(oc)
		o.Expect(errNode).NotTo(o.HaveOccurred())
		exutil.By("verify egressIP object was applied to egress node.")
		if ipStackType == "dualstack" {
			verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 2)
			// This is to cover case OCP-43465
			msg, errOutput := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressip", egressip1.name, "-o=jsonpath={.status.items[*]}").Output()
			o.Expect(errOutput).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(msg, freeIPs[0]) && strings.Contains(msg, freeIPs[1])).To(o.BeTrue())
		} else {
			verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 1)
		}

		exutil.By("curl from egress node to service:port")
		CurlNode2SvcPass(oc, nodeList.Items[0].Name, ns1, svc.servicename)
		exutil.By("curl from non egress node to service:port")
		CurlNode2SvcPass(oc, masterNode, ns1, svc.servicename)
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:huirwang-High-61344-EgressIP was migrated to correct workers after deleting machine it was assigned. [Disruptive]", func() {
		//This is from customer bug: https://bugzilla.redhat.com/show_bug.cgi?id=2079012
		platform := exutil.CheckPlatform(oc)
		if strings.Contains(platform, "baremetal") || strings.Contains(platform, "none") {
			g.Skip("Skip for non-supported auto scaling machineset platforms!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("Create a new machineset with 2 nodes")
		exutil.SkipConditionally(oc)
		machinesetName := "machineset-61344"
		ms := exutil.MachineSetDescription{machinesetName, 2}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)
		exutil.WaitForMachinesRunning(oc, 2, machinesetName)
		machineName := exutil.GetMachineNamesFromMachineSet(oc, machinesetName)
		nodeName0 := exutil.GetNodeNameFromMachine(oc, machineName[0])
		nodeName1 := exutil.GetNodeNameFromMachine(oc, machineName[1])

		exutil.By("Apply EgressLabel Key to one node. \n")
		// No defer here, as this node will be deleted explicitly in the following step.
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeName0, egressNodeLabel, "true")

		exutil.By("Create an egressip object\n")
		freeIPs := findFreeIPs(oc, nodeName0, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-61344",
			template:      egressIP1Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 1)

		exutil.By("Apply egess label to another worker node.\n")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, nodeName1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, nodeName1, egressNodeLabel, "true")

		exutil.By("Remove the first egress node.\n")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("machines.machine.openshift.io", machineName[0], "-n", "openshift-machine-api").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)

		exutil.By("Verify egressIP was moved to second egress node.\n")
		o.Eventually(func() bool {
			egressIPMaps := getAssignedEIPInEIPObject(oc, egressip1.name)
			return len(egressIPMaps) == 1 && egressIPMaps[0]["node"] == nodeName1
		}, "120s", "10s").Should(o.BeTrue(), "egressIP was not migrated to correct workers!!")
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-Critical-64293-EgressIP should not break access from a pod with EgressIP to other host networked pods on different nodes. [Disruptive]", func() {
		//This is from customer bug: https://bugzilla.redhat.com/show_bug.cgi?id=2070929
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP1Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")
		pingPodNodeTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-specific-node-template.yaml")

		exutil.By("Verify there are two more worker nodes in the cluster.")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(nodeList.Items) < 2 {
			g.Skip("This case requires 2 nodes, but the cluster has less than two nodes")
		}

		exutil.By("Get namespace")
		ns := oc.Namespace()

		exutil.By("Apply EgressLabel Key to one node. \n")
		egessNode := nodeList.Items[0].Name
		nonEgressNode := nodeList.Items[1].Name
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egessNode, egressNodeLabel, "true")

		exutil.By("6. Create an egressip object\n")
		ipStackType := checkIPStackType(oc)
		var freeIPs []string
		if ipStackType == "ipv6single" {
			freeIPs = findFreeIPv6s(oc, egessNode, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
		} else {
			freeIPs = findFreeIPs(oc, egessNode, 1)
			o.Expect(len(freeIPs)).Should(o.Equal(1))
		}

		exutil.By("Create an egressip object\n")
		egressip1 := egressIPResource1{
			name:          "egressip-64293-1",
			template:      egressIP1Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		verifyExpectedEIPNumInEIPObject(oc, egressip1.name, 1)

		exutil.By("Create a test pod on non-egress node\n")
		pod1 := pingPodResourceNode{
			name:      "hello-pod1",
			namespace: ns,
			nodename:  nonEgressNode,
			template:  pingPodNodeTemplate,
		}
		pod1.createPingPodNode(oc)
		waitPodReady(oc, ns, pod1.name)

		exutil.By("patch label to namespace and pod")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer exutil.LabelPod(oc, ns, pod1.name, "color-")
		err = exutil.LabelPod(oc, ns, pod1.name, "color=pink")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Should be able to access the api service\n")
		//  The backend pod of api server are hostnetwork pod and located on master nodes.  That are different nodes from egress nodes as egress nodes are worker nodes here.
		svcIP, _ := getSvcIP(oc, "default", "kubernetes")
		curlCmd := fmt.Sprintf("curl -s %s --connect-timeout 5", net.JoinHostPort(svcIP, "443"))
		_, err = e2eoutput.RunHostCmd(ns, pod1.name, curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

	})
})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP", func() {
	//Cases in this function, do not need curl ip-echo
	defer g.GinkgoRecover()

	var (
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := exutil.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp") || strings.Contains(platform, "openstack") || strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "azure") || strings.Contains(platform, "nutanix")
		if !acceptedPlatform || !strings.Contains(networkType, "ovn") {
			g.Skip("Test cases should be run on AWS/GCP/Azure/Openstack/Vsphere/Baremetal/Nutanix cluster with ovn network plugin, skip for other platforms or other network plugin!!")
		}
	})

	// author: huirwang@redhat.com
	g.It("ConnectedOnly-Author:huirwang-High-47163-High-47026-Deleting EgressIP object and recreating it works,EgressIP was removed after delete egressIP object. [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIPTemplate := filepath.Join(buildPruningBaseDir, "egressip-config1-template.yaml")

		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}

		exutil.By("Get the temporary namespace")
		ns := oc.Namespace()

		exutil.By("Get schedulable worker nodes")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, "tcpdump", "true")

		exutil.By("Create tcpdump sniffer Daemonset.")
		primaryInf, infErr := getSnifPhyInf(oc, egressNode)
		o.Expect(infErr).NotTo(o.HaveOccurred())
		dstHost := nslookDomainName("ifconfig.me")
		defer deleteTcpdumpDS(oc, "tcpdump-47163", ns)
		tcpdumpDS, snifErr := createSnifferDaemonset(oc, ns, "tcpdump-47163", "tcpdump", "true", dstHost, primaryInf, 80)
		o.Expect(snifErr).NotTo(o.HaveOccurred())

		exutil.By("Apply EgressLabel Key for this test on one node.")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode, egressNodeLabel)

		exutil.By("Apply label to namespace")
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name=test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "name-").Output()

		exutil.By("Create an egressip object")
		freeIPs := findFreeIPs(oc, egressNode, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))
		egressip1 := egressIPResource1{
			name:      "egressip-47163",
			template:  egressIPTemplate,
			egressIP1: freeIPs[0],
			egressIP2: freeIPs[1],
		}
		egressip1.createEgressIPObject1(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1) == 1).Should(o.BeTrue(), fmt.Sprintf("The egressIP was not assigned correctly!"))

		exutil.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: ns,
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		defer pod1.deletePingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("Check source IP is EgressIP")
		egressErr := verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps1[0]["egressIP"], dstHost, ns, tcpdumpDS.name, true)
		o.Expect(egressErr).NotTo(o.HaveOccurred())

		exutil.By("Deleting egressip object")
		egressip1.deleteEgressIPObject1(oc)
		waitCloudPrivateIPconfigUpdate(oc, egressIPMaps1[0]["egressIP"], false)
		egressipErr := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			randomStr, url := getRequestURL(dstHost)
			_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, url)
			if checkMatchedIPs(oc, ns, tcpdumpDS.name, randomStr, egressIPMaps1[0]["egressIP"], false) != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to clear egressip:%s", egressipErr))

		exutil.By("Recreating egressip object")
		egressip1.createEgressIPObject1(oc)
		egressIPMaps2 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps2) == 1).Should(o.BeTrue(), fmt.Sprintf("The egressIP was not assigned correctly!"))

		exutil.By("Check source IP is EgressIP")
		egressErr = verifyEgressIPinTCPDump(oc, pod1.name, pod1.namespace, egressIPMaps2[0]["egressIP"], dstHost, ns, tcpdumpDS.name, true)
		o.Expect(egressErr).NotTo(o.HaveOccurred())

		exutil.By("Deleting EgressIP object and recreating it works!!! ")

	})

	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-54647-No stale or duplicated SNAT on gateway router after egressIP failover to new egress node. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		statefulSetPodTemplate := filepath.Join(buildPruningBaseDir, "statefulset-hello.yaml")
		completedPodTemplate := filepath.Join(buildPruningBaseDir, "countdown-job-completed-pod.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1. Get list of nodes, get two worker nodes that have same subnet, use them as egress nodes\n")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		exutil.By("2. Apply EgressLabel Key to two egress nodes.")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")

		exutil.By("3. Create an egressip object")
		freeIPs := findFreeIPs(oc, egressNode1, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))

		egressip1 := egressIPResource1{
			name:          "egressip-54647",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "purple",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		// The egress node that currently hosts the egressIP will be the node to be rebooted to create egressIP failover
		nodeToBeRebooted := egressIPMaps1[0]["node"]
		e2e.Logf("egressNode to be rebooted is:%v", nodeToBeRebooted)

		var hostLeft []string
		for i, v := range egressNodes {
			if v == nodeToBeRebooted {
				hostLeft = append(egressNodes[:i], egressNodes[i+1:]...)
				break
			}
		}
		e2e.Logf("\n Get the egressNode that did not host egressIP address previously: %v\n", hostLeft)

		exutil.By("4. create a namespace, apply label to the namespace")
		ns1 := oc.Namespace()
		exutil.SetNamespacePrivileged(oc, ns1)

		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org-").Execute()
		nsLabelErr := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "org=qe").Execute()
		o.Expect(nsLabelErr).NotTo(o.HaveOccurred())

		exutil.By("5.1 Create a statefulSet Hello pod in the namespace, apply pod label to it. ")
		createResourceFromFile(oc, ns1, statefulSetPodTemplate)
		podErr := waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "The statefulSet pod is not ready")
		statefulSetPodName := getPodName(oc, ns1, "app=hello")

		defer exutil.LabelPod(oc, ns1, statefulSetPodName[0], "color-")
		podLabelErr := exutil.LabelPod(oc, ns1, statefulSetPodName[0], "color=purple")
		exutil.AssertWaitPollNoErr(podLabelErr, "Was not able to apply pod label")

		helloPodIPv4, _ := getPodIP(oc, ns1, statefulSetPodName[0])
		e2e.Logf("Pod's IP for the statefulSet Hello Pod is:%v", helloPodIPv4)

		exutil.By("5.2 Create a completed pod in the namespace, apply pod label to it. ")
		createResourceFromFile(oc, ns1, completedPodTemplate)
		completedPodName := getPodName(oc, ns1, "job-name=countdown")
		waitPodReady(oc, ns1, completedPodName[0])

		defer exutil.LabelPod(oc, ns1, completedPodName[0], "color-")
		podLabelErr = exutil.LabelPod(oc, ns1, completedPodName[0], "color=purple")
		exutil.AssertWaitPollNoErr(podLabelErr, "Was not able to apply pod label")

		completedPodIPv4, _ := getPodIP(oc, ns1, completedPodName[0])
		e2e.Logf("Pod's IP for the completed countdown pod is:%v", completedPodIPv4)

		exutil.By("6. Check SNATs of stateful pod and completed pod on the egressNode before rebooting it.\n")
		routerIDOfEgressNode1, routerErr := getRouterID(oc, nodeToBeRebooted)
		o.Expect(routerErr).NotTo(o.HaveOccurred())
		e2e.Logf("routerID Of node to be rebooted is:%v", routerIDOfEgressNode1)
		snatIP, snatErr := getSNATofEgressIP(oc, routerIDOfEgressNode1, egressNode1, freeIPs[0])
		o.Expect(snatErr).NotTo(o.HaveOccurred())
		e2e.Logf("the SNAT IP for the egressIP is:%v", snatIP)
		o.Expect(snatIP).Should(o.Equal(helloPodIPv4))
		o.Expect(snatIP).ShouldNot(o.Equal(completedPodIPv4))

		exutil.By("7. Reboot egress node.\n")
		defer checkNodeStatus(oc, egressIPMaps1[0]["node"], "Ready")
		rebootNode(oc, egressIPMaps1[0]["node"])
		checkNodeStatus(oc, egressIPMaps1[0]["node"], "NotReady")

		exutil.By("8. As soon as the rebooted node is in NotReady state, delete the statefulSet pod to force it be recreated while the node is rebooting.\n")
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", statefulSetPodName[0], "-n", ns1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		podErr = waitForPodWithLabelReady(oc, ns1, "app=hello")
		exutil.AssertWaitPollNoErr(podErr, "this pod with label app=hello not ready")

		// Re-apply label for pod as pod is re-created with same pod name
		defer exutil.LabelPod(oc, ns1, statefulSetPodName[0], "color-")
		podLabelErr = exutil.LabelPod(oc, ns1, statefulSetPodName[0], "color=purple")
		exutil.AssertWaitPollNoErr(podLabelErr, "Was not able to apply pod label")

		newHelloPodIPv4, _ := getPodIP(oc, ns1, statefulSetPodName[0])
		e2e.Logf("Pod's IP for the newly created Hello Pod is:%v", newHelloPodIPv4)

		// get completed pod name again, relabel it, get its IP address
		newCompletedPodName := getPodName(oc, ns1, "job-name=countdown")
		waitPodReady(oc, ns1, newCompletedPodName[0])
		defer exutil.LabelPod(oc, ns1, newCompletedPodName[0], "color-")
		podLabelErr = exutil.LabelPod(oc, ns1, newCompletedPodName[0], "color=purple")
		exutil.AssertWaitPollNoErr(podLabelErr, "Was not able to apply pod label")
		newCompletedPodIPv4, _ := getPodIP(oc, ns1, newCompletedPodName[0])
		e2e.Logf("Pod's IP for the new completed countdown pod is:%v", newCompletedPodIPv4)

		exutil.By("9. Check egress node in egress object again, egressIP should fail to the second egressNode.\n")
		egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
		newEgressIPHostNode := egressIPMaps1[0]["node"]
		e2e.Logf("new egressNode that hosts the egressIP is:%v", newEgressIPHostNode)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))
		o.Expect(egressIPMaps1[0]["node"]).Should(o.Equal(hostLeft[0]))

		exutil.By("10. Check SNAT on the second egressNode\n")
		routerIDOfEgressNode2, routerErr := getRouterID(oc, hostLeft[0])
		o.Expect(routerErr).NotTo(o.HaveOccurred())
		snatIP, snatErr = getSNATofEgressIP(oc, routerIDOfEgressNode2, egressNode2, freeIPs[0])
		o.Expect(snatErr).NotTo(o.HaveOccurred())

		e2e.Logf("After egressIP failover, the SNAT IP for the egressIP on second router is:%v", snatIP)
		exutil.By("10.1 There should be the IP of the newly created statefulState hello pod, not the IP of old hello pod.\n")
		o.Expect(snatIP).Should(o.Equal(newHelloPodIPv4))
		o.Expect(snatIP).ShouldNot(o.Equal(helloPodIPv4))

		exutil.By("10.2 There should be no SNAT for old or new completed pod's IP.\n")
		o.Expect(snatIP).ShouldNot(o.Equal(newEgressIPHostNode)) //there should be no SNAT for completed pod's old or new IP address
		o.Expect(snatIP).ShouldNot(o.Equal(completedPodIPv4))    //there should be no SNAT for completed pod's old or new IP address

		// Make sure the rebooted node is back to Ready state
		checkNodeStatus(oc, egressIPMaps1[0]["node"], "Ready")

		exutil.By("11. Check SNAT on all other unassigned nodes, it should be no stale NAT on all other unassigned nodes.\n")
		var unassignedNodes []string
		for i := 0; i < len(nodeList.Items); i++ {
			if nodeList.Items[i].Name != newEgressIPHostNode {
				unassignedNodes = append(unassignedNodes, nodeList.Items[i].Name)
			}
		}
		e2e.Logf("unassigned nodes are:%v", unassignedNodes)

		for i := 0; i < len(unassignedNodes); i++ {
			routerID, routerErr := getRouterID(oc, unassignedNodes[i])
			o.Expect(routerErr).NotTo(o.HaveOccurred())
			snatIP, snatErr = getSNATofEgressIP(oc, routerID, unassignedNodes[i], freeIPs[0])
			o.Expect(snatErr).To(o.HaveOccurred())
			o.Expect(snatIP).Should(o.Equal(""))
		}
	})

	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:jechen-High-67091-Egressip status is synced with cloudprivateipconfig and egressip is assigned correctly after OVNK restart. [Disruptive]", func() {

		// This is for OCPBUGS-12747
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		// cloudprivateipconfig is a resource only available on cloud platforms like AWS, GCP and Azure that egressIP is supported, skip other platforms
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp", "azure")

		exutil.By("1. Get list of nodes, get two worker nodes that have same subnet, use them as egress nodes\n")
		var egressNode1, egressNode2 string
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		ok, egressNodes := getTwoNodesSameSubnet(oc, nodeList)
		if !ok || egressNodes == nil || len(egressNodes) < 2 {
			g.Skip("The prerequirement was not fullfilled, skip the case!!")
		}
		egressNode1 = egressNodes[0]
		egressNode2 = egressNodes[1]

		exutil.By("2. Apply EgressLabel Key to two egress nodes.\n")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode2, egressNodeLabel, "true")

		exutil.By("3. Get two unused IP addresses from the egress node.\n")
		freeIPs := findFreeIPs(oc, egressNode1, 2)
		o.Expect(len(freeIPs)).Should(o.Equal(2))

		exutil.By("4. Create an egressip object, verify egressip is assigned to an egress node.\n")
		egressip1 := egressIPResource1{
			name:          "egressip-67091",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "purple",
		}
		defer egressip1.deleteEgressIPObject1(oc)
		egressip1.createEgressIPObject2(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("5. Verify egressIP is in cloudprivateipconfig.\n")
		waitCloudPrivateIPconfigUpdate(oc, freeIPs[0], true)

		exutil.By("6. Restart OVNK, before OVNK is back up, delete cloudprivateipconfig and replace egressip in egressip object to another valid unused IP address\n")
		//Restart OVNK by deleting all ovnkube-node pods
		defer waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		delPodErr := oc.AsAdmin().Run("delete").Args("pod", "-l", "app=ovnkube-node", "-n", "openshift-ovn-kubernetes").Execute()
		o.Expect(delPodErr).NotTo(o.HaveOccurred())

		delCloudPrivateIPConfigErr := oc.AsAdmin().Run("delete").Args("cloudprivateipconfig", egressIPMaps1[0]["egressIP"]).Execute()
		o.Expect(delCloudPrivateIPConfigErr).NotTo(o.HaveOccurred())

		// Update the egressip address in the egressip object with another unused ip address
		patchErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("egressip/"+egressip1.name, "-p", "{\"spec\":{\"egressIPs\":[\""+freeIPs[1]+"\"]}}", "--type=merge").Execute()
		o.Expect(patchErr).NotTo(o.HaveOccurred())

		exutil.By("7. Wait for ovnkube-node back up.\n")
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("8. Verify cloudprivateipconfig is updated to new egressip address.\n")
		waitCloudPrivateIPconfigUpdate(oc, freeIPs[1], true)

		exutil.By("9. Verify egressIP object is updated with new egressIP address, and egressIP is assigned to an egressNode.\n")
		o.Eventually(func() bool {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			return len(egressIPMaps1) == 1 && egressIPMaps1[0]["egressIP"] == freeIPs[1]
		}, "300s", "10s").Should(o.BeTrue(), "egressIP was not updated to new ip address, or egressip was not assigned to an egressNode!!")
		currenAssignedEgressNode := egressIPMaps1[0]["node"]

		exutil.By("10. Unlabel current assigned egress node, verify that egressIP fails over to the other egressNode.\n")
		e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, currenAssignedEgressNode, egressNodeLabel)
		var newAssignedEgressNode string
		if currenAssignedEgressNode == egressNode1 {
			newAssignedEgressNode = egressNode2
		} else if currenAssignedEgressNode == egressNode2 {
			newAssignedEgressNode = egressNode1
		}
		o.Eventually(func() bool {
			egressIPMaps1 = getAssignedEIPInEIPObject(oc, egressip1.name)
			return len(egressIPMaps1) == 1 && egressIPMaps1[0]["node"] == newAssignedEgressNode
		}, "300s", "10s").Should(o.BeTrue(), "egressIP was not migrated to second egress node after unlabel first egress node!!")
	})

})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP on hypershift", func() {
	defer g.GinkgoRecover()

	var (
		oc                                                          = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		egressNodeLabel                                             = "k8s.ovn.org/egress-assignable"
		hostedClusterName, hostedClusterKubeconfig, hostedclusterNS string
	)

	g.BeforeEach(func() {
		hostedClusterName, hostedClusterKubeconfig, hostedclusterNS = exutil.ValidHypershiftAndGetGuestKubeConf(oc)
		oc.SetGuestKubeconf(hostedClusterKubeconfig)

	})
	g.It("ROSA-OSD_CCS-HyperShiftMGMT-NonPreRelease-Longduration-ConnectedOnly-Author:jechen-High-54741-EgressIP health check through monitoring port over GRPC on hypershift cluster. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("1. Check ovnkube-config configmap in hypershift mgmt NS if egressip-node-healthcheck-port=9107 is in it \n")

		configmapName := "ovnkube-config"
		envString := " egressip-node-healthcheck-port=9107"
		hyperShiftMgmtNS := hostedclusterNS + "-" + hostedClusterName
		cmCheckErr := checkEnvInConfigMap(oc, hyperShiftMgmtNS, configmapName, envString)
		o.Expect(cmCheckErr).NotTo(o.HaveOccurred())

		exutil.By("2. Check if ovnkube-control-plane is ready on hypershift cluster \n")
		readyErr := waitForPodWithLabelReady(oc, hyperShiftMgmtNS, "app=ovnkube-control-plane")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-control-plane pods are not ready on the hypershift cluster")

		leaderOVNKCtrlPlanePodName := getOVNKCtrlPlanePodOnHostedCluster(oc, "openshift-ovn-kubernetes", "ovn-kubernetes-master", hyperShiftMgmtNS)
		e2e.Logf("\n\n leaderOVNKCtrlPlanePodName for the hosted cluster is: %s\n\n", leaderOVNKCtrlPlanePodName)

		readyErr = waitForPodWithLabelReadyOnHostedCluster(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		exutil.AssertWaitPollNoErr(readyErr, "ovnkube-node pods are not ready on hosted cluster")
		ovnkubeNodePods := getPodNameOnHostedCluster(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")

		exutil.By("3. Get list of scheduleable nodes on hosted cluster, pick one as egressNode, apply EgressLabel Key to it \n")
		scheduleableNodes, err := getReadySchedulableNodesOnHostedCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		// Pick first scheduleable node as egressNode
		egressNode := scheduleableNodes[0]

		defer exutil.DeleteLabelFromNode(oc.AsAdmin().AsGuestKubeconf(), egressNode, egressNodeLabel)
		labelValue := ""
		exutil.AddLabelToNode(oc.AsAdmin().AsGuestKubeconf(), egressNode, egressNodeLabel, labelValue)

		exutil.By("4. Check leader ovnkube-control-plane pod's log that health check connection has been made to the egressNode on port 9107 \n")
		// get the OVNK8s managementIP for the egressNode on hosted cluster
		nodeOVNK8sMgmtIP := getOVNK8sNodeMgmtIPv4OnHostedCluster(oc, egressNode)
		e2e.Logf("\n\n OVNK8s managementIP of the egressNode on hosted cluster is: %s\n\n", nodeOVNK8sMgmtIP)

		expectedConnectString := "Connected to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr := checkLogMessageInPod(oc, hyperShiftMgmtNS, "ovnkube-control-plane", leaderOVNKCtrlPlanePodName, "'"+expectedConnectString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedConnectString))

		exutil.By("5. Check each ovnkube-node pod's log on hosted cluster that health check server is started on it \n")
		expectedSeverStartString := "Starting Egress IP Health Server on "
		for _, ovnkubeNodePod := range ovnkubeNodePods {
			podLogs, LogErr := checkLogMessageInPodOnHostedCluster(oc, "openshift-ovn-kubernetes", "ovnkube-controller", ovnkubeNodePod, "'egress ip'")
			o.Expect(LogErr).NotTo(o.HaveOccurred())
			o.Expect(podLogs).To(o.ContainSubstring(expectedSeverStartString))
		}

		exutil.By("6. Create an egressip object, verify egressIP is assigned to the egressNode")
		freeIPs := findFreeIPs(oc.AsAdmin().AsGuestKubeconf(), egressNode, 1)
		o.Expect(len(freeIPs)).Should(o.Equal(1))
		egressip1 := egressIPResource1{
			name:          "egressip-54741",
			template:      egressIP2Template,
			egressIP1:     freeIPs[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "red",
		}
		defer egressip1.deleteEgressIPObject1(oc.AsAdmin().AsGuestKubeconf())
		egressip1.createEgressIPObject2(oc.AsAdmin().AsGuestKubeconf())
		egressIPMaps1 := getAssignedEIPInEIPObject(oc.AsAdmin().AsGuestKubeconf(), egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("7. Add iptables on to block port 9107 on egressNode, verify from log of ovnkube-control-plane pod that the health check connection is closed.\n")
		delCmdOptions := []string{"iptables", "-D", "INPUT", "-p", "tcp", "--destination-port", "9107", "-j", "DROP"}
		addCmdOptions := []string{"iptables", "-I", "INPUT", "1", "-p", "tcp", "--destination-port", "9107", "-j", "DROP"}
		defer execCmdOnDebugNodeOfHostedCluster(oc, egressNode, delCmdOptions)
		debugNodeErr := execCmdOnDebugNodeOfHostedCluster(oc, egressNode, addCmdOptions)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		expectedCloseConnectString := "Closing connection with " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, hyperShiftMgmtNS, "ovnkube-control-plane", leaderOVNKCtrlPlanePodName, "'"+expectedCloseConnectString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedCloseConnectString))
		expectedCouldNotConnectString := "Could not connect to " + egressNode + " (" + nodeOVNK8sMgmtIP + ":9107)"
		podLogs, LogErr = checkLogMessageInPod(oc, hyperShiftMgmtNS, "ovnkube-control-plane", leaderOVNKCtrlPlanePodName, "'"+expectedCouldNotConnectString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedCouldNotConnectString))

		exutil.By("8. Verify egressIP is not in cloudprivateipconfig after blocking iptable rule on port 9170 is added.\n")
		waitCloudPrivateIPconfigUpdate(oc, freeIPs[0], false)

		exutil.By("9. Delete the iptables rule, verify from log of lead ovnkube-control-plane pod that the health check connection is re-established.\n")
		debugNodeErr = execCmdOnDebugNodeOfHostedCluster(oc, egressNode, delCmdOptions)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())

		podLogs, LogErr = checkLogMessageInPod(oc, hyperShiftMgmtNS, "ovnkube-control-plane", leaderOVNKCtrlPlanePodName, "'"+expectedConnectString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedConnectString))

		exutil.By("10. Verify egressIP is re-applied after blocking iptable rule on port 9170 is deleted.\n")
		egressIPMaps := getAssignedEIPInEIPObject(oc.AsAdmin().AsGuestKubeconf(), egressip1.name)
		o.Expect(len(egressIPMaps)).Should(o.Equal(1))

		exutil.By("11. Unlabel the egressNoe egressip-assignable, verify from log of lead ovnkube-control-plane pod that the health check connection is closed.\n")
		exutil.DeleteLabelFromNode(oc.AsAdmin().AsGuestKubeconf(), egressNode, egressNodeLabel)

		podLogs, LogErr = checkLogMessageInPod(oc, hyperShiftMgmtNS, "ovnkube-control-plane", leaderOVNKCtrlPlanePodName, "'"+expectedCloseConnectString+"'"+"| tail -1")
		o.Expect(LogErr).NotTo(o.HaveOccurred())
		o.Expect(podLogs).To(o.ContainSubstring(expectedCloseConnectString))
	})

})

var _ = g.Describe("[sig-networking] SDN OVN EgressIP IPv6", func() {
	defer g.GinkgoRecover()

	var (
		oc              = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
		egressNodeLabel = "k8s.ovn.org/egress-assignable"
		rduBastionHost  = "2620:52:0:800:3673:5aff:fe99:92f0"
	)

	g.BeforeEach(func() {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("routes", "console", "-n", "openshift-console").Output()
		if err != nil || !(strings.Contains(msg, "sriov.openshift-qe.sdn.com") || strings.Contains(msg, "offload.openshift-qe.sdn.com")) {
			g.Skip("This case will only run on rdu1 or rdu2 dual stack cluster. , skip for other envrionment!!!")
		}
		if strings.Contains(msg, "offload.openshift-qe.sdn.com") {
			rduBastionHost = "2620:52:0:800:3673:5aff:fe98:d2d0"
		}
	})

	// author: huirwang@redhat.com
	g.It("NonHyperShiftHOST-Author:huirwang-Medium-43466-EgressIP works well with ipv6 address. [Serial]", func() {
		ipStackType := checkIPStackType(oc)
		//We already have many egressIP cases cover ipv4 addresses on both ipv4 and dualstack clusters,so this case focuses on dualstack cluster for ipv6 addresses.
		if ipStackType != "dualstack" {
			g.Skip("Current env is not dualsatck cluster, skip this test!!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod-template.yaml")
		egressIP2Template := filepath.Join(buildPruningBaseDir, "egressip-config2-template.yaml")

		exutil.By("create new namespace")
		ns := oc.Namespace()

		exutil.By("Label EgressIP node")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode1 := nodeList.Items[0].Name

		exutil.By("Apply EgressLabel Key for this test on one node.")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel)
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, egressNode1, egressNodeLabel, "true")

		exutil.By("Apply label to namespace")
		defer oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org-").Output()
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns, "org=qe").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create an egressip object")
		freeIPv6s := findFreeIPv6s(oc, egressNode1, 1)
		o.Expect(len(freeIPv6s)).Should(o.Equal(1))

		egressip1 := egressIPResource1{
			name:          "egressip-43466",
			template:      egressIP2Template,
			egressIP1:     freeIPv6s[0],
			nsLabelKey:    "org",
			nsLabelValue:  "qe",
			podLabelKey:   "color",
			podLabelValue: "pink",
		}
		egressip1.createEgressIPObject2(oc)
		defer egressip1.deleteEgressIPObject1(oc)
		egressIPMaps1 := getAssignedEIPInEIPObject(oc, egressip1.name)
		o.Expect(len(egressIPMaps1)).Should(o.Equal(1))

		exutil.By("Create a pod ")
		pod1 := pingPodResource{
			name:      "hello-pod",
			namespace: oc.Namespace(),
			template:  pingPodTemplate,
		}
		pod1.createPingPod(oc)
		waitPodReady(oc, pod1.namespace, pod1.name)

		exutil.By("Apply label to pod")
		err = exutil.LabelPod(oc, ns, pod1.name, "color=pink")
		defer exutil.LabelPod(oc, ns, pod1.name, "color-")
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Start tcpdump on node1")
		e2e.Logf("Trying to get physical interface on the node,%s", egressNode1)
		phyInf, nicError := getSnifPhyInf(oc, egressNode1)
		o.Expect(nicError).NotTo(o.HaveOccurred())
		exutil.SetNamespacePrivileged(oc, oc.Namespace())
		tcpdumpCmd := fmt.Sprintf("timeout 60s tcpdump -c 4 -nni %s icmp6 and dst %s", phyInf, rduBastionHost)
		cmdTcpdump, cmdOutput, _, err := oc.AsAdmin().Run("debug").Args("node/"+egressNode1, "--", "bash", "-c", tcpdumpCmd).Background()
		defer cmdTcpdump.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check source IP is EgressIP")
		pingCmd := fmt.Sprintf("ping -c4 %s", rduBastionHost)
		_, err = e2eoutput.RunHostCmd(pod1.namespace, pod1.name, pingCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmdErr := cmdTcpdump.Wait()
		o.Expect(cmdErr).NotTo(o.HaveOccurred())
		o.Expect(cmdOutput.String()).To(o.ContainSubstring(freeIPv6s[0]))

	})
})
