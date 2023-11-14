package router

import (
	"fmt"
	"os"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-network-edge] Network_Edge should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("router-tunning", exutil.KubeConfigPath())

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-40747-The 'tune.maxrewrite' value can be modified with 'headerBufferMaxRewriteBytes' parameter", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp40747",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with tune.maxrewrite buffer value")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferMaxRewriteBytes\": 8192}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("check the haproxy config on the router pod for the tune.maxrewrite buffer value")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", "cat haproxy.config | grep tune.maxrewrite").Output()
		o.Expect(output2).To(o.ContainSubstring(`tune.maxrewrite 8192`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Critical-40748-The 'tune.bufsize' value can be modified with 'headerBufferBytes' parameter", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp40748",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("Patch ingresscontroller with tune.bufsize buffer value")
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 18000}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+routerpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newrouterpod := getRouterPod(oc, ingctrl.name)

		g.By("check the haproxy config on the router pod for the tune.bufsize buffer value")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newrouterpod, "--", "bash", "-c", "cat haproxy.config | grep tune.bufsize").Output()
		o.Expect(output2).To(o.ContainSubstring(`tune.bufsize 18000`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-High-40821-The 'tune.bufsize' and 'tune.maxwrite' values can be defined per haproxy router basis", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp40821a",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrl2 = ingressControllerDescription{
				name:      "ocp40821b",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("check the haproxy config on the router pod for existing maxrewrite and bufsize value")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "cat haproxy.config | grep -e tune.maxrewrite -e tune.bufsize").Output()
		o.Expect(output).To(o.ContainSubstring(`tune.maxrewrite 4097`))
		o.Expect(output).To(o.ContainSubstring(`tune.bufsize 16385`))

		g.By("Create a second custom ingresscontroller, and get its router name")
		baseDomain = getBaseDomain(oc)
		ingctrl2.domain = ingctrl2.name + "." + baseDomain
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		err2 := waitForCustomIngressControllerAvailable(oc, ingctrl2.name)
		exutil.AssertWaitPollNoErr(err2, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl2.name))
		secondRouterpod := getRouterPod(oc, ingctrl2.name)

		g.By("Patch the second ingresscontroller with maxrewrite and bufsize value")
		ingctrlResource := "ingresscontrollers/" + ingctrl2.name
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 18000, \"headerBufferMaxRewriteBytes\":10000}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+secondRouterpod)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Router  %v failed to fully terminate", "pod/"+routerpod))
		newSecondRouterpod := getRouterPod(oc, ingctrl2.name)

		g.By("check the haproxy config on the router pod of second ingresscontroller for the tune.bufsize buffer value")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", newSecondRouterpod, "--", "bash", "-c", "cat haproxy.config | grep -e tune.maxrewrite -e tune.bufsize").Output()
		o.Expect(output1).To(o.ContainSubstring(`tune.maxrewrite 10000`))
		o.Expect(output1).To(o.ContainSubstring(`tune.bufsize 18000`))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-40822-The 'headerBufferBytes' and 'headerBufferMaxRewriteBytes' strictly honours the default minimum values", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp40822",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create a custom ingresscontroller, and get its router name")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))
		routerpod := getRouterPod(oc, ingctrl.name)

		g.By("check the existing maxrewrite and bufsize value")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerpod, "--", "bash", "-c", "/usr/bin/env | grep -ie buf -ie rewrite").Output()
		o.Expect(output).To(o.ContainSubstring(`ROUTER_BUF_SIZE=16385`))
		o.Expect(output).To(o.ContainSubstring(`ROUTER_MAX_REWRITE_SIZE=4097`))

		g.By("Patch ingresscontroller with minimum values and check whether it is configurable")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("ingresscontroller/ocp40822", "-p", "{\"spec\":{\"tuningOptions\" :{\"headerBufferBytes\": 8192, \"headerBufferMaxRewriteBytes\":2048}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output1).To(o.ContainSubstring(`The IngressController "ocp40822" is invalid`))
		o.Expect(output1).To(o.ContainSubstring("spec.tuningOptions.headerBufferMaxRewriteBytes: Invalid value: 2048: spec.tuningOptions.headerBufferMaxRewriteBytes in body should be greater than or equal to 4096"))
		o.Expect(output1).To(o.ContainSubstring("spec.tuningOptions.headerBufferBytes: Invalid value: 8192: spec.tuningOptions.headerBufferBytes in body should be greater than or equal to 16384"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Critical-41110-The threadCount ingresscontroller parameter controls the nbthread option for the haproxy router", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp41110",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			threadcount = "6"
		)

		g.By("Create a ingresscontroller with threadCount set")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new ingresscontroller with tuningOptions/threadCount " + threadcount)
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		podname := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount+"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check the router env to verify the PROXY variable ROUTER_THREADS with " + threadcount + " is applied")
		newpodname := getRouterPod(oc, ingctrl.name)
		dssearch := readRouterPodEnv(oc, newpodname, "ROUTER_THREADS")
		o.Expect(dssearch).To(o.ContainSubstring("ROUTER_THREADS=" + threadcount))
		g.By("check the haproxy config on the router pod to ensure the nbthread is updated")

		nbthread := readRouterPodData(oc, newpodname, "cat haproxy.config", "nbthread")
		o.Expect(nbthread).To(o.ContainSubstring("nbthread " + threadcount))
	})

	// author: mjoseph@redhat.com
	g.It("Author:mjoseph-Low-41128-Ingresscontroller should not accept invalid nbthread setting", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		customTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		var (
			ingctrl = ingressControllerDescription{
				name:      "ocp41128",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			threadcountDefault = "4"
			threadcount1       = "-1"
			threadcount2       = "512"
			threadcount3       = `"abc"`
		)

		g.By("create a custom ingresscontroller")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch the new ingresscontroller with negative(" + threadcount1 + ") value as threadCount")
		output1, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount1+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output1).To(o.ContainSubstring("Invalid value: -1: spec.tuningOptions.threadCount in body should be greater than or equal to 1"))

		g.By("Patch the new ingresscontroller with high(" + threadcount2 + ") value for threadCount")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount2+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output2).To(o.ContainSubstring("Invalid value: 512: spec.tuningOptions.threadCount in body should be less than or equal to 64"))

		g.By("Patch the new ingresscontroller with string(" + threadcount3 + ") value for threadCount")
		output3, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(
			"ingresscontroller/"+ingctrl.name, "-p", "{\"spec\": {\"tuningOptions\": {\"threadCount\": "+threadcount3+"}}}",
			"--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(output3).To(o.ContainSubstring(`Invalid value: "string": spec.tuningOptions.threadCount in body must be of type integer: "string"`))

		g.By("Check the router env to verify the default value of ROUTER_THREADS is applied")
		podname := getRouterPod(oc, ingctrl.name)
		threadValue := readRouterPodEnv(oc, podname, "ROUTER_THREADS")
		o.Expect(threadValue).To(o.ContainSubstring("ROUTER_THREADS=" + threadcountDefault))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-50662-Make ROUTER_BACKEND_CHECK_INTERVAL Configurable", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "router")
		baseTemp := filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
		extraParas := "    tuningOptions:\n      healthCheckInterval: 20s\n"
		customTemp1 := addExtraParametersToYamlFile(baseTemp, "spec:", extraParas)
		defer os.Remove(customTemp1)
		extraParas = "    tuningOptions:\n      healthCheckInterval: 100m\n"
		customTemp2 := addExtraParametersToYamlFile(baseTemp, "spec:", extraParas)
		defer os.Remove(customTemp2)
		ingctrl1 := ingressControllerDescription{
			name:      "ocp50662one",
			namespace: "openshift-ingress-operator",
			domain:    "",
			template:  customTemp1,
		}
		ingctrl2 := ingressControllerDescription{
			name:      "ocp50662two",
			namespace: "openshift-ingress-operator",
			domain:    "",
			template:  customTemp2,
		}
		ingctrlResource1 := "ingresscontrollers/" + ingctrl1.name
		ingctrlResource2 := "ingresscontrollers/" + ingctrl2.name

		exutil.By("Create two custom ICs for testing ROUTER_BACKEND_CHECK_INTERVAL")
		baseDomain := getBaseDomain(oc)
		ingctrl1.domain = ingctrl1.name + "." + baseDomain
		ingctrl2.domain = ingctrl2.name + "." + baseDomain
		defer ingctrl1.delete(oc)
		ingctrl1.create(oc)
		defer ingctrl2.delete(oc)
		ingctrl2.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl1.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl1.name))
		err = waitForCustomIngressControllerAvailable(oc, ingctrl2.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl2.name))

		exutil.By("Check ROUTER_BACKEND_CHECK_INTERVAL env in a route pod of IC ocp50662one, which should be 20s")
		podname1 := getRouterPod(oc, ingctrl1.name)
		hciSearch := readRouterPodEnv(oc, podname1, "ROUTER_BACKEND_CHECK_INTERVAL")
		o.Expect(hciSearch).To(o.ContainSubstring("ROUTER_BACKEND_CHECK_INTERVAL=20s"))

		exutil.By("Check ROUTER_BACKEND_CHECK_INTERVAL env in a route pod of IC ocp50662two, which should be 100m")
		podname2 := getRouterPod(oc, ingctrl2.name)
		hciSearch = readRouterPodEnv(oc, podname2, "ROUTER_BACKEND_CHECK_INTERVAL")
		o.Expect(hciSearch).To(o.ContainSubstring("ROUTER_BACKEND_CHECK_INTERVAL=100m"))

		exutil.By("Patch tuningOptions/healthCheckInterval with max 2147483647ms to IC ocp50662one, while tuningOptions/healthCheckInterval 0s to the IC ocp50662two")
		healthCheckInterval := "2147483647ms"
		patchResourceAsAdmin(oc, ingctrl1.namespace, ingctrlResource1, "{\"spec\": {\"tuningOptions\": {\"healthCheckInterval\": \""+healthCheckInterval+"\"}}}")
		healthCheckInterval = "0s"
		patchResourceAsAdmin(oc, ingctrl2.namespace, ingctrlResource2, "{\"spec\": {\"tuningOptions\": {\"healthCheckInterval\": \""+healthCheckInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname1)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname1))
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname2)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname2))

		exutil.By("Check ROUTER_BACKEND_CHECK_INTERVAL env in a route pod of IC ocp50662one, which should be 2147483647ms")
		podname1 = getRouterPod(oc, ingctrl1.name)
		hciSearch = readRouterPodEnv(oc, podname1, "ROUTER_BACKEND_CHECK_INTERVAL")
		o.Expect(hciSearch).To(o.ContainSubstring("ROUTER_BACKEND_CHECK_INTERVAL=2147483647ms"))

		exutil.By("Try to find the ROUTER_BACKEND_CHECK_INTERVAL env in a route pod which shouldn't be seen by default")
		podname2 = getRouterPod(oc, ingctrl2.name)
		cmd := fmt.Sprintf("/usr/bin/env | grep %s", "ROUTER_BACKEND_CHECK_INTERVAL")
		_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", podname2, "--", "bash", "-c", cmd).Output()
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-50663-Negative Test of Make ROUTER_BACKEND_CHECK_INTERVAL Configurable", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp50663",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create an custom ingresscontroller for testing ROUTER_BACKEND_CHECK_INTERVAL")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Try to patch tuningOptions/healthCheckInterval 2147483900ms which is larger than the max healthCheckInterval, to the ingress-controller")
		NegHealthCheckInterval := "2147483900ms"
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		podname := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"healthCheckInterval\": \""+NegHealthCheckInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check ROUTER_BACKEND_CHECK_INTERVAL env in a route pod which should be the max: 2147483647ms")
		podname = getRouterPod(oc, ingctrl.name)
		hciSearch := readRouterPodEnv(oc, podname, "ROUTER_BACKEND_CHECK_INTERVAL")
		o.Expect(hciSearch).To(o.ContainSubstring("ROUTER_BACKEND_CHECK_INTERVAL=" + "2147483647ms"))

		g.By("Try to patch tuningOptions/healthCheckInterval -1s which is a minus value, to the ingress-controller")
		NegHealthCheckInterval = "-1s"
		output, err1 := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", "{\"spec\": {\"tuningOptions\": {\"healthCheckInterval\": \""+NegHealthCheckInterval+"\"}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err1).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"-1s\""))

		g.By("Try to patch tuningOptions/healthCheckInterval abc which is a string, to the ingress-controller")
		NegHealthCheckInterval = "0abc"
		output, err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", "{\"spec\": {\"tuningOptions\": {\"healthCheckInterval\": \""+NegHealthCheckInterval+"\"}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err2).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"0abc\":"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-50926-Support a Configurable ROUTER_MAX_CONNECTIONS in HAproxy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp50926",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("Create a custom IC, and then patch tuningOptions/maxConnections 2000 to it")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"maxConnections\": 2000}}}")
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		exutil.By("Check ROUTER_MAX_CONNECTIONS env under a router pod of IC ocp50926, which should be 2000")
		podname := getRouterPod(oc, ingctrl.name)
		maxConnSearch := readRouterPodEnv(oc, podname, "ROUTER_MAX_CONNECTIONS")
		o.Expect(maxConnSearch).To(o.ContainSubstring("ROUTER_MAX_CONNECTIONS=2000"))

		exutil.By("Check maxconn in haproxy.config under a router pod of IC ocp50926, which should be 2000")
		maxConnCfg := readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 2000"))

		exutil.By("Patch tuningOptions/maxConnections with max 2000000 to the ingress-controller ocp50926")
		podname = getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"maxConnections\": 2000000}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		exutil.By("Check ROUTER_MAX_CONNECTIONS env under a router pod of IC ocp50926,  which should be 2000000")
		podname = getRouterPod(oc, ingctrl.name)
		maxConnSearch = readRouterPodEnv(oc, podname, "ROUTER_MAX_CONNECTIONS")
		o.Expect(maxConnSearch).To(o.ContainSubstring("ROUTER_MAX_CONNECTIONS=2000000"))

		exutil.By("Check maxconn in haproxy.config under a router pod of IC ocp50926, which should be 2000000")
		maxConnCfg = readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 2000000"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-50928-Negative test of Support a Configurable ROUTER_MAX_CONNECTIONS in HAproxy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-tuning.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp50928",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
			ingctrlResource = "ingresscontrollers/" + ingctrl.name
		)

		exutil.By("Create a custom IC with tuningOptions/maxConnections -1 specified by ingresscontroller-tuning.yaml")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		exutil.By("Check ROUTER_MAX_CONNECTIONS env under a route pod for the configured maxConnections -1,  which should be auto")
		podname := getRouterPod(oc, ingctrl.name)
		maxConnSearch := readRouterPodEnv(oc, podname, "ROUTER_MAX_CONNECTIONS")
		o.Expect(maxConnSearch).To(o.ContainSubstring("ROUTER_MAX_CONNECTIONS=auto"))

		exutil.By("Check maxconn in haproxy.config which won't appear after configured tuningOptions/maxConnections with -1")
		cmd := fmt.Sprintf("%s | grep \"%s\"", "cat haproxy.config", "maxconn")
		_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", podname, "--", "bash", "-c", cmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Patch tuningOptions/maxConnections 0 to the IC")
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"maxConnections\": 0}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		exutil.By("Try to Check ROUTER_MAX_CONNECTIONS env in a route pod set to the default maxConnections by 0")
		podname = getRouterPod(oc, ingctrl.name)
		cmd = fmt.Sprintf("/usr/bin/env | grep %s", "ROUTER_MAX_CONNECTIONS")
		_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", podname, "--", "bash", "-c", cmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Check maxconn in haproxy.config under a router pod of the IC , which should be 50000")
		maxConnCfg := readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 50000"))

		exutil.By("Try to patch the ingress-controller with tuningOptions/maxConnections 1999, which is less than the min 2000")
		NegMaxConnections := "1999"
		output, err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", "{\"spec\": {\"tuningOptions\": {\"maxConnections\": "+NegMaxConnections+"}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err2).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Unsupported value: " + NegMaxConnections + ": supported values: \"-1\", \"0\""))

		exutil.By("Try to patch the ingress-controller with tuningOptions/maxConnections 2000001, which is a larger than the max 2000000")
		NegMaxConnections = "2000001"
		output, err2 = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", "{\"spec\": {\"tuningOptions\": {\"maxConnections\": "+NegMaxConnections+"}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err2).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Unsupported value: " + NegMaxConnections + ": supported values: \"-1\", \"0\""))

		exutil.By("Try to patch the ingress-controller with tuningOptions/maxConnections abc, which is a string")
		NegMaxConnections = "abc"
		output, err2 = oc.AsAdmin().WithoutNamespace().Run("patch").Args(ingctrlResource, "-p", "{\"spec\": {\"tuningOptions\": {\"maxConnections\": \""+NegMaxConnections+"\"}}}", "--type=merge", "-n", ingctrl.namespace).Output()
		o.Expect(err2).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"string\": spec.tuningOptions.maxConnections in body must be of type integer"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-High-55367-Default HAProxy maxconn value to 50000 for OCP 4.12", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp55367",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		exutil.By("Create an custom ingresscontroller for testing ROUTER_MAX_CONNECTIONS")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		exutil.By("Check default value of ROUTER_MAX_CONNECTIONS env in a route pod, which shouldn't appear in it")
		podname := getRouterPod(oc, ingctrl.name)
		cmd := fmt.Sprintf("/usr/bin/env | grep %s", "ROUTER_MAX_CONNECTIONS")
		_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", podname, "--", "bash", "-c", cmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Check maxconn in haproxy.config which should be 50000")
		maxConnCfg := readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 50000"))

		exutil.By("Patch tuningOptions/maxConnections with null to the ingress-controller")
		maxConnections := "null"
		jpath := ".status.observedGeneration"
		observedGen1 := fetchJSONPathValue(oc, "openshift-ingress", "deployment.apps/router-default", jpath)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"maxConnections\": "+maxConnections+"}}}")
		observedGen2 := fetchJSONPathValue(oc, "openshift-ingress", "deployment.apps/router-default", jpath)
		o.Expect(observedGen1).To(o.ContainSubstring(observedGen2))

		exutil.By("Check ROUTER_MAX_CONNECTIONS env in a route pod which shouldn't appear in it by default")
		podname = getRouterPod(oc, ingctrl.name)
		_, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", podname, "--", "bash", "-c", cmd).Output()
		o.Expect(err).To(o.HaveOccurred())

		exutil.By("Check maxconn in haproxy.config which should be 50000")
		maxConnCfg = readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 50000"))

		exutil.By("Patch tuningOptions/maxConnections 50000 to the ingress-controller")
		maxConnections = "500000"
		podname = getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"maxConnections\": "+maxConnections+"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		exutil.By("Check ROUTER_MAX_CONNECTIONS env in a route pod which should be " + maxConnections)
		podname = getRouterPod(oc, ingctrl.name)
		maxConnSearch := readRouterPodEnv(oc, podname, "ROUTER_MAX_CONNECTIONS")
		o.Expect(maxConnSearch).To(o.ContainSubstring("ROUTER_MAX_CONNECTIONS=" + maxConnections))

		exutil.By("Check maxconn in haproxy.config which should be 50000")
		maxConnCfg = readRouterPodData(oc, podname, "cat haproxy.config", "maxconn")
		o.Expect(maxConnCfg).To(o.ContainSubstring("maxconn 50000"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-NonPreRelease-Longduration-High-53605-Expose a Configurable Reload Interval in HAproxy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp53605",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create an custom ingresscontroller for testing router reload interval")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Patch tuningOptions/reloadInterval 15s to the ingress-controller")
		reloadInterval := "15s"
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		podname := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+reloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check RELOAD_INTERVAL env in a route pod which should be " + reloadInterval)
		podname = getRouterPod(oc, ingctrl.name)
		riSearch := readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=" + reloadInterval))

		g.By("Patch tuningOptions/reloadInterval with max 120s to the ingress-controller")
		reloadInterval = "120s"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+reloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check RELOAD_INTERVAL env in a route pod which should be 2m")
		podname = getRouterPod(oc, ingctrl.name)
		riSearch = readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=2m"))

		g.By("Patch tuningOptions/reloadInterval with other valid unit m, for exmpale 1m to the ingress-controller")
		reloadInterval = "1m"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+reloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check RELOAD_INTERVAL env in a route pod which should be " + reloadInterval)
		podname = getRouterPod(oc, ingctrl.name)
		riSearch = readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=" + reloadInterval))

		g.By("Patch tuningOptions/reloadInterval with 0s to the ingress-controller, expect to set it to default 5s")
		reloadInterval = "0s"
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+reloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Try to find the RELOAD_INTERVAL env in a route pod which is the default 5s")
		podname = getRouterPod(oc, ingctrl.name)
		riSearch = readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=5s"))
	})

	// author: shudili@redhat.com
	g.It("Author:shudili-Low-53608-Negative Test of Expose a Configurable Reload Interval in HAproxy", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "router")
			customTemp          = filepath.Join(buildPruningBaseDir, "ingresscontroller-np.yaml")
			ingctrl             = ingressControllerDescription{
				name:      "ocp53608",
				namespace: "openshift-ingress-operator",
				domain:    "",
				template:  customTemp,
			}
		)

		g.By("Create an custom ingresscontroller for testing router reload interval")
		baseDomain := getBaseDomain(oc)
		ingctrl.domain = ingctrl.name + "." + baseDomain
		defer ingctrl.delete(oc)
		ingctrl.create(oc)
		err := waitForCustomIngressControllerAvailable(oc, ingctrl.name)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ingresscontroller %s conditions not available", ingctrl.name))

		g.By("Try to patch tuningOptions/reloadInterval 121s which is larger than the max 120s, to the ingress-controller")
		NegReloadInterval := "121s"
		ingctrlResource := "ingresscontrollers/" + ingctrl.name
		podname := getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+NegReloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check RELOAD_INTERVAL env in a route pod which should be the max 2m")
		podname = getRouterPod(oc, ingctrl.name)
		riSearch := readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=2m"))

		g.By("Try to patch tuningOptions/reloadInterval 0.5s which is less than the min 1s, to the ingress-controller")
		NegReloadInterval = "0.5s"
		ingctrlResource = "ingresscontrollers/" + ingctrl.name
		podname = getRouterPod(oc, ingctrl.name)
		patchResourceAsAdmin(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+NegReloadInterval+"\"}}}")
		err = waitForResourceToDisappear(oc, "openshift-ingress", "pod/"+podname)
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("resource %v does not disapper", "pod/"+podname))

		g.By("Check RELOAD_INTERVAL env in a route pod which should be the min 1s")
		podname = getRouterPod(oc, ingctrl.name)
		riSearch = readRouterPodEnv(oc, podname, "RELOAD_INTERVAL")
		o.Expect(riSearch).To(o.ContainSubstring("RELOAD_INTERVAL=1s"))

		g.By("Try to patch tuningOptions/reloadInterval -1s which is a minus value, to the ingress-controller")
		NegReloadInterval = "-1s"
		output, errCfg := patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+NegReloadInterval+"\"}}}")
		o.Expect(errCfg).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"" + NegReloadInterval + "\""))

		g.By("Try to patch tuningOptions/reloadInterval 1abc which is a string, to the ingress-controller")
		NegReloadInterval = "1abc"
		output, errCfg = patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+NegReloadInterval+"\"}}}")
		o.Expect(errCfg).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"" + NegReloadInterval + "\""))

		g.By("Try to patch tuningOptions/reloadInterval 012 s which contains a space character, to the ingress-controller")
		NegReloadInterval = "012 s"
		output, errCfg = patchResourceAsAdminAndGetLog(oc, ingctrl.namespace, ingctrlResource, "{\"spec\": {\"tuningOptions\": {\"reloadInterval\": \""+NegReloadInterval+"\"}}}")
		o.Expect(errCfg).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"" + NegReloadInterval + "\""))
	})
})
