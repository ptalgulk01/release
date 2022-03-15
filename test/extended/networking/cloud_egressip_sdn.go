package networking

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	ci "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-networking] SDN", func() {
	defer g.GinkgoRecover()

	var (
		ipEchoUrl string
		a         *exutil.Aws_client

		oc = exutil.NewCLI("networking-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		platform := ci.CheckPlatform(oc)
		networkType := checkNetworkType(oc)
		e2e.Logf("\n\nThe platform is %v,  networkType is %v\n", platform, networkType)
		acceptedPlatform := strings.Contains(platform, "aws") || strings.Contains(platform, "gcp")
		if !acceptedPlatform || !strings.Contains(networkType, "sdn") {
			g.Skip("Test cases should be run on AWS or GCP cluster with Openshift-SDN network plugin, skip for other platforms or other network plugin!!")
		}

		switch platform {
		case "aws":
			e2e.Logf("\n AWS is detected, running the case on AWS\n")
			if ipEchoUrl == "" {
				getAwsCredentialFromCluster(oc)
				a = exutil.InitAwsSession()
				_, err := getAwsIntSvcInstanceID(a, oc)
				if err != nil {
					e2e.Logf("There is no int svc instance in this cluster, %v", err)
					g.Skip("There is no int svc instance in this cluster, skip the cases!!")
				}

				ipEchoUrl, err = installIpEchoServiceOnAWS(a, oc)
				if err != nil {
					e2e.Logf("No ip-echo service installed on the bastion host, %v", err)
					g.Skip("No ip-echo service installed on the bastion host, skip the cases!!")
				}
			}
		case "gcp":
			e2e.Logf("\n GCP is detected, running the case on GCP\n")
			if ipEchoUrl == "" {
				// If an int-svc instance with external IP found, IpEcho service will be installed on the int-svc instance
				// otherwise, just give error message and skip the test
				infraId, err := exutil.GetInfraId(oc)
				o.Expect(err).NotTo(o.HaveOccurred())
				host, err := getIntSvcExternalIpFromGcp(oc, infraId)
				if err != nil {
					e2e.Logf("There is no int svc instance in this cluster, %v", err)
					g.Skip("There is no int svc instance in this cluster, skip the cases!!")
				}
				ipEchoUrl, err = installIpEchoServiceOnGCP(oc, infraId, host)
				if err != nil {
					e2e.Logf("No ip-echo service installed on the bastion host, %v", err)
					g.Skip("No ip-echo service installed on the bastion host, skip the cases!!")
				}
			}
		default:
			e2e.Logf("Not support cloud provider for auto egressip cases for now.")
			g.Skip("Not support cloud provider for auto egressip cases for now.")
		}

	})

	// author: jechen@redhat.com
	g.It("Author:jechen-High-46701-The same egressIP will not be assigned to different netnamespace. [Disruptive]", func() {

		buildPruningBaseDir := exutil.FixturePath("testdata", "networking")
		pingPodTemplate := filepath.Join(buildPruningBaseDir, "ping-for-pod.yaml")

		g.By("1. Pick a node as egressIP node, add egressCIDRs to it")
		// get CIDR on the node
		nodeList, err := e2enode.GetReadySchedulableNodes(oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		egressNode := nodeList.Items[0].Name
		sub1 := getIfaddrFromNode(egressNode, oc)

		patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[\""+sub1+"\"]}")
		defer patchResourceAsAdmin(oc, "hostsubnet/"+egressNode, "{\"egressCIDRs\":[]}")

		g.By("2. Create first namespace then create a test pod in it")
		oc.SetupProject()
		ns1 := oc.Namespace()

		pod1 := pingPodResource{
			name:      "hello-pod1",
			namespace: ns1,
			template:  pingPodTemplate,
		}

		pod1.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod1.name, "-n", pod1.namespace).Execute()
		waitPodReady(oc, pod1.namespace, pod1.name)

		g.By("3. Find an unused IP on the node, use it as egressIP address, add it to netnamespace of the first project")
		freeIps := findUnUsedIPsOnNode(oc, egressNode, sub1, 1)
		o.Expect(len(freeIps) == 1).Should(o.BeTrue())
		patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[\""+freeIps[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod1.namespace, "{\"egressIPs\":[]}")

		g.By("4. Verify egressCIDRs and egressIPs on the node")
		output := getEgressCIDRs(oc, egressNode)
		o.Expect(output).To(o.ContainSubstring(sub1))
		ip, err := getEgressIPonSDNHost(oc, nodeList.Items[0].Name)
		e2e.Logf("\n\n\n got egressIP as -->%v<--\n\n\n", ip)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIps[0]).Should(o.BeElementOf(ip))

		g.By("4.From first namespace, check source IP is EgressIP")
		e2e.Logf("\n ipEchoUrl is %v\n", ipEchoUrl)
		sourceIp, err := e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoUrl+" --connect-timeout 5")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(sourceIp).Should(o.BeElementOf(freeIps[0]))

		g.By("5. Create second namespace, create a test pod in it")
		oc.SetupProject()
		ns2 := oc.Namespace()

		pod2 := pingPodResource{
			name:      "hello-pod2",
			namespace: ns2,
			template:  pingPodTemplate,
		}

		pod2.createPingPod(oc)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", pod2.name, "-n", pod2.namespace).Execute()
		waitPodReady(oc, pod2.namespace, pod2.name)

		g.By("6. Add same egressIP address to netnamespace of the second project")
		patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[\""+freeIps[0]+"\"]}")
		defer patchResourceAsAdmin(oc, "netnamespace/"+pod2.namespace, "{\"egressIPs\":[]}")

		g.By("7. check egressIP again after second netnamespace is patched with same egressIP")
		ip, err = getEgressIPonSDNHost(oc, nodeList.Items[0].Name)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(freeIps[0]).Should(o.BeElementOf(ip))

		g.By("8.Check source IP again from first project, curl command should not succeed, error is expected")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoUrl+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())

		g.By("9.Check source IP again from second project, curl command should not succeed, error is expected")
		_, err = e2e.RunHostCmd(pod1.namespace, pod1.name, "curl -s "+ipEchoUrl+" --connect-timeout 5")
		o.Expect(err).To(o.HaveOccurred())
	})

})
