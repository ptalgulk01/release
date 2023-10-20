package router

import (
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("route-whitelist", exutil.KubeConfigPath())

	// author: aiyengar@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:aiyengar-Medium-42230-route can be configured to whitelist more than 61 ips/CIDRs", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)
		g.By("create project, pod, svc resources")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		g.By("expose a service in the project")
		exposeRoute(oc, oc.Namespace(), "svc/service-unsecure")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure"))

		g.By("annotate the route with haproxy.router.openshift.io/ip_whitelist with 61 CIDR values and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=192.168.0.0/24 192.168.1.0/24 192.168.2.0/24 192.168.3.0/24 192.168.4.0/24 192.168.5.0/24 192.168.6.0/24 192.168.7.0/24 192.168.8.0/24 192.168.9.0/24 192.168.10.0/24 192.168.11.0/24 192.168.12.0/24 192.168.13.0/24 192.168.14.0/24 192.168.15.0/24 192.168.16.0/24 192.168.17.0/24 192.168.18.0/24 192.168.19.0/24 192.168.20.0/24 192.168.21.0/24 192.168.22.0/24 192.168.23.0/24 192.168.24.0/24 192.168.25.0/24 192.168.26.0/24 192.168.27.0/24 192.168.28.0/24 192.168.29.0/24 192.168.30.0/24 192.168.31.0/24 192.168.32.0/24 192.168.33.0/24 192.168.34.0/24 192.168.35.0/24 192.168.36.0/24 192.168.37.0/24 192.168.38.0/24 192.168.39.0/24 192.168.40.0/24 192.168.41.0/24 192.168.42.0/24 192.168.43.0/24 192.168.44.0/24 192.168.45.0/24 192.168.46.0/24 192.168.47.0/24 192.168.48.0/24 192.168.49.0/24 192.168.50.0/24 192.168.51.0/24 192.168.52.0/24 192.168.53.0/24 192.168.54.0/24 192.168.55.0/24 192.168.56.0/24 192.168.57.0/24 192.168.58.0/24 192.168.59.0/24 192.168.60.0/24")
		output, err = oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("haproxy.router.openshift.io/ip_whitelist"))

		g.By("verify the acl whitelist parameter inside router pod for whitelist with 61 CIDR values")
		podName := getRouterPod(oc, "default")
		//backendName is the leading context of the route
		backendName := "be_http:" + oc.Namespace() + ":service-unsecure"
		output = readHaproxyConfig(oc, podName, backendName, "-A10", "acl whitelist")
		o.Expect(output).To(o.ContainSubstring(`acl whitelist src 192.168.0.0/24`))
		o.Expect(output).To(o.ContainSubstring(`tcp-request content reject if !whitelist`))
		o.Expect(output).NotTo(o.ContainSubstring(`acl whitelist src -f /var/lib/haproxy/router/whitelists/`))

		g.By("annotate the route with haproxy.router.openshift.io/ip_whitelist with more than 61 CIDR values and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=192.168.0.0/24 192.168.1.0/24 192.168.2.0/24 192.168.3.0/24 192.168.4.0/24 192.168.5.0/24 192.168.6.0/24 192.168.7.0/24 192.168.8.0/24 192.168.9.0/24 192.168.10.0/24 192.168.11.0/24 192.168.12.0/24 192.168.13.0/24 192.168.14.0/24 192.168.15.0/24 192.168.16.0/24 192.168.17.0/24 192.168.18.0/24 192.168.19.0/24 192.168.20.0/24 192.168.21.0/24 192.168.22.0/24 192.168.23.0/24 192.168.24.0/24 192.168.25.0/24 192.168.26.0/24 192.168.27.0/24 192.168.28.0/24 192.168.29.0/24 192.168.30.0/24 192.168.31.0/24 192.168.32.0/24 192.168.33.0/24 192.168.34.0/24 192.168.35.0/24 192.168.36.0/24 192.168.37.0/24 192.168.38.0/24 192.168.39.0/24 192.168.40.0/24 192.168.41.0/24 192.168.42.0/24 192.168.43.0/24 192.168.44.0/24 192.168.45.0/24 192.168.46.0/24 192.168.47.0/24 192.168.48.0/24 192.168.49.0/24 192.168.50.0/24 192.168.51.0/24 192.168.52.0/24 192.168.53.0/24 192.168.54.0/24 192.168.55.0/24 192.168.56.0/24 192.168.57.0/24 192.168.58.0/24 192.168.59.0/24 192.168.60.0/24 192.168.61.0/24 192.168.62.0/24 192.168.63.0/24 192.168.64.0/24")
		output1, err1 := oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.ContainSubstring("haproxy.router.openshift.io/ip_whitelist"))

		g.By("verify the acl whitelist parameter inside router pod for whitelist with 64 CIDR values")
		//backendName is the leading context of the route
		output2 := readHaproxyConfig(oc, podName, backendName, "-A10", "acl whitelist")
		o.Expect(output2).To(o.ContainSubstring(`acl whitelist src -f /var/lib/haproxy/router/whitelists/` + oc.Namespace() + `:service-unsecure.txt`))
		o.Expect(output2).To(o.ContainSubstring(`tcp-request content reject if !whitelist`))
	})

	// author: mjoseph@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-High-45399-ingress controller continue to function normally with unexpected high timeout value", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)
		g.By("create project, pod, svc resources")
		oc.SetupProject()
		createResourceFromFile(oc, oc.Namespace(), testPodSvc)
		err := waitForPodWithLabelReady(oc, oc.Namespace(), "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		g.By("expose a service in the project")
		exposeRoute(oc, oc.Namespace(), "svc/service-secure")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-secure"))

		g.By("annotate the route with haproxy.router.openshift.io/timeout annotation to high value and verify")
		setAnnotation(oc, oc.Namespace(), "route/service-secure", "haproxy.router.openshift.io/timeout=9999d")
		output, err = oc.Run("get").Args("route", "service-secure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`haproxy.router.openshift.io/timeout":"9999d`))

		g.By("Verify the haproxy configuration for the set timeout value")
		podName := getRouterPod(oc, "default")
		output = readHaproxyConfig(oc, podName, oc.Namespace(), "-A6", `timeout`)
		o.Expect(output).To(o.ContainSubstring(`timeout server  2147483647ms`))

		g.By("Verify the pod logs to see any timer overflow error messages")
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", "openshift-ingress", podName, "-c", "router").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(log).NotTo(o.ContainSubstring(`timer overflow`))
	})

	// author: mjoseph@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-High-49802-HTTPS redirect happens even if there is a more specific http-only", func() {
		//curling through defualt controller will not work for proxy cluster.
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, skip the test.")
		}
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
			customTemp          = filepath.Join(buildPruningBaseDir, "49802-route.yaml")
			rut                 = routeDescription{
				namespace: "",
				template:  customTemp,
			}
		)

		g.By("create project and a pod")
		baseDomain := getBaseDomain(oc)
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=hello-pod, Ready status not met")
		podName := getPodName(oc, project1, "name=web-server-rc")
		defaultContPod := getRouterPod(oc, "default")

		g.By("create routes and get the details")
		rut.namespace = project1
		rut.create(oc)
		getRoutes(oc, project1)

		g.By("check the reachability of the secure route with redirection")
		waitForCurl(oc, podName[0], baseDomain, "hello-pod-"+project1+".apps.", "HTTP/1.1 302 Found", "")
		waitForCurl(oc, podName[0], baseDomain, "hello-pod-"+project1+".apps.", `location: https://hello-pod-`, "")

		g.By("check the reachability of the insecure routes")
		waitForCurl(oc, podName[0], baseDomain+"/test/", "hello-pod-http-"+project1+".apps.", "HTTP/1.1 200 OK", "")

		g.By("check the reachability of the secure route")
		curlCmd := fmt.Sprintf("curl -I -k https://hello-pod-%s.apps.%s", project1, baseDomain)
		statsOut, err := exutil.RemoteShPod(oc, project1, podName[0], "sh", "-c", curlCmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(statsOut).Should(o.ContainSubstring("HTTP/1.1 200 OK"))

		g.By("check the router pod and ensure the routes are loaded in haproxy.config")
		searchOutput := readRouterPodData(oc, defaultContPod, "cat haproxy.config", "hello-pod")
		o.Expect(searchOutput).To(o.ContainSubstring("backend be_edge_http:" + project1 + ":hello-pod"))
		searchOutput1 := readRouterPodData(oc, defaultContPod, "cat haproxy.config", "hello-pod-http")
		o.Expect(searchOutput1).To(o.ContainSubstring("backend be_http:" + project1 + ":hello-pod-http"))
	})

	// author: mjoseph@redhat.com
	g.It("Longduration-Author:mjoseph-Critical-53696-Route status should updates accordingly when ingress routes cleaned up [Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp53696",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("check the intial canary route status")
		routerpods := getResourceName(oc, "openshift-ingress", "pods")
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "default", false)

		g.By("shard the default ingress controller")
		defer patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":null}}}}")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":\"shard\"}}}}")
		waitForRangeOfResourceToDisappear(oc, "openshift-ingress", routerpods)
		newrouterpods := getResourceName(oc, "openshift-ingress", "pods")

		g.By("check whether canary route status is cleared")
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status", "default", true)

		g.By("patch the controller back to default check the canary route status")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":null}}}}")
		waitForRangeOfResourceToDisappear(oc, "openshift-ingress", newrouterpods)
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "default", false)

		g.By("Create a shard ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = "shard." + baseDomain
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		crouterpod := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"nodePlacement\":{\"nodeSelector\":{\"matchLabels\":{\"node-role.kubernetes.io/worker\":\"\"}}}}}")
		err2 := waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+crouterpod)
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+crouterpod))
		custContPod := getRouterPod(oc, ingctrl.name)

		g.By("check the canary route status with shard controller")
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "default", false)
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "ocp53696", false)

		g.By("delete the shard and check the status")
		ingctrl.delete(oc)
		err3 := waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+custContPod)
		exutil.AssertWaitPollNoErr(err3, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+custContPod))
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "default", false)
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "ocp53696", true)
	})

	// bugzilla: 2021446
	g.It("Author:mjoseph-High-55895-When canary route is not available, Ingress should be in degarded state	[Disruptive]", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			cltPodName          = "hello-pod"
			cltPodLabel         = "app=hello-pod"
		)

		g.By("Deploy a project with a client pod")
		project1 := oc.Namespace()
		baseDomain := getBaseDomain(oc)
		g.By("create a client pod")
		createResourceFromFile(oc, project1, clientPod)
		err := waitForPodWithLabelReady(oc, project1, cltPodLabel)
		exutil.AssertWaitPollNoErr(err, "A client pod failed to be ready state within allowed time!")

		g.By("Check the intial canary route status")
		routerpods := getResourceName(oc, "openshift-ingress", "pods")
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status.ingress[*].routerName", "default", false)

		g.By("Check the reachability of the canary route")
		routehost := "canary-openshift-ingress-canary.apps." + baseDomain
		cmdOnPod := []string{cltPodName, "--", "curl", "-Ik", "https://" + routehost}
		result := repeatCmd(oc, cmdOnPod, "200 OK", 5)
		o.Expect(result).To(o.ContainSubstring("passed"))

		g.By("Patch the ingress controller and deleting the canary route")
		defer ensureClusterOperatorNormal(oc, "ingress", 5, 300)
		defer patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":null}}")
		patchResourceAsAdmin(oc, "openshift-ingress-operator", "ingresscontrollers/default", "{\"spec\":{\"routeSelector\":{\"matchLabels\":{\"type\":\"default\"}}}}")
		// Deleting canary route
		err = oc.AsAdmin().Run("delete").Args("-n", "openshift-ingress-canary", "route", "canary").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForRangeOfResourceToDisappear(oc, "openshift-ingress", routerpods)

		g.By("Check whether the canary route status cleared and route is not accessible")
		getNamespaceRouteDetails(oc, "openshift-ingress-canary", "canary", ".status", "default", true)
		cmdOnPod = []string{cltPodName, "--", "curl", "-Ik", "https://" + routehost}
		result = repeatCmd(oc, cmdOnPod, "503", 5)
		o.Expect(result).To(o.ContainSubstring("passed"))

		// Wait may be about 300 seconds
		g.By("Check the ingress operator status to confirm it is in degraded state cause by canary route")
		jpath := ".status.conditions[*].message"
		waitForOutput(oc, "default", "co/ingress", jpath, "The \"default\" ingress controller reports Degraded=True")
		waitForOutput(oc, "default", "co/ingress", jpath, "Canary route is not admitted by the default ingress controller")
	})

	// bugzilla: 1934904
	// no openshift-machine-api namespace on HyperShift guest cluster so this case is not available
	g.It("NonHyperShiftHOST-Author:mjoseph-NonPreRelease-Longduration-High-56240-Canary daemonset can schedule pods to both worker and infra nodes [Disruptive]", func() {
		var (
			machinSetName = "machineset-56240"
		)

		g.By("Check the intial machines and canary pod details")
		getResourceName(oc, "openshift-machine-api", "machine")
		getResourceName(oc, "openshift-ingress-canary", "pods")

		g.By("Create a new machineset")
		exutil.SkipConditionally(oc)
		ms := exutil.MachineSetDescription{Name: machinSetName, Replicas: 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset to schedule infra nodes")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("machinesets.machine.openshift.io", "machineset-56240", "-n", "openshift-machine-api", "-p", "{\"spec\":{\"template\":{\"spec\":{\"taints\":null}}}}", "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("machineset.machine.openshift.io/machineset-56240 patched"))
		out, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args("machinesets.machine.openshift.io", "machineset-56240", "-n", "openshift-machine-api", "-p", "{\"spec\":{\"template\":{\"spec\":{\"metadata\":{\"labels\":{\"ingress\": \"true\", \"node-role.kubernetes.io/infra\": \"\"}}}}}}", "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring("machineset.machine.openshift.io/machineset-56240 patched"))
		updatedMachineName := exutil.WaitForMachinesRunningByLabel(oc, 1, "machine.openshift.io/cluster-api-machineset=machineset-56240")

		g.By("Reschedule the running machineset with infra details")
		exutil.DeleteMachine(oc, updatedMachineName[0])
		updatedMachineName1 := exutil.WaitForMachinesRunningByLabel(oc, 1, "machine.openshift.io/cluster-api-machineset=machineset-56240")

		g.By("Check the canary deamonset is scheduled on infra node which is newly created")
		// confirm the new machineset is already created
		updatedMachineSetName := exutil.ListWorkerMachineSetNames(oc)
		checkGivenStringPresentOrNot(true, updatedMachineSetName, machinSetName)
		// confirm infra node presence among the nodes
		infraNode := searchStringUsingLabel(oc, "node", "node-role.kubernetes.io/infra", ".items[*].metadata.name")
		// confirm a canary pod got scheduled on to the infra node
		searchInDescribeResource(oc, "node", infraNode, "canary")

		g.By("Confirming the canary namespace is over-rided with the default node selector")
		annotations, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", "openshift-ingress-canary", "-ojsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(annotations).To(o.ContainSubstring(`openshift.io/node-selector":""`))

		g.By("Confirming the canary daemonset has the default tolerations included for infra role")
		tolerations := fetchJSONPathValue(oc, "openshift-ingress-canary", "daemonset/ingress-canary", ".spec.template.spec.tolerations")
		o.Expect(tolerations).To(o.ContainSubstring(`effect":"NoSchedule`))
		o.Expect(tolerations).To(o.ContainSubstring(`key":"node-role.kubernetes.io/infra`))

		g.By("Tainting the infra nodes and confirm canary pods continues to remain up and functional on those nodes")
		nodeNameOfMachine := exutil.GetNodeNameFromMachine(oc, updatedMachineName1[0])
		output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", "nodes", nodeNameOfMachine, "node-role.kubernetes.io/infra:NoSchedule").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("node/" + nodeNameOfMachine + " tainted"))
		// confirm the canary pod is still present in the infra node
		searchInDescribeResource(oc, "node", infraNode, "canary")
	})

	g.It("ROSA-OSD_CCS-ARO-Author:mjoseph-Medium-63004-Ipv6 addresses are also acceptable for whitelisting", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			output              string
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-rc.yaml")
		)

		g.By("Create a server pod")
		project1 := oc.Namespace()
		createResourceFromFile(oc, project1, testPodSvc)
		err := waitForPodWithLabelReady(oc, project1, "name=web-server-rc")
		exutil.AssertWaitPollNoErr(err, "the pod with name=web-server-rc Ready status not met")

		g.By("expose a service in the project")
		exposeRoute(oc, project1, "svc/service-unsecure")
		output, err = oc.Run("get").Args("route").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("service-unsecure"))

		g.By("Annotate the route with Ipv6 subnet and verify it")
		setAnnotation(oc, project1, "route/service-unsecure", "haproxy.router.openshift.io/ip_whitelist=2600:14a0::/40")
		output, err = oc.Run("get").Args("route", "service-unsecure", "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`"haproxy.router.openshift.io/ip_whitelist":"2600:14a0::/40"`))

		g.By("Verify the acl whitelist parameter inside router pod with Ipv6 address")
		defaultPod := getRouterPod(oc, "default")
		backendName := "be_http:" + project1 + ":service-unsecure"
		output = readHaproxyConfig(oc, defaultPod, backendName, "-A5", "acl whitelist src")
		o.Expect(output).To(o.ContainSubstring(`acl whitelist src 2600:14a0::/40`))
	})
})
