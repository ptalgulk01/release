package node

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/sjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"k8s.io/apimachinery/pkg/util/wait"

	//e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
)

var _ = g.Describe("[sig-node] NODE initContainer policy,volume,readines,quota", func() {
	defer g.GinkgoRecover()

	var (
		oc                        = exutil.NewCLI("node-"+getRandomString(), exutil.KubeConfigPath())
		buildPruningBaseDir       = exutil.FixturePath("testdata", "node")
		customTemp                = filepath.Join(buildPruningBaseDir, "pod-modify.yaml")
		podTerminationTemp        = filepath.Join(buildPruningBaseDir, "pod-termination.yaml")
		podInitConTemp            = filepath.Join(buildPruningBaseDir, "pod-initContainer.yaml")
		podSleepTemp              = filepath.Join(buildPruningBaseDir, "sleepPod46306.yaml")
		kubeletConfigTemp         = filepath.Join(buildPruningBaseDir, "kubeletconfig-hardeviction.yaml")
		memHogTemp                = filepath.Join(buildPruningBaseDir, "mem-hog-ocp11600.yaml")
		podTwoContainersTemp      = filepath.Join(buildPruningBaseDir, "pod-with-two-containers.yaml")
		podUserNSTemp             = filepath.Join(buildPruningBaseDir, "pod-user-namespace.yaml")
		ctrcfgOverlayTemp         = filepath.Join(buildPruningBaseDir, "containerRuntimeConfig-overlay.yaml")
		podHelloTemp              = filepath.Join(buildPruningBaseDir, "pod-hello.yaml")
		podWkloadCpuTemp          = filepath.Join(buildPruningBaseDir, "pod-workload-cpu.yaml")
		podWkloadCpuNoAnTemp      = filepath.Join(buildPruningBaseDir, "pod-workload-cpu-without-anotation.yaml")
		podNoWkloadCpuTemp        = filepath.Join(buildPruningBaseDir, "pod-without-workload-cpu.yaml")
		runtimeTimeoutTemp        = filepath.Join(buildPruningBaseDir, "kubeletconfig-runReqTout.yaml")
		upgradeMachineConfigTemp1 = filepath.Join(buildPruningBaseDir, "custom-kubelet-test1.yaml")
		upgradeMachineConfigTemp2 = filepath.Join(buildPruningBaseDir, "custom-kubelet-test2.yaml")
		systemreserveTemp         = filepath.Join(buildPruningBaseDir, "kubeletconfig-defaultsysres.yaml")
		podLogLinkTemp            = filepath.Join(buildPruningBaseDir, "pod-loglink.yaml")
		livenessProbeTemp         = filepath.Join(buildPruningBaseDir, "livenessProbe-terminationPeriod.yaml")

		podDisruptionBudgetTemp = filepath.Join(buildPruningBaseDir, "pod-disruption-budget.yaml")
		genericDeploymentTemp   = filepath.Join(buildPruningBaseDir, "generic-deployment.yaml")

		podLogLink65404 = podLogLinkDescription{
			name:      "",
			namespace: "",
			template:  podLogLinkTemp,
		}

		podWkloadCpu52313 = podNoWkloadCpuDescription{
			name:      "",
			namespace: "",
			template:  podNoWkloadCpuTemp,
		}

		podWkloadCpu52326 = podWkloadCpuDescription{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuTemp,
		}

		podWkloadCpu52328 = podWkloadCpuDescription{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuTemp,
		}

		podWkloadCpu52329 = podWkloadCpuNoAnotation{
			name:        "",
			namespace:   "",
			workloadcpu: "",
			template:    podWkloadCpuNoAnTemp,
		}

		podHello = podHelloDescription{
			name:      "",
			namespace: "",
			template:  podHelloTemp,
		}

		podUserNS47663 = podUserNSDescription{
			name:      "",
			namespace: "",
			template:  podUserNSTemp,
		}

		podModify = podModifyDescription{
			name:          "",
			namespace:     "",
			mountpath:     "",
			command:       "",
			args:          "",
			restartPolicy: "",
			user:          "",
			role:          "",
			level:         "",
			template:      customTemp,
		}

		podTermination = podTerminationDescription{
			name:      "",
			namespace: "",
			template:  podTerminationTemp,
		}

		podInitCon38271 = podInitConDescription{
			name:      "",
			namespace: "",
			template:  podInitConTemp,
		}

		podSleep = podSleepDescription{
			namespace: "",
			template:  podSleepTemp,
		}

		kubeletConfig = kubeletConfigDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   kubeletConfigTemp,
		}

		memHog = memHogDescription{
			name:       "",
			namespace:  "",
			labelkey:   "",
			labelvalue: "",
			template:   memHogTemp,
		}

		podTwoContainers = podTwoContainersDescription{
			name:      "",
			namespace: "",
			template:  podTwoContainersTemp,
		}

		ctrcfgOverlay = ctrcfgOverlayDescription{
			name:     "",
			overlay:  "",
			template: ctrcfgOverlayTemp,
		}

		runtimeTimeout = runtimeTimeoutDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   runtimeTimeoutTemp,
		}

		upgradeMachineconfig1 = upgradeMachineconfig1Description{
			name:     "",
			template: upgradeMachineConfigTemp1,
		}
		upgradeMachineconfig2 = upgradeMachineconfig2Description{
			name:     "",
			template: upgradeMachineConfigTemp2,
		}
		systemReserveES = systemReserveESDescription{
			name:       "",
			labelkey:   "",
			labelvalue: "",
			template:   systemreserveTemp,
		}
	)
	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12893-Init containers with restart policy Always", func() {
		oc.SetupProject()
		podModify.name = "init-always-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "Always"

		g.By("create FAILED init container with pod restartPolicy Always")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain CrashLoopBackOff")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy Always")

		podModify.name = "init-always-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Always"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12894-Init containers with restart policy OnFailure", func() {
		oc.SetupProject()
		podModify.name = "init-onfailure-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "OnFailure"

		g.By("create FAILED init container with pod restartPolicy OnFailure")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain CrashLoopBackOff")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy OnFailure")

		podModify.name = "init-onfailure-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "OnFailure"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12896-Init containers with restart policy Never", func() {
		oc.SetupProject()
		podModify.name = "init-never-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "exit 1"
		podModify.restartPolicy = "Never"

		g.By("create FAILED init container with pod restartPolicy Never")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusterminatedReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain Error")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with pod restartPolicy Never")

		podModify.name = "init-never-succ"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12911-App container status depends on init containers exit code	", func() {
		oc.SetupProject()
		podModify.name = "init-fail"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/false"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		g.By("create FAILED init container with exit code and command /bin/false")
		podModify.create(oc)
		g.By("Check pod failure reason")
		err := podStatusterminatedReason(oc)
		exutil.AssertWaitPollNoErr(err, "pod status does not contain Error")
		g.By("Delete Pod ")
		podModify.delete(oc)

		g.By("create SUCCESSFUL init container with command /bin/true")
		podModify.name = "init-success"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/true"
		podModify.args = "sleep 30"
		podModify.restartPolicy = "Never"

		podModify.create(oc)
		g.By("Check pod Status")
		err = podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Delete Pod ")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Author:pmali-High-12913-Init containers with volume work fine", func() {

		oc.SetupProject()
		podModify.name = "init-volume"
		podModify.namespace = oc.Namespace()
		podModify.mountpath = "/init-test"
		podModify.command = "/bin/bash"
		podModify.args = "echo This is OCP volume test > /work-dir/volume-test"
		podModify.restartPolicy = "Never"

		g.By("Create a pod with initContainer using volume\n")
		podModify.create(oc)
		g.By("Check pod status")
		err := podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Check Vol status\n")
		err = volStatus(oc)
		exutil.AssertWaitPollNoErr(err, "Init containers with volume do not work fine")
		g.By("Delete Pod\n")
		podModify.delete(oc)
	})

	// author: pmali@redhat.com
	g.It("Author:pmali-Medium-30521-CRIO Termination Grace Period test", func() {

		oc.SetupProject()
		podTermination.name = "pod-termination"
		podTermination.namespace = oc.Namespace()

		g.By("Create a pod with termination grace period\n")
		podTermination.create(oc)
		g.By("Check pod status\n")
		err := podStatus(oc, podTermination.namespace, podTermination.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Check container TimeoutStopUSec\n")
		err = podTermination.getTerminationGrace(oc)
		exutil.AssertWaitPollNoErr(err, "terminationGracePeriodSeconds is not valid")
		g.By("Delete Pod\n")
		podTermination.delete(oc)
	})

	// author: minmli@redhat.com
	g.It("Author:minmli-High-38271-Init containers should not restart when the exited init container is removed from node", func() {
		g.By("Test for case OCP-38271")
		oc.SetupProject()
		podInitCon38271.name = "initcon-pod"
		podInitCon38271.namespace = oc.Namespace()

		g.By("Create a pod with init container")
		podInitCon38271.create(oc)
		defer podInitCon38271.delete(oc)

		g.By("Check pod status")
		err := podStatus(oc, podInitCon38271.namespace, podInitCon38271.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check init container exit normally")
		err = podInitCon38271.containerExit(oc)
		exutil.AssertWaitPollNoErr(err, "conainer not exit normally")

		g.By("Delete init container")
		_, err = podInitCon38271.deleteInitContainer(oc)
		exutil.AssertWaitPollNoErr(err, "fail to delete container")

		g.By("Check init container not restart again")
		err = podInitCon38271.initContainerNotRestart(oc)
		exutil.AssertWaitPollNoErr(err, "init container restart")
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-NonPreRelease-Longduration-Author:pmali-High-46306-Node should not becomes NotReady with error creating container storage layer not known[Disruptive][Slow]", func() {

		oc.SetupProject()
		podSleep.namespace = oc.Namespace()

		g.By("Get Worker Node and Add label app=sleep\n")
		workerNodeName := getSingleWorkerNode(oc)
		addLabelToNode(oc, "app=sleep", workerNodeName, "nodes")
		defer removeLabelFromNode(oc, "app-", workerNodeName, "nodes")

		g.By("Create a 50 pods on the same node\n")
		for i := 0; i < 50; i++ {
			podSleep.create(oc)
		}

		g.By("Check pod status\n")
		err := podStatus(oc, podModify.namespace, podModify.name)
		exutil.AssertWaitPollNoErr(err, "pod is NOT running")

		g.By("Delete project\n")
		go podSleep.deleteProject(oc)

		g.By("Reboot Worker node\n")
		go rebootNode(oc, workerNodeName)

		//g.By("****** Reboot Worker Node ****** ")
		//exutil.DebugNodeWithChroot(oc, workerNodeName, "reboot")

		g.By("Check Nodes Status\n")
		err = checkNodeStatus(oc, workerNodeName)
		exutil.AssertWaitPollNoErr(err, "node is not ready")

		g.By("Get Master node\n")
		masterNode := getSingleMasterNode(oc)

		g.By("Check Master Node Logs\n")
		err = masterNodeLog(oc, masterNode)
		exutil.AssertWaitPollNoErr(err, "Logs Found, Test Failed")
	})

	// author: pmali@redhat.com
	g.It("DEPRECATED-Longduration-NonPreRelease-Author:pmali-Medium-11600-kubelet will evict pod immediately when met hard eviction threshold memory [Disruptive][Slow]", func() {

		oc.SetupProject()
		kubeletConfig.name = "kubeletconfig-ocp11600"
		kubeletConfig.labelkey = "custom-kubelet-ocp11600"
		kubeletConfig.labelvalue = "hard-eviction"

		memHog.name = "mem-hog-ocp11600"
		memHog.namespace = oc.Namespace()
		memHog.labelkey = kubeletConfig.labelkey
		memHog.labelvalue = kubeletConfig.labelvalue

		g.By("Get Worker Node and Add label custom-kubelet-ocp11600=hard-eviction\n")
		addLabelToNode(oc, "custom-kubelet-ocp11600=hard-eviction", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-ocp11600-", "worker", "mcp")

		g.By("Create Kubelet config \n")
		kubeletConfig.create(oc)
		defer getmcpStatus(oc, "worker") // To check all the Nodes are in Ready State after deleteing kubeletconfig
		defer cleanupObjectsClusterScope(oc, objectTableRefcscope{"kubeletconfig", "kubeletconfig-ocp11600"})

		g.By("Make sure Worker mcp is Updated correctly\n")
		err := getmcpStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "mcp is not updated")

		g.By("Create a 10 pods on the same node\n")
		for i := 0; i < 10; i++ {
			memHog.create(oc)
		}
		defer cleanupObjectsClusterScope(oc, objectTableRefcscope{"ns", oc.Namespace()})

		g.By("Check worker Node events\n")
		workerNodeName := getSingleWorkerNode(oc)
		err = getWorkerNodeDescribe(oc, workerNodeName)
		exutil.AssertWaitPollNoErr(err, "Logs did not Found memory pressure, Test Failed")
	})

	// author: weinliu@redhat.com
	g.It("Author:weinliu-Critical-11055-/dev/shm can be automatically shared among all of a pod's containers", func() {
		g.By("Test for case OCP-11055")
		oc.SetupProject()
		podTwoContainers.name = "pod-twocontainers"
		podTwoContainers.namespace = oc.Namespace()
		g.By("Create a pod with two containers")
		podTwoContainers.create(oc)
		defer podTwoContainers.delete(oc)
		g.By("Check pod status")
		err := podStatus(oc, podTwoContainers.namespace, podTwoContainers.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")
		g.By("Enter container 1 and write files")
		_, err = exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift", "echo 'written_from_container1' > /dev/shm/c1")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Enter container 2 and check whether it can share container 1 shared files")
		containerFile1, err := exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift-fedora", "cat /dev/shm/c1")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Container1 File Content is: %v", containerFile1)
		o.Expect(containerFile1).To(o.Equal("written_from_container1"))
		g.By("Enter container 2 and write files")
		_, err = exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift-fedora", "echo 'written_from_container2' > /dev/shm/c2")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Enter container 1 and check whether it can share container 2 shared files")
		containerFile2, err := exutil.RemoteShPodWithBashSpecifyContainer(oc, podTwoContainers.namespace, podTwoContainers.name, "hello-openshift", "cat /dev/shm/c2")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Container2 File Content is: %v", containerFile2)
		o.Expect(containerFile2).To(o.Equal("written_from_container2"))
	})

	// author: minmli@redhat.com
	g.It("Author:minmli-High-47663-run pods in user namespaces via crio workload annotation", func() {
		oc.SetupProject()
		g.By("Test for case OCP-47663")
		podUserNS47663.name = "userns-47663"
		podUserNS47663.namespace = oc.Namespace()

		g.By("Check workload of openshift-builder exist in crio config")
		err := podUserNS47663.crioWorkloadConfigExist(oc)
		exutil.AssertWaitPollNoErr(err, "crio workload config not exist")

		g.By("Check user containers exist in /etc/sub[ug]id")
		err = podUserNS47663.userContainersExistForNS(oc)
		exutil.AssertWaitPollNoErr(err, "user containers not exist for user namespace")

		g.By("Create a pod with annotation of openshift-builder workload")
		podUserNS47663.createPodUserNS(oc)
		defer podUserNS47663.deletePodUserNS(oc)

		g.By("Check pod status")
		err = podStatus(oc, podUserNS47663.namespace, podUserNS47663.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check pod run in user namespace")
		err = podUserNS47663.podRunInUserNS(oc)
		exutil.AssertWaitPollNoErr(err, "pod not run in user namespace")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-52328-set workload resource usage from pod level : pod should not take effect if not defaulted or specified in workload [Disruptive][Slow]", func() {
		oc.SetupProject()
		exutil.By("Test for case OCP-52328")

		exutil.By("Create a machine config for workload setting")
		mcCpuOverride := filepath.Join(buildPruningBaseDir, "machineconfig-cpu-override-52328.yaml")
		mcpName := "worker"
		defer func() {
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcCpuOverride).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcCpuOverride).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check mcp finish rolling out")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check workload setting is as expected")
		wkloadConfig := []string{"crio.runtime.workloads.management", "activation_annotation = \"io.openshift.manager\"", "annotation_prefix = \"io.openshift.workload.manager\"", "crio.runtime.workloads.management.resources", "cpushares = 512"}
		configPath := "/etc/crio/crio.conf.d/01-workload.conf"
		err = crioConfigExist(oc, wkloadConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "workload setting is not set as expected")

		exutil.By("Create a pod not specify cpuset in workload setting by annotation")
		defer podWkloadCpu52328.delete(oc)
		podWkloadCpu52328.name = "wkloadcpu-52328"
		podWkloadCpu52328.namespace = oc.Namespace()
		podWkloadCpu52328.workloadcpu = "{\"cpuset\": \"\", \"cpushares\": 1024}"
		podWkloadCpu52328.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc, podWkloadCpu52328.namespace, podWkloadCpu52328.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check the pod only override cpushares")
		cpuset := ""
		err = overrideWkloadCpu(oc, cpuset, podWkloadCpu52328.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not only override cpushares in workload setting")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-52313-High-52326-High-52329-set workload resource usage from pod level : pod can get configured to defaults and override defaults and pod should not be set if annotation not specified [Disruptive][Slow]", func() {
		oc.SetupProject()
		exutil.By("Test for case OCP-52313, OCP-52326 and OCP-52329")

		exutil.By("Create a machine config for workload setting")
		mcCpuOverride := filepath.Join(buildPruningBaseDir, "machineconfig-cpu-override.yaml")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcCpuOverride).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcCpuOverride).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check mcp finish rolling out")
		mcpName := "worker"
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check workload setting is as expected")
		wkloadConfig := []string{"crio.runtime.workloads.management", "activation_annotation = \"io.openshift.manager\"", "annotation_prefix = \"io.openshift.workload.manager\"", "crio.runtime.workloads.management.resources", "cpushares = 512", "cpuset = \"0\""}
		configPath := "/etc/crio/crio.conf.d/01-workload.conf"
		err = crioConfigExist(oc, wkloadConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "workload setting is not set as expected")

		exutil.By("Create a pod with default workload setting by annotation")
		podWkloadCpu52313.name = "wkloadcpu-52313"
		podWkloadCpu52313.namespace = oc.Namespace()
		podWkloadCpu52313.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc, podWkloadCpu52313.namespace, podWkloadCpu52313.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check the pod get configured to default workload setting")
		cpuset := "0"
		err = overrideWkloadCpu(oc, cpuset, podWkloadCpu52313.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod is not configured to default workload setting")
		podWkloadCpu52313.delete(oc)

		exutil.By("Create a pod override the default workload setting by annotation")
		podWkloadCpu52326.name = "wkloadcpu-52326"
		podWkloadCpu52326.namespace = oc.Namespace()
		podWkloadCpu52326.workloadcpu = "{\"cpuset\": \"0-1\", \"cpushares\": 200}"
		podWkloadCpu52326.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc, podWkloadCpu52326.namespace, podWkloadCpu52326.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check the pod override the default workload setting")
		cpuset = "0-1"
		err = overrideWkloadCpu(oc, cpuset, podWkloadCpu52326.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not override the default workload setting")
		podWkloadCpu52326.delete(oc)

		exutil.By("Create a pod without annotation but with prefix")
		defer podWkloadCpu52329.delete(oc)
		podWkloadCpu52329.name = "wkloadcpu-52329"
		podWkloadCpu52329.namespace = oc.Namespace()
		podWkloadCpu52329.workloadcpu = "{\"cpuset\": \"0-1\", \"cpushares\": 1800}"
		podWkloadCpu52329.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc, podWkloadCpu52329.namespace, podWkloadCpu52329.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check the pod keep default workload setting")
		cpuset = "0-1"
		err = defaultWkloadCpu(oc, cpuset, podWkloadCpu52329.namespace)
		exutil.AssertWaitPollNoErr(err, "the pod not keep efault workload setting")
	})

	// author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-46313-set overlaySize in containerRuntimeConfig should take effect in container [Disruptive][Slow]", func() {
		oc.SetupProject()
		g.By("Test for case OCP-46313")
		ctrcfgOverlay.name = "ctrcfg-46313"
		ctrcfgOverlay.overlay = "9G"

		g.By("Create a containerRuntimeConfig to set overlaySize")
		ctrcfgOverlay.create(oc)
		defer func() {
			g.By("Deleting configRuntimeConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"ContainerRuntimeConfig", "ctrcfg-46313"})
			g.By("Check mcp finish rolling out")
			err := getmcpStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		g.By("Check mcp finish rolling out")
		err := getmcpStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "mcp is not updated")

		g.By("Check overlaySize take effect in config file")
		err = checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize not take effect")

		g.By("Create a pod")
		podTermination.name = "pod-46313"
		podTermination.namespace = oc.Namespace()
		podTermination.create(oc)
		defer podTermination.delete(oc)

		g.By("Check pod status")
		err = podStatus(oc, podTermination.namespace, podTermination.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Check in pod the root partition size for Overlay is correct.")
		err = checkPodOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "pod overlay size is not correct !!!")
	})

	g.It("Author:minmli-High-56266-kubelet/crio will delete netns when a pod is deleted", func() {
		g.By("Test for case OCP-56266")
		oc.SetupProject()

		g.By("Create a pod")
		podHello.name = "pod-56266"
		podHello.namespace = oc.Namespace()
		podHello.create(oc)

		g.By("Check pod status")
		err := podStatus(oc, podHello.namespace, podHello.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		g.By("Get Pod's Node name")
		hostname := getPodNodeName(oc, podHello.namespace)

		g.By("Get Pod's NetNS")
		netNsPath, err := getPodNetNs(oc, hostname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Delete the pod")
		podHello.delete(oc)

		g.By("Check the NetNs file was cleaned")
		err = checkNetNs(oc, hostname, netNsPath)
		exutil.AssertWaitPollNoErr(err, "the NetNs file is not cleaned !!!")
	})

	g.It("Author:minmli-High-55486-check not exist error MountVolume SetUp failed for volume serviceca object openshift-image-registry serviceca not registered", func() {
		g.By("Test for case OCP-55486")
		oc.SetupProject()

		g.By("Check events of each cronjob")
		err := checkEventsForErr(oc)
		exutil.AssertWaitPollNoErr(err, "Found error: MountVolume.SetUp failed for volume ... not registered ")
	})
	//author: asahay@redhat.com
	g.It("Author:asahay-Medium-55033-check KUBELET_LOG_LEVEL is 2", func() {
		g.By("Test for OCP-55033")
		g.By("check Kubelet Log Level\n")
		assertKubeletLogLevel(oc)
	})

	//author: asahay@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:asahay-High-52472-update runtimeRequestTimeout parameter using KubeletConfig CR [Disruptive][Slow]", func() {

		oc.SetupProject()
		runtimeTimeout.name = "kubeletconfig-52472"
		runtimeTimeout.labelkey = "custom-kubelet"
		runtimeTimeout.labelvalue = "test-timeout"

		g.By("Label mcp worker custom-kubelet as test-timeout \n")
		addLabelToNode(oc, "custom-kubelet=test-timeout", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-", "worker", "mcp")

		g.By("Create KubeletConfig \n")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer runtimeTimeout.delete(oc)
		runtimeTimeout.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName := "worker"
		err := checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Check Runtime Request Timeout")
		runTimeTimeout(oc)
	})

	//author :asahay@redhat.com

	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:asahay-High-45436-Upgrading a cluster by making sure not keep duplicate machine config when it has multiple kubeletconfig [Disruptive][Slow]", func() {

		upgradeMachineconfig1.name = "max-pod"
		upgradeMachineconfig2.name = "max-pod-1"
		g.By("Create first KubeletConfig \n")
		upgradeMachineconfig1.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName := "worker"
		err := checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		g.By("Create second KubeletConfig \n")
		upgradeMachineconfig2.create(oc)

		g.By("Check mcp finish rolling out")
		mcpName1 := "worker"
		err1 := checkMachineConfigPoolStatus(oc, mcpName1)
		exutil.AssertWaitPollNoErr(err1, "macineconfigpool worker update failed")

	})

	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:asahay-High-45436-post check Upgrading a cluster by making sure not keep duplicate machine config when it has multiple kubeletconfig [Disruptive][Slow]", func() {
		upgradeMachineconfig1.name = "max-pod"
		defer func() {
			g.By("Delete the KubeletConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"KubeletConfig", upgradeMachineconfig1.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		upgradeMachineconfig2.name = "max-pod-1"
		defer func() {
			g.By("Delete the KubeletConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"KubeletConfig", upgradeMachineconfig2.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()
		g.By("Checking no duplicate machine config")
		checkUpgradeMachineConfig(oc)

	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PreChkUpgrade-Author:minmli-High-45351-prepare to check crioConfig[Disruptive][Slow]", func() {
		rhelWorkers, err := exutil.GetAllWorkerNodesByOSID(oc, "rhel")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(rhelWorkers) > 0 {
			g.Skip("ctrcfg.overlay can't be supported by rhel nodes")
		}

		g.By("1) oc debug one worker and edit /etc/crio/crio.conf")
		// we update log_level = "debug" in /etc/crio/crio.conf
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/log_level = \"info\"/log_level = \"debug\"/g' /etc/crio/crio.conf")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2) create a ContainerRuntimeConfig to set overlaySize")
		ctrcfgOverlay.name = "ctrcfg-45351"
		ctrcfgOverlay.overlay = "35G"
		mcpName := "worker"
		ctrcfgOverlay.create(oc)

		g.By("3) check mcp finish rolling out")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "mcp update failed")

		g.By("4) check overlaySize update as expected")
		err = checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize not update as expected")
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:minmli-High-45351-post check crioConfig[Disruptive][Slow]", func() {
		rhelWorkers, err := exutil.GetAllWorkerNodesByOSID(oc, "rhel")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(rhelWorkers) > 0 {
			g.Skip("ctrcfg.overlay can't be supported by rhel nodes")
		}

		g.By("1) check overlaySize don't change after upgrade")
		ctrcfgOverlay.name = "ctrcfg-45351"
		ctrcfgOverlay.overlay = "35G"

		defer func() {
			g.By("Delete the configRuntimeConfig")
			cleanupObjectsClusterScope(oc, objectTableRefcscope{"ContainerRuntimeConfig", ctrcfgOverlay.name})
			g.By("Check mcp finish rolling out")
			err := checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "mcp is not updated")
		}()

		defer func() {
			g.By("Restore /etc/crio/crio.conf")
			nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, node := range nodeList.Items {
				nodename := node.Name
				_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/log_level = \"debug\"/log_level = \"info\"/g' /etc/crio/crio.conf")
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		}()

		err = checkOverlaySize(oc, ctrcfgOverlay.overlay)
		exutil.AssertWaitPollNoErr(err, "overlaySize change after upgrade")

		g.By("2) check conmon value from crio config")
		//we need check every node for the conmon = ""
		checkConmonForAllNode(oc)
	})

	g.It("Author:asahay-Medium-57332-collecting the audit log with must gather", func() {

		defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-57332").Output()
		g.By("Running the must gather command \n")
		_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir=/tmp/must-gather-57332", "--", "/usr/bin/gather_audit_logs").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("check the must-gather result")
		_, err = exec.Command("bash", "-c", "ls -l /tmp/must-gather-57332").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-57401-Create ImageDigestMirrorSet successfully [Disruptive][Slow]", func() {
		//If a cluster contains any ICSP or IDMS, it will skip the case
		if checkICSP(oc) || checkIDMS(oc) {
			g.Skip("This cluster contain ICSP or IDMS, skip the test.")
		}
		exutil.By("Create an ImageDigestMirrorSet")
		idms := filepath.Join(buildPruningBaseDir, "ImageDigestMirrorSet.yaml")
		defer func() {
			err := checkMachineConfigPoolStatus(oc, "master")
			exutil.AssertWaitPollNoErr(err, "macineconfigpool master update failed")
			err = checkMachineConfigPoolStatus(oc, "worker")
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + idms).Execute()

		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + idms).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, "master")
		exutil.AssertWaitPollNoErr(err, "macineconfigpool master update failed")
		err = checkMachineConfigPoolStatus(oc, "worker")
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the ImageDigestMirrorSet apply to config")
		err = checkRegistryForIdms(oc)
		exutil.AssertWaitPollNoErr(err, "check registry config failed")

		exutil.By("The ImageContentSourcePolicy can't exist wiht ImageDigestMirrorSet or ImageTagMirrorSet")
		icsp := filepath.Join(buildPruningBaseDir, "ImageContentSourcePolicy.yaml")
		out, _ := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", icsp).Output()
		o.Expect(strings.Contains(out, "Kind.ImageContentSourcePolicy: Forbidden: can't create ImageContentSourcePolicy when ImageDigestMirrorSet resources exist")).To(o.BeTrue())
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-Author:minmli-Medium-59552-Enable image signature verification for Red Hat Container Registries [Serial]", func() {
		exutil.By("Apply a machine config to set image signature policy for worker nodes")
		mcImgSig := filepath.Join(buildPruningBaseDir, "machineconfig-image-signature-59552.yaml")
		mcpName := "worker"
		defer func() {
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcImgSig).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcImgSig).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the signature configuration policy.json")
		err = checkImgSignature(oc)
		exutil.AssertWaitPollNoErr(err, "check signature configuration failed")
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:asahay-Medium-62746-A default SYSTEM_RESERVED_ES value is applied if it is empty [Disruptive][Slow]", func() {

		exutil.By("set SYSTEM_RESERVED_ES as empty")
		nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
		o.Expect(err).NotTo(o.HaveOccurred())
		nodename := nodeList.Items[0].Name
		_, err = exutil.DebugNodeWithChroot(oc, nodename, "/bin/bash", "-c", "sed -i 's/SYSTEM_RESERVED_ES=1Gi/SYSTEM_RESERVED_ES=/g' /etc/crio/crio.conf")
		o.Expect(err).NotTo(o.HaveOccurred())

		systemReserveES.name = "kubeletconfig-62746"
		systemReserveES.labelkey = "custom-kubelet"
		systemReserveES.labelvalue = "reserve-space"

		exutil.By("Label mcp worker custom-kubelet as reserve-space \n")
		addLabelToNode(oc, "custom-kubelet=reserve-space", "worker", "mcp")
		defer removeLabelFromNode(oc, "custom-kubelet-", "worker", "mcp")

		exutil.By("Create KubeletConfig \n")
		defer func() {
			mcpName := "worker"
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		defer systemReserveES.delete(oc)
		systemReserveES.create(oc)

		exutil.By("Check mcp finish rolling out")
		mcpName := "worker"
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check Default value")
		parameterCheck(oc)
	})

	//author: minmli@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-65404-log link inside pod via crio works well [Disruptive]", func() {
		exutil.By("Apply a machine config to enable log link via crio")
		mcLogLink := filepath.Join(buildPruningBaseDir, "machineconfig-log-link.yaml")
		mcpName := "worker"
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + mcLogLink).Execute()
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + mcLogLink).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check the crio config as expected")
		logLinkConfig := []string{"crio.runtime.workloads.linked", "activation_annotation = \"io.kubernetes.cri-o.LinkLogs\"", "allowed_annotations = [ \"io.kubernetes.cri-o.LinkLogs\" ]"}
		configPath := "/etc/crio/crio.conf.d/99-linked-log.conf"
		err = crioConfigExist(oc, logLinkConfig, configPath)
		exutil.AssertWaitPollNoErr(err, "crio config is not set as expected")

		exutil.By("Create a pod with LinkLogs annotation")
		podLogLink65404.name = "httpd"
		podLogLink65404.namespace = oc.Namespace()
		err = oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", oc.Namespace(), "security.openshift.io/scc.podSecurityLabelSync=false", "pod-security.kubernetes.io/enforce=privileged", "--overwrite").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer podLogLink65404.delete(oc)
		podLogLink65404.create(oc)

		exutil.By("Check pod status")
		err = podStatus(oc, podLogLink65404.namespace, podLogLink65404.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check log link successfully")
		checkLogLink(oc, podLogLink65404.namespace)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:minmli-High-55683-Crun on OpenShift enable [Disruptive]", func() {
		exutil.By("Apply a ContarinerRuntimeConfig to enable crun")
		ctrcfgCrun := filepath.Join(buildPruningBaseDir, "containerRuntimeConfig-crun.yaml")
		mcpName := "worker"
		defer func() {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + ctrcfgCrun).Execute()
			err := checkMachineConfigPoolStatus(oc, mcpName)
			exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")
		}()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + ctrcfgCrun).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the mcp finish updating")
		err = checkMachineConfigPoolStatus(oc, mcpName)
		exutil.AssertWaitPollNoErr(err, "macineconfigpool worker update failed")

		exutil.By("Check crun is running")
		checkCrun(oc)
	})

	g.It("Author:minmli-High-68184-container_network metrics should keep reporting after container restart", func() {
		livenessProbeTermP68184 := liveProbeTermPeriod{
			name:                  "liveness-probe",
			namespace:             oc.Namespace(),
			terminationgrace:      60,
			probeterminationgrace: 10,
			template:              livenessProbeTemp,
		}

		exutil.By("Create a pod")
		defer livenessProbeTermP68184.delete(oc)
		livenessProbeTermP68184.create(oc)

		exutil.By("Check pod status")
		err := podStatus(oc, livenessProbeTermP68184.namespace, livenessProbeTermP68184.name)
		exutil.AssertWaitPollNoErr(err, "pod is not running")

		exutil.By("Check the container_network* metrics report well")
		podNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", livenessProbeTermP68184.name, "-o=jsonpath={.spec.nodeName}", "-n", livenessProbeTermP68184.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var cmdOut1 string
		var cmdOut2 string
		waitErr := wait.Poll(30*time.Second, 1*time.Minute, func() (bool, error) {
			cmd1 := fmt.Sprintf(`oc get --raw /api/v1/nodes/%v/proxy/metrics/cadvisor  | grep container_network_transmit | grep %v`, podNode, livenessProbeTermP68184.name)
			cmdOut1, err := exec.Command("bash", "-c", cmd1).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(string(cmdOut1), "container_network_transmit_bytes_total") && strings.Contains(string(cmdOut1), "container_network_transmit_errors_total") && strings.Contains(string(cmdOut1), "container_network_transmit_packets_dropped_total") && strings.Contains(string(cmdOut1), "container_network_transmit_packets_total") {
				e2e.Logf("\ncontainer_network* metrics report well after pod start")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("check metrics failed after pod start! Metric result is: \n %v \n", cmdOut1))

		exutil.By("Check the container_network* metrics still report after container restart")
		waitErr = wait.Poll(80*time.Second, 5*time.Minute, func() (bool, error) {
			restartCount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", livenessProbeTermP68184.name, "-o=jsonpath={.status.containerStatuses[0].restartCount}", "-n", livenessProbeTermP68184.namespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("restartCount is :%v", restartCount)
			o.Expect(strconv.Atoi(restartCount)).Should(o.BeNumerically(">=", 1), "error: the pod restart time < 1")

			cmd2 := fmt.Sprintf(`oc get --raw /api/v1/nodes/%v/proxy/metrics/cadvisor  | grep container_network_transmit | grep %v`, podNode, livenessProbeTermP68184.name)
			cmdOut2, err := exec.Command("bash", "-c", cmd2).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(string(cmdOut2), "container_network_transmit_bytes_total") && strings.Contains(string(cmdOut2), "container_network_transmit_errors_total") && strings.Contains(string(cmdOut2), "container_network_transmit_packets_dropped_total") && strings.Contains(string(cmdOut2), "container_network_transmit_packets_total") {
				e2e.Logf("\ncontainer_network* metrics report well after pod restart")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("check metrics failed after pod restart! Metric result is: \n %v \n", cmdOut2))
	})

	//author: jfrancoa@redhat.com
	//automates: https://issues.redhat.com/browse/OCPBUGS-15035
	g.It("NonHyperShiftHOST-NonPreRelease-Author:jfrancoa-Medium-67564-node's drain should block when PodDisruptionBudget minAvailable equals 100 percentage and selector is empty [Disruptive]", func() {
		exutil.By("Create a deployment with 6 replicas")
		deploy := NewDeployment("hello-openshift", oc.Namespace(), "6", genericDeploymentTemp)
		defer deploy.delete(oc)
		deploy.create(oc)
		deploy.waitForCreation(oc, 5)

		exutil.By("Create PodDisruptionBudget")
		pdb := NewPDB("my-pdb", oc.Namespace(), "100%", podDisruptionBudgetTemp)
		defer pdb.delete(oc)
		pdb.create(oc)

		worker := getSingleWorkerNode(oc)
		exutil.By(fmt.Sprintf("Obtain the pods running on node %v", worker))

		podsInWorker, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-n", oc.Namespace(), "-o=jsonpath={.items[?(@.spec.nodeName=='"+worker+"')].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(strings.Split(podsInWorker, " "))).Should(o.BeNumerically(">", 0))

		// if the pdb's status is false and reason InsufficientPods
		// means that it's not possible to drain a node keeping the
		// required minimum availability, therefore the drain operation
		// should block.
		exutil.By("Make sure that PDB's status is False")
		pdbStatus, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("poddisruptionbudget", "my-pdb", "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[0].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(pdbStatus, "False")).Should(o.BeTrue())

		exutil.By(fmt.Sprintf("Drain the node %v", worker))
		defer waitClusterOperatorAvailable(oc)
		defer oc.WithoutNamespace().AsAdmin().Run("adm").Args("uncordon", worker).Execute()
		// Try to drain the node (it should fail) due to the 100%'s PDB minAvailability
		// as the draining is impossible to happen, if we don't pass a timeout value this
		// command will wait forever, as default timeout is 0s, which means infinite.
		out, err := oc.WithoutNamespace().AsAdmin().Run("adm").Args("drain", worker, "--ignore-daemonsets", "--delete-emptydir-data", "--timeout=30s").Output()
		o.Expect(err).To(o.HaveOccurred(), "Drain operation should have been blocked but it wasn't")
		o.Expect(strings.Contains(out, "Cannot evict pod as it would violate the pod's disruption budget")).Should(o.BeTrue())
		o.Expect(strings.Contains(out, "There are pending nodes to be drained")).Should(o.BeTrue())

		exutil.By("Verify that the pods were not drained from the node")
		podsAfterDrain, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-n", oc.Namespace(), "-o=jsonpath={.items[?(@.spec.nodeName=='"+worker+"')].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podsInWorker).Should(o.BeIdenticalTo(podsAfterDrain))
	})
})

var _ = g.Describe("[sig-node] NODE keda", func() {

	defer g.GinkgoRecover()
	var (
		oc                        = exutil.NewCLI("keda-operator", exutil.KubeConfigPath())
		cmaKedaControllerTemplate string
		buildPruningBaseDir       = exutil.FixturePath("testdata", "node")
	)
	g.BeforeEach(func() {
		// skip ARM64 arch
		architecture.SkipNonAmd64SingleArch(oc)
		buildPruningBaseDir := exutil.FixturePath("testdata", "node")
		cmaKedaControllerTemplate = filepath.Join(buildPruningBaseDir, "cma-keda-controller-template.yaml")
		exutil.SkipMissingQECatalogsource(oc)
		createKedaOperator(oc)
	})
	// author: weinliu@redhat.com
	g.It("StagerunBoth-Author:weinliu-High-52383-Keda Install", func() {
		g.By("CMA (Keda) operator has been installed successfully")
	})

	// author: weinliu@redhat.com
	g.It("StagerunBoth-Author:weinliu-High-62570-Verify must-gather tool works with CMA", func() {
		var (
			mustgatherName = "mustgather" + getRandomString()
			mustgatherDir  = "/tmp/" + mustgatherName
			mustgatherLog  = mustgatherName + ".log"
			logFile        string
		)
		g.By("Get the mustGatherImage")
		mustGatherImage, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("packagemanifest", "-n=openshift-marketplace", "openshift-custom-metrics-autoscaler-operator", "-o=jsonpath={.status.channels[?(.name=='stable')].currentCSVDesc.annotations.containerImage}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Running the must gather command \n")
		defer os.RemoveAll(mustgatherDir)
		logFile, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--dest-dir="+mustgatherDir, "--image="+mustGatherImage).Output()
		if err != nil {
			e2e.Logf("mustgather created from image %v in %v logged to %v,%v %v", mustGatherImage, mustgatherDir, mustgatherLog, logFile, err)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})
	// author: weinliu@redhat.com
	g.It("Author:weinliu-High-60961-Audit logging test - stdout Metadata[Serial]", func() {
		g.By("Create KedaController with log level Metadata")
		g.By("Create CMA Keda Controller ")
		cmaKedaController := cmaKedaControllerDescription{
			level:     "Metadata",
			template:  cmaKedaControllerTemplate,
			name:      "keda",
			namespace: "openshift-keda",
		}
		defer cmaKedaController.delete(oc)
		cmaKedaController.create(oc)
		metricsApiserverPodName := getPodNameByLabel(oc, "openshift-keda", "app=keda-metrics-apiserver")
		waitPodReady(oc, "openshift-keda", "app=keda-metrics-apiserver")
		g.By("Check the Audit Logged as configed")
		log, err := exutil.GetSpecificPodLogs(oc, "openshift-keda", "", metricsApiserverPodName[0], "")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(log, "\"level\":\"Metadata\"")).Should(o.BeTrue())
	})

	// author: weinliu@redhat.com
	g.It("Author:weinliu-Critical-52384-Automatically scaling pods based on Kafka Metrics[Serial][Slow]", func() {
		var (
			scaledObjectStatus string
		)
		kedaControllerDefault := filepath.Join(buildPruningBaseDir, "keda-controller-default52384.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-keda", "KedaController", "keda").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + kedaControllerDefault).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		kafaksNs := "kafka-52384"
		defer deleteProject(oc, kafaksNs)
		createProject(oc, kafaksNs)
		//Create kafak
		exutil.By("Subscribe to AMQ operator")
		defer removeAmqOperator(oc)
		createAmqOperator(oc)
		exutil.By("Test for case OCP-52384")
		exutil.By("Create a Kafka instance")
		kafka := filepath.Join(buildPruningBaseDir, "kafka-52384.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + kafka).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + kafka).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a Kafka topic")
		kafkaTopic := filepath.Join(buildPruningBaseDir, "kafka-topic-52384.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f=" + kafkaTopic).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + kafkaTopic).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Check if Kafka and Kafka topic are ready")
		// Wait for Kafka and KafkaTopic to be ready
		waitForKafkaReady(oc, "my-cluster", kafaksNs)
		namespace := oc.Namespace()
		exutil.By("Create a Kafka Comsumer")
		kafkaComsumerDeployment := filepath.Join(buildPruningBaseDir, "kafka-comsumer-deployment-52384.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f="+kafkaComsumerDeployment, "-n", namespace).Execute()
		err = oc.AsAdmin().Run("create").Args("-f="+kafkaComsumerDeployment, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a scaledobjectc")
		kafkaScaledobject := filepath.Join(buildPruningBaseDir, "kafka-scaledobject-52384.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f="+kafkaScaledobject, "-n", namespace).Execute()
		err = oc.AsAdmin().Run("create").Args("-f="+kafkaScaledobject, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create a Kafka load")
		kafkaLoad := filepath.Join(buildPruningBaseDir, "kafka-load-52384.yaml")
		defer oc.AsAdmin().Run("delete").Args("jobs", "--field-selector", "status.successful=1", "-n", namespace).Execute()
		err = oc.AsAdmin().Run("create").Args("-f="+kafkaLoad, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check ScaledObject is up")
		err = wait.Poll(3*time.Second, 300*time.Second, func() (bool, error) {
			scaledObjectStatus, _ = oc.AsAdmin().Run("get").Args("ScaledObject", "kafka-amqstreams-consumer-scaledobject", "-o=jsonpath={.status.health.s0-kafka-my-topic.status}", "-n", namespace).Output()
			if scaledObjectStatus == "Happy" {
				e2e.Logf("ScaledObject is up and working")
				return true, nil
			}
			e2e.Logf("ScaledObject is not in working status, current status: %v", scaledObjectStatus)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "scaling failed")
		exutil.By("Kafka scaling is up and ready")
	})

	// author: weinliu@redhat.com
	g.It("ConnectedOnly-Author:weinliu-Critical-52385-Automatically scaling pods based on Prometheus metrics[Serial]", func() {
		exutil.By("Create a kedacontroller with default template")
		kedaControllerDefault := filepath.Join(buildPruningBaseDir, "keda-controller-default52384.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-keda", "KedaController", "keda").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f=" + kedaControllerDefault).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		var scaledObjectStatus string
		triggerAuthenticationTempl := filepath.Join(buildPruningBaseDir, "triggerauthentication-52385.yaml")
		triggerAuthentication52385 := triggerAuthenticationDescription{
			secretname: "",
			namespace:  "",
			template:   triggerAuthenticationTempl,
		}
		cmaNs := "cma-52385"
		defer deleteProject(oc, cmaNs)
		createProject(oc, cmaNs)

		exutil.By("1) Create OpenShift monitoring for user-defined projects")
		// Look for cluster-level monitoring configuration
		getOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ConfigMap", "cluster-monitoring-config", "-n", "openshift-monitoring", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Enable user workload monitoring
		if len(getOutput) > 0 {
			exutil.By("ConfigMap cluster-monitoring-config exists, extracting cluster-monitoring-config ...")
			extractOutput, _, _ := oc.AsAdmin().WithoutNamespace().Run("extract").Args("ConfigMap/cluster-monitoring-config", "-n", "openshift-monitoring", "--to=-").Outputs()
			//if strings.Contains(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(extractOutput, "'", ""), "\"", ""), " ", ""), "enableUserWorkload:true") {
			cleanedOutput := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(extractOutput, "'", ""), "\"", ""), " ", "")
			e2e.Logf("cleanedOutput is  %s", cleanedOutput)
			if matched, _ := regexp.MatchString("enableUserWorkload:\\s*true", cleanedOutput); matched {
				exutil.By("User workload is enabled, doing nothing ... ")
			} else {
				exutil.By("User workload is not enabled, enabling ...")
				exutil.By("Get current monitoring configuration to recover")
				originclusterMonitoringConfig, getContentError := oc.AsAdmin().Run("get").Args("ConfigMap/cluster-monitoring-config", "-ojson", "-n", "openshift-monitoring").Output()
				o.Expect(getContentError).NotTo(o.HaveOccurred())
				originclusterMonitoringConfig, getContentError = sjson.Delete(originclusterMonitoringConfig, `metadata.resourceVersion`)
				o.Expect(getContentError).NotTo(o.HaveOccurred())
				originclusterMonitoringConfig, getContentError = sjson.Delete(originclusterMonitoringConfig, `metadata.uid`)
				o.Expect(getContentError).NotTo(o.HaveOccurred())
				originclusterMonitoringConfigFilePath := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-52385.json")
				o.Expect(ioutil.WriteFile(originclusterMonitoringConfigFilePath, []byte(originclusterMonitoringConfig), 0644)).NotTo(o.HaveOccurred())
				defer func() {
					errReplace := oc.AsAdmin().WithoutNamespace().Run("replace").Args("-f", originclusterMonitoringConfigFilePath).Execute()
					o.Expect(errReplace).NotTo(o.HaveOccurred())
				}()
				exutil.By("Deleting current monitoring configuration")
				oc.WithoutNamespace().AsAdmin().Run("delete").Args("ConfigMap/cluster-monitoring-config", "-n", "openshift-monitoring").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
				exutil.By("Create my monitoring configuration")
				prometheusConfigmap := filepath.Join(buildPruningBaseDir, "prometheus-configmap-52385.yaml")
				_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f=" + prometheusConfigmap).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
		} else {
			e2e.Logf("ConfigMap cluster-monitoring-config does not exist, creating ...")
			prometheusConfigmap := filepath.Join(buildPruningBaseDir, "prometheus-configmap-52385.yaml")
			defer func() {
				errDelete := oc.WithoutNamespace().AsAdmin().Run("delete").Args("-f=" + prometheusConfigmap).Execute()
				o.Expect(errDelete).NotTo(o.HaveOccurred())
			}()
			_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f=" + prometheusConfigmap).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		exutil.By("2) Deploy application that exposes Prometheus metrics")
		prometheusComsumer := filepath.Join(buildPruningBaseDir, "prometheus-comsumer-deployment-52385.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f="+prometheusComsumer, "-n", cmaNs).Execute()
		err = oc.AsAdmin().Run("create").Args("-f="+prometheusComsumer, "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3) Create a Service Account")
		defer oc.WithoutNamespace().AsAdmin().Run("delete").Args("sa", "thanos-52385", "-n", cmaNs).Execute()
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", "thanos-52385", "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("3.1) Get the SA token name")
		cmd := `oc describe sa thanos-52385 -n cma-52385 | awk '/Tokens:/ {printf "%s",$2}'`
		saTokenName, err := exec.Command("bash", "-c", cmd).Output()
		e2e.Logf("----- saTokenName is %v -----, error is %v", saTokenName[0], err)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("3.2) Define TriggerAuthentication with the Service Account's token")
		triggerAuthentication52385.secretname = string(saTokenName[:])
		triggerAuthentication52385.namespace = cmaNs
		defer oc.AsAdmin().Run("delete").Args("-n", cmaNs, "TriggerAuthentication", "keda-trigger-auth-prometheus").Execute()
		triggerAuthentication52385.create(oc)

		exutil.By("4) Create a role for reading metric from Thanos")
		role := filepath.Join(buildPruningBaseDir, "role-52385.yaml")
		defer oc.AsAdmin().Run("delete").Args("-f="+role, "-n", cmaNs).Execute()
		err = oc.AsAdmin().Run("create").Args("-f="+role, "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("5) Add the role for reading metrics from Thanos to the Service Account")
		rolebinding := filepath.Join(buildPruningBaseDir, "rolebinding-52385.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f="+rolebinding, "-n", cmaNs).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f="+rolebinding, "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("6) Deploy ScaledObject to enable application autoscaling")
		scaledobject := filepath.Join(buildPruningBaseDir, "scaledobject-52385.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f="+scaledobject, "-n", cmaNs).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f="+scaledobject, "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("7) Generate requests to test the application autoscaling")
		load := filepath.Join(buildPruningBaseDir, "load-52385.yaml")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f="+load, "-n", cmaNs).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f="+load, "-n", cmaNs).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("8) Check ScaledObject is up")
		err = wait.Poll(3*time.Second, 100*time.Second, func() (bool, error) {
			scaledObjectStatus, _ = oc.AsAdmin().Run("get").Args("ScaledObject", "prometheus-scaledobject", "-o=jsonpath={.status.health.s0-prometheus-http_requests_total.status}", "-n", cmaNs).Output()
			if scaledObjectStatus == "Happy" {
				e2e.Logf("ScaledObject is up and working")
				return true, nil
			}
			e2e.Logf("ScaledObject is not in working status, current status: %v", scaledObjectStatus)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "scaling failed")
		exutil.By("prometheus scaling is up and ready")
	})
})

var _ = g.Describe("[sig-node] NODE VPA Vertical Pod Autoscaler", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("vpa-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipMissingQECatalogsource(oc)
		createVpaOperator(oc)
	})
	// author: weinliu@redhat.com
	g.It("DEPRECATED-StagerunBoth-Author:weinliu-High-60991-VPA Install", func() {
		g.By("VPA operator is installed successfully")
	})
})

var _ = g.Describe("[sig-node] NODE Install and verify Cluster Resource Override Admission Webhook", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("clusterresourceoverride-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {

		g.By("Skip test when precondition not meet !!!")
		exutil.SkipMissingQECatalogsource(oc)
		installOperatorClusterresourceoverride(oc)

	})
	// author: asahay@redhat.com

	g.It("StagerunBoth-Author:asahay-High-27070-Cluster Resource Override Operator. [Serial]", func() {
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Execute()
		createCRClusterresourceoverride(oc)
		var err error
		var croCR string
		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			croCR, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Output()
			if err != nil {
				e2e.Logf("error  %v, please try next round", err)
				return false, nil
			}
			if !strings.Contains(croCR, "cluster") {
				return false, nil
			}
			return true, nil

		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not get cluster with output %v, the error is %v", croCR, err))
		e2e.Logf("Operator is installed successfully")
	})

	g.It("Author:asahay-Medium-27075-Testing the config changes. [Serial]", func() {

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ClusterResourceOverride", "cluster").Execute()
		createCRClusterresourceoverride(oc)
		var err error
		var croCR string
		errCheck := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
			croCR, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ClusterResourceOverride", "cluster", "-n", "clusterresourceoverride-operator").Output()
			if err != nil {
				e2e.Logf("error  %v, please try next round", err)
				return false, nil
			}
			if !strings.Contains(croCR, "cluster") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("can not get cluster with output %v, the error is %v", croCR, err))
		e2e.Logf("Operator is installed successfully")

		g.By("Testing the changes\n")
		testCRClusterresourceoverride(oc)

	})

})
