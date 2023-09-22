package workloads

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-apps] Workloads", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: yinzhou@redhat.com
	// Adding NonHyperShiftHOST due to bug https://issues.redhat.com/browse/HOSTEDCP-936
	//It is destructive case, will make kube-scheduler roll out, so adding [Disruptive]. One rollout costs about 5mins, so adding [Slow]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:yinzhou-Medium-31939-Verify logLevel settings in kube scheduler operator [Disruptive][Slow]", func() {
		patchYamlToRestore := `[{"op": "replace", "path": "/spec/logLevel", "value":"Normal"}]`

		g.By("Set the loglevel to TraceAll")
		patchYamlTraceAll := `[{"op": "replace", "path": "/spec/logLevel", "value":"TraceAll"}]`
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubescheduler", "cluster", "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Restoring the scheduler cluster's logLevel")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubescheduler", "cluster", "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check the scheduler operator should be in Progressing")
			e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		}()

		g.By("Check the scheduler operator should be in Progressing")
		e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		g.By("Check the loglevel setting for the pod")
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pods", "-n", "openshift-kube-scheduler", "-l", "app=openshift-kube-scheduler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("-v=10", output); matched {
			e2e.Logf("clusteroperator kube-scheduler is running with logLevel 10\n")
		}

		g.By("Set the loglevel to Trace")
		patchYamlTrace := `[{"op": "replace", "path": "/spec/logLevel", "value":"Trace"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubescheduler", "cluster", "--type=json", "-p", patchYamlTrace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the scheduler operator should be in Progressing")
		e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
		expectedStatus = map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		g.By("Check the loglevel setting for the pod")
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("pods", "-n", "openshift-kube-scheduler", "-l", "app=openshift-kube-scheduler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("-v=6", output); matched {
			e2e.Logf("clusteroperator kube-scheduler is running with logLevel 6\n")
		}

		g.By("Set the loglevel to Debug")
		patchYamlDebug := `[{"op": "replace", "path": "/spec/logLevel", "value":"Debug"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubescheduler", "cluster", "--type=json", "-p", patchYamlDebug).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the scheduler operator should be in Progressing")
		e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
		expectedStatus = map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		g.By("Check the loglevel setting for the pod")
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("pods", "-n", "openshift-kube-scheduler", "-l", "app=openshift-kube-scheduler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("-v=4", output); matched {
			e2e.Logf("clusteroperator kube-scheduler is running with logLevel 4\n")
		}
	})

	g.It("Author:knarra-High-44049-DefaultPodTopologySpread doesn't work in non-CloudProvider env in OpenShift 4.7 [Disruptive][Flaky]", func() {
		workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("workernodeList is %v", workerNodeList)
		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		// check and label the worker node with topology.kubernetes.io/zone if it is not present
		Output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", workerNodeList[0], "-o", "jsonpath='{.metadata.labels}'").Output()
		e2e.Logf("Output is %v", Output)
		if strings.Contains(Output, "topology.kubernetes.io/zone") {
			g.Skip("Worker node has zone label so the test can be skipped, as this is only meant for worker with no zone label, for more info please refer BZ1979433")
			return
		}
		defer func() {
			for _, v := range workerNodeList {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", v)).Execute()
			}
			for _, v := range workerNodeList {
				err = checkNodeUncordoned(oc, v)
				exutil.AssertWaitPollNoErr(err, "node is not ready")
			}
		}()

		for _, v := range workerNodeList {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", v)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		// Uncordon first two nodes
		g.By("Uncordon first two nodes")
		err = oc.AsAdmin().Run("adm").Args("uncordon", workerNodeList[0]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().Run("adm").Args("uncordon", workerNodeList[1]).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Label Node1 & Node2")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workerNodeList[0], "topology.kubernetes.io/zone")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workerNodeList[0], "topology.kubernetes.io/zone", "ocp44049zoneA")
		defer e2enode.RemoveLabelOffNode(oc.KubeFramework().ClientSet, workerNodeList[1], "topology.kubernetes.io/zone")
		e2enode.AddOrUpdateLabelOnNode(oc.KubeFramework().ClientSet, workerNodeList[1], "topology.kubernetes.io/zone", "ocp44049zoneB")

		// Test starts here
		// Test for Large pods
		err = oc.Run("create").Args("deployment", "ocp44049large", "--image", "gcr.io/google-samples/node-hello:1.0", "--replicas", "0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("resources", "deployment/ocp44049large", "--limits=cpu=2,memory=4Gi").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("scale").Args("deployment/ocp44049large", "--replicas", "2").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp44049large", oc.Namespace(), "2"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		expectNodeList := []string{workerNodeList[0], workerNodeList[1]}
		g.By("Geting the node list where pods running")
		lpodNodeList := getPodNodeListByLabel(oc, oc.Namespace(), "app=ocp44049large")
		sort.Strings(lpodNodeList)

		if reflect.DeepEqual(lpodNodeList, expectNodeList) {
			e2e.Logf("All large pods have spread properly, which is expected")
		} else {
			e2e.Failf("Large pods have not been spread properly")
		}

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		// Test for small pods
		err = oc.Run("create").Args("deployment", "ocp44049small", "--image", "gcr.io/google-samples/node-hello:1.0", "--replicas", "0").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("resources", "deployment/ocp44049small", "--limits=cpu=0.1,memory=128Mi").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("scale").Args("deployment/ocp44049small", "--replicas", "6").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check all the pods should running")
		if ok := waitForAvailableRsRunning(oc, "deployment", "ocp44049small", oc.Namespace(), "2"); ok {
			e2e.Logf("All pods are runnnig now\n")
		}

		spodNodeList := getPodNodeListByLabel(oc, oc.Namespace(), "app=ocp44049small")
		spodNodeList = removeDuplicateElement(spodNodeList)
		sort.Strings(spodNodeList)

		if reflect.DeepEqual(spodNodeList, expectNodeList) {
			e2e.Logf("All small pods have spread properly, which is expected")
		} else {
			e2e.Failf("small pods have not been spread properly")
		}

	})

	// author: knarra@redhat.com
	// Added NonHyperShiftHOST as added another case 67153 in same file to test this on HypershiftHost a adjusting this becomes very complex.
	//It is destructive case, will make kube-scheduler roll out, so adding [Disruptive]. One rollout costs about 5mins, so adding [Slow]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:knarra-High-50931-Validate HighNodeUtilization profile 4.10 and above [Disruptive][Slow]", func() {
		patchYamlToRestore := `[{"op": "remove", "path": "/spec/profile"}]`

		g.By("Set profile to HighNodeUtilization")
		patchYamlTraceAll := `[{"op": "add", "path": "/spec/profile", "value":"HighNodeUtilization"}]`
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Restoring the scheduler cluster's logLevel")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Checking KSO operator should be in Progressing and Available after rollout and recovery")
			e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		}()

		g.By("Checking KSO operator should be in Progressing and Available after rollout and recovery")
		e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		//Get the kube-scheduler pod name & check logs
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-kube-scheduler", "pods", "-l", "app=openshift-kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		schedulerLogs, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args(podName, "-n", "openshift-kube-scheduler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("score.*\n.*disabled.*\n.*NodeResourcesBalancedAllocation.*\n.*weight.*0.*", schedulerLogs); !match {
			e2e.Failf("Enabling HighNodeUtilization Profile failed: %v", err)
		}
	})

	// author: knarra@redhat.com
	// Added NonHyperShiftHOST as added another case 67153 in same file to test this on HypershiftHost a adjusting this becomes very complex.
	//It is destructive case, will make kube-scheduler roll out, so adding [Disruptive]. One rollout costs about 5mins, so adding [Slow]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:knarra-High-50932-Validate NoScoring profile 4.10 and above [Disruptive][Slow]", func() {
		patchYamlToRestore := `[{"op": "remove", "path": "/spec/profile"}]`

		g.By("Set profile to NoScoring")
		patchYamlTraceAll := `[{"op": "add", "path": "/spec/profile", "value":"NoScoring"}]`
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patchYamlTraceAll).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Restoring the scheduler cluster's logLevel")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("Scheduler", "cluster", "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Checking KSO operator should be in Progressing and Available after rollout and recovery")
			e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
			e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		}()

		g.By("Checking KSO operator should be in Progressing and Available after rollout and recovery")
		e2e.Logf("Checking kube-scheduler operator should be in Progressing in 100 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-scheduler", 100, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-scheduler operator should be Available in 1500 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-scheduler", 1500, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-scheduler operator is not becomes available in 1500 seconds")

		//Get the kube-scheduler pod name and check logs
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-kube-scheduler", "pods", "-l", "app=openshift-kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		schedulerLogs, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args(podName, "-n", "openshift-kube-scheduler").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("score.*\n.*disabled.*\n.*name:.'*'.*\n.*weight.*0.*", schedulerLogs); !match {
			e2e.Failf("Enabling NoScoring Profile failed: %v", err)
		}
	})

	// author: knarra@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:knarra-High-60542-Guard controller set the readiness probe endpoint explicitly", func() {
		// If SNO cluster skip the case as there is no quorum guard pod present in there
		exutil.SkipForSNOCluster(oc)

		// Check if openshift-kube-apiserver guard pod endpoint has been set to readyz
		g.By("Check if all guard pods in openshift-kube-apiserver namespace are running fine")
		guardPodName, guardPodError := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", "openshift-kube-apiserver", "-l=app=guard", `-ojsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
		o.Expect(guardPodError).NotTo(o.HaveOccurred())

		guardPodNames := strings.Fields(guardPodName)
		if len(guardPodNames) != 3 {
			e2e.Failf("All guard pods inside openshift-kube-apiserver namespace are not running fine")
		}

		g.By("Check if guard pod path is set to readyz instead of healthz")

		guardPodOutput, guardPodOutputError := oc.WithoutNamespace().AsAdmin().Run("get").Args("po", guardPodNames[0], "-n", "openshift-kube-apiserver", "-o", "yaml").Output()
		o.Expect(guardPodOutputError).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("readyz", guardPodOutput); !match {
			e2e.Failf("Openshift api server guard pod probe endpoint has not been set to readyz")
		}

		// Check if openshift-kube-scheduler guard pod endpoint has been set to healthz
		g.By("Check if all guard pods in openshift-kube-scheduler namespace are running fine")
		guardPodName, guardPodError = oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", "openshift-kube-scheduler", "-l=app=guard", `-ojsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
		o.Expect(guardPodError).NotTo(o.HaveOccurred())

		guardPodNames = strings.Fields(guardPodName)
		if len(guardPodNames) != 3 {
			e2e.Failf("All guard pods inside openshift-kube-apiserver namespace are not running fine")
		}

		g.By("Check if guard pod path in openshift-kube-scheduler namespace is set to healthz")

		guardPodOutput, guardPodOutputError = oc.WithoutNamespace().AsAdmin().Run("get").Args("po", guardPodNames[0], "-n", "openshift-kube-scheduler", "-o", "yaml").Output()
		o.Expect(guardPodOutputError).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("healthz", guardPodOutput); !match {
			e2e.Failf("Openshift kube scheduler guard pod probe endpoint has not been set to healthz")
		}

		// Check if openshift-kube-controller-manager guard pod endpoint has been set to healthz
		g.By("Check if all guard pods in openshift-kube-controller-manager namespace are running fine")
		guardPodName, guardPodError = oc.WithoutNamespace().AsAdmin().Run("get").Args("po", "-n", "openshift-kube-controller-manager", "-l=app=guard", `-ojsonpath={.items[?(@.status.phase=="Running")].metadata.name}`).Output()
		o.Expect(guardPodError).NotTo(o.HaveOccurred())

		guardPodNames = strings.Fields(guardPodName)
		if len(guardPodNames) != 3 {
			e2e.Failf("All guard pods inside openshift-kube-apiserver namespace are not running fine")
		}

		g.By("Check if guard pod path in openshift-controller-manager namespace is set to healthz")

		guardPodOutput, guardPodOutputError = oc.WithoutNamespace().AsAdmin().Run("get").Args("po", guardPodNames[0], "-n", "openshift-kube-controller-manager", "-o", "yaml").Output()
		o.Expect(guardPodOutputError).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("healthz", guardPodOutput); !match {
			e2e.Failf("Openshift kube controller manager guard pod probe endpoint has not been set to healthz")
		}

	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Critical-60691-Validate DynamicResourceAllocation feature gate is enabled with TPNoUpgrade", func() {
		g.By("Check if the cluster is TechPreviewNoUpgrade")
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip for featuregate set as TechPreviewNoUpgrade")
		}
		// Get kubecontrollermanager pod name & check if the feature gate is enabled
		kcmPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-controller-manager", "-l", "app=kube-controller-manager", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		kcmPodOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", kcmPodName, "-n", "openshift-kube-controller-manager", "-o=jsonpath={.spec.containers[0].args}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(kcmPodOut, "--feature-gates=DynamicResourceAllocation=true")).To(o.BeTrue())

		// Get kubescheduler pod name & check if the feature gate is enabled
		ksPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-scheduler", "-l", "app=openshift-kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ksPodOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ksPodName, "-n", "openshift-kube-scheduler", "-o=jsonpath={.spec.containers[0].args}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ksPodOut, "DynamicResourceAllocation=true")).To(o.BeTrue())

		// Get kubeapiserver pod name & check if the feature gate is enabled
		kaPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-apiserver", "-l", "app=openshift-kube-apiserver", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		kaPodOut, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("pod/"+kaPodName, "-n", "openshift-kube-apiserver").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(kaPodOut, "DynamicResourceAllocation=true")).To(o.BeTrue())

		// Verify if featuregate is enabled for kubelet
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())
		kubeletOutput, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+masterNodes[0], "-n", "openshift-kube-scheduler", "--", "chroot", "/host", "cat", "/etc/kubernetes/kubelet.conf").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(kubeletOutput, `"DynamicResourceAllocation": true`)).To(o.BeTrue())

	})

	// author: knarra@redhat.com
	//It is destructive case, will make kube-scheduler roll out, so adding [Disruptive]. One rollout costs about 5mins, so adding [Slow]
	g.It("HyperShiftMGMT-Longduration-NonPreRelease-Author:knarra-High-67153-Validate highNodeUtilization,noScoring,lowNodeUtilization profile on hypershift clusters [Disruptive][Slow]", func() {
		guestClusterName, _, hostedClusterName := exutil.ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc)
		hostedClusterNS := hostedClusterName + "-" + guestClusterName
		e2e.Logf("hostedClusterNS is %s", hostedClusterNS)

		patchYamlToRestore := `[{"op": "remove", "path": "/spec/configuration"}]`

		g.By("Set profile to HighNodeUtilization")
		patchYamlHighNodeUtilization := `[{"op": "add", "path": "/spec/configuration", "value":{"scheduler":{"profile":"HighNodeUtilization"}}}]`
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterName, "--type=json", "-p", patchYamlHighNodeUtilization).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			e2e.Logf("Restoring the scheduler cluster's logLevel")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterName, "--type=json", "-p", patchYamlToRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("Check all the kube-scheduler pods in the hosted cluster namespace should be up and running")
			waitForDeploymentPodsToBeReady(oc, hostedClusterNS, "kube-scheduler")

		}()

		g.By("Wait for kube-scheduler pods to restart and run fine")
		waitForDeploymentPodsToBeReady(oc, hostedClusterNS, "kube-scheduler")

		//Get the kube-scheduler pod name & check logs
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", hostedClusterNS, "pods", "-l", "app=kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		schedulerLogs, err := oc.WithoutNamespace().AsAdmin().Run("logs").Args(podName, "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("score.*\n.*disabled.*\n.*NodeResourcesBalancedAllocation.*\n.*weight.*0.*", schedulerLogs); !match {
			e2e.Failf("Enabling HighNodeUtilization Profile failed: %v", err)
		}

		g.By("Set profile to NoScoring")
		patchYamlNoScoring := `[{"op": "add", "path": "/spec/configuration", "value":{"scheduler":{"profile":"NoScoring"}}}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("hostedcluster", guestClusterName, "-n", hostedClusterName, "--type=json", "-p", patchYamlNoScoring).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Wait for kube-scheduler pods to restart and run fine")
		waitForDeploymentPodsToBeReady(oc, hostedClusterNS, "kube-scheduler")

		//Get the kube-scheduler pod name & check logs
		podName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", hostedClusterNS, "pods", "-l", "app=kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		schedulerLogs, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args(podName, "-n", hostedClusterNS).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if match, _ := regexp.MatchString("score.*\n.*disabled.*\n.*name:.'*'.*\n.*weight.*0.*", schedulerLogs); !match {
			e2e.Failf("Enabling NoScoring Profile failed: %v", err)
		}

	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-64819-Validate MatchLabelKeysInPodTopologySpread feature is not set when TechPreviewNoUpgrade is enabled", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deployment60691Yaml := filepath.Join(buildPruningBaseDir, "deployment60691.yaml")
		nodeZeroOccurences := 0
		nodeOneOccurences := 0

		g.By("Check if the cluster is TechPreviewNoUpgrade")
		if !isTechPreviewNoUpgrade(oc) {
			g.Skip("Skip for featuregate set as TechPreviewNoUpgrade")
		}

		//Retrieve worker nodes from the cluster
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("\nNode Names are %v", nodeName)
		node := strings.Fields(nodeName)

		// If no.of workernodes are less than three, skip the test.
		nodeNum := 3
		if len(node) < nodeNum {
			g.Skip("Not enough worker nodes for this test, skip the case!!")
		}

		// Get kubescheduler pod name & check if the feature gate is enabled
		ksPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-kube-scheduler", "-l", "app=openshift-kube-scheduler", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		ksPodOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", ksPodName, "-n", "openshift-kube-scheduler", "-o=jsonpath={.spec.containers[0].args}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(ksPodOut, "MatchLabelKeysInPodTopologySpread=true")).To(o.BeFalse())

		g.By("Add label to the nodes so that pods could run fine")
		defer removeLabelFromNode(oc, "ocp64819-zone-", node[0], "nodes")
		addLabelToNode(oc, "ocp64819-zone=ocp64819zoneA", node[0], "nodes")
		defer removeLabelFromNode(oc, "ocp64819-zone-", node[1], "nodes")
		addLabelToNode(oc, "ocp64819-zone=ocp64819zoneA", node[1], "nodes")
		defer removeLabelFromNode(oc, "ocp64819-zone-", node[2], "nodes")
		addLabelToNode(oc, "ocp64819-zone=ocp64819zoneB", node[2], "nodes")

		// Create test project
		g.By("Create test project")
		oc.SetupProject()

		g.By("Create deployment and see that they violate max skew")
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deployment60691Yaml, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForDeploymentPodsToBeReady(oc, oc.Namespace(), "app-ocp64819")

		// Rollout and wait for the deployment to restart
		g.By("Rollout/restart deployment")
		err = oc.AsAdmin().WithoutNamespace().Run("rollout").Args("restart", "deployment", "app-ocp64819", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Wait for the pods to be running after rollout
		g.By("Wait for the pods to be running after rollout")
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf("Fail to get pods in the namespace %s, error: %s. Trying again", oc.Namespace(), err)
				return false, nil
			}
			if !strings.Contains("Terminating", output) && !strings.Contains("ContainerCreating", output) {
				e2e.Logf("All the pods have started running:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Pods have not started running even waiting for about 60 seconds")

		g.By("Get pod nodes using label")
		podNodeList := getPodNodeListByLabel(oc, oc.Namespace(), "app=app-ocp64819")
		for _, podNode := range podNodeList {
			if strings.Compare(podNode, string(node[0])) == 0 || strings.Compare(podNode, string(node[1])) == 0 {
				nodeZeroOccurences = nodeZeroOccurences + 1
			} else {
				nodeOneOccurences = nodeOneOccurences + 1
			}
		}
		currentMaxSkew := nodeZeroOccurences - nodeOneOccurences
		if currentMaxSkew > 1 || currentMaxSkew < 0 {
			e2e.Logf("Pods violate currentMaxSkew, which is expected %s", string(currentMaxSkew))
		}

		// Patch the deployment
		patchYamlLabelKeys := `[{"op": "add", "path": "/spec/template/spec/topologySpreadConstraints/0/matchLabelKeys", "value": ["app", "pod-template-hash"]}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", "app-ocp64819", "-n", oc.Namespace(), "--type=json", "-p", patchYamlLabelKeys).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Increase the replica number
		patchYamlReplicas := `[{"op": "replace", "path": "/spec/replicas", "value": 8}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("deployment", "app-ocp64819", "-n", oc.Namespace(), "--type=json", "-p", patchYamlReplicas).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Rollout and wait for the deployment to restart
		g.By("Rollout/restart deployment")
		err = oc.AsAdmin().WithoutNamespace().Run("rollout").Args("restart", "deployment", "app-ocp64819", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Wait for the pods to be running after rollout
		//waitForDeploymentPodsToBeReady(oc, oc.Namespace(), "app-ocp64819")
		g.By("Wait for the pods to be running after rollout")
		err = wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf("Fail to get pods in the namespace %s, error: %s. Trying again", oc.Namespace(), err)
				return false, nil
			}
			if !strings.Contains("Terminating", output) && !strings.Contains("ContainerCreating", output) {
				e2e.Logf("All the pods have started running:\n%s", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Pods have not started running even waiting for about 60 seconds")

		g.By("Get pod nodes using label")
		nodeZeroOccurencesAR := 0
		nodeOneOccurencesAR := 0
		podNodeList = getPodNodeListByLabel(oc, oc.Namespace(), "app=app-ocp64819")
		for _, podNode := range podNodeList {
			if strings.Compare(podNode, string(node[0])) == 0 || strings.Compare(podNode, string(node[1])) == 0 {
				nodeZeroOccurencesAR = nodeZeroOccurencesAR + 1
			} else {
				nodeOneOccurencesAR = nodeOneOccurencesAR + 1
			}
		}
		currentMaxSkewAR := nodeZeroOccurencesAR - nodeOneOccurencesAR
		if currentMaxSkewAR > 1 || currentMaxSkewAR < 0 {
			e2e.Failf("Pods violate currentMaxSkew, which is not expected %s", string(currentMaxSkewAR))
		}

	})

})
