package workloads

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"

	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	expect "github.com/google/goexpect"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfra"
)

var _ = g.Describe("[sig-cli] Workloads test oc works well", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("oc", exutil.KubeConfigPath())
	)

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Longduration-ConnectedOnly-NonPreRelease-Author:yinzhou-High-43032-oc adm release mirror generating correct imageContentSources when using --to and --to-release-image [Slow]", func() {
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, command in pod without proxy will failed, skip it.")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		podMirrorT := filepath.Join(buildPruningBaseDir, "pod_mirror.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:1.2.0",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		serInfo := registry.createregistry(oc)
		defer registry.deleteregistry(oc)

		g.By("Get the cli image from openshift")
		cliImage := getCliImage(oc)

		g.By("Create the  pull secret from the localfile")
		createPullSecret(oc, oc.Namespace())
		defer oc.Run("delete").Args("secret/my-secret", "-n", oc.Namespace()).Execute()

		imageSouceS := "--from=quay.io/openshift-release-dev/ocp-release:4.5.8-x86_64"
		imageToS := "--to=" + serInfo.serviceName + "/zhouytest/test-release"
		imageToReleaseS := "--to-release-image=" + serInfo.serviceName + "/zhouytest/ocptest-release:4.5.8-x86_64"
		imagePullSecretS := "-a " + "/etc/foo/" + ".dockerconfigjson"

		pod43032 := podMirror{
			name:            "mypod43032",
			namespace:       oc.Namespace(),
			cliImageID:      cliImage,
			imagePullSecret: imagePullSecretS,
			imageSource:     imageSouceS,
			imageTo:         imageToS,
			imageToRelease:  imageToReleaseS,
			template:        podMirrorT,
		}

		g.By("Trying to launch the mirror pod")
		pod43032.createPodMirror(oc)
		defer oc.Run("delete").Args("pod/mypod43032", "-n", oc.Namespace()).Execute()
		g.By("check the mirror pod status")
		err := wait.Poll(5*time.Second, 900*time.Second, func() (bool, error) {
			out, err := oc.Run("get").Args("-n", oc.Namespace(), "pod", pod43032.name, "-o=jsonpath={.status.phase}").Output()
			if err != nil {
				e2e.Logf("Fail to get pod: %s, error: %s and try again", pod43032.name, err)
			}
			if matched, _ := regexp.MatchString("Succeeded", out); matched {
				e2e.Logf("Mirror completed: %s", out)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Mirror is not completed")

		g.By("Check the mirror result")
		mirrorOutFile, err := oc.Run("logs").Args("-n", oc.Namespace(), "pod/"+pod43032.name).OutputToFile(getRandomString() + "workload-mirror.txt")
		o.Expect(err).NotTo(o.HaveOccurred())

		reg := regexp.MustCompile(`(?m:^  -.*/zhouytest/test-release$)`)
		reg2 := regexp.MustCompile(`(?m:^  -.*/zhouytest/ocptest-release$)`)
		if reg == nil && reg2 == nil {
			e2e.Failf("regexp err")
		}
		b, err := ioutil.ReadFile(mirrorOutFile)
		if err != nil {
			e2e.Failf("failed to read the file ")
		}
		s := string(b)
		match := reg.FindString(s)
		match2 := reg2.FindString(s)
		if match != "" && match2 != "" {
			e2e.Logf("mirror succeed %v and %v ", match, match2)
		} else {
			e2e.Failf("Failed to mirror")
		}

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-44797-Could define a Command for DC", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.11") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		g.By("create new namespace")
		oc.SetupProject()

		g.By("Create the dc with define command")
		err := oc.WithoutNamespace().Run("create").Args("deploymentconfig", "-n", oc.Namespace(), "dc44797", "--image="+"quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--", "tail", "-f", "/dev/null").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the command should be defined")
		comm, _, err := oc.Run("get").WithoutNamespace().Args("dc/dc44797", "-n", oc.Namespace(), "-o=jsonpath={.spec.template.spec.containers[0].command[0]}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect("tail").To(o.Equal(comm))

		g.By("Create the deploy with define command")
		err = oc.WithoutNamespace().Run("create").Args("deployment", "-n", oc.Namespace(), "deploy44797", "--image="+"quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "--", "tail", "-f", "/dev/null").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the command should be defined")
		comm1, err := oc.Run("get").WithoutNamespace().Args("deploy/deploy44797", "-n", oc.Namespace(), "-o=jsonpath={.spec.template.spec.containers[0].command[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect("tail").To(o.Equal(comm1))

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-High-43034-should not show signature verify error msgs while trying to mirror OCP image repository to [Flaky]", func() {
		if checkProxy(oc) {
			g.Skip("This is proxy cluster, command in pod without proxy will failed, skip the test.")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		podMirrorT := filepath.Join(buildPruningBaseDir, "pod_mirror.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:1.2.0",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Get the cli image from openshift")
		cliImage := getCliImage(oc)

		g.By("Create the  pull secret from the localfile")
		defer oc.Run("delete").Args("secret/my-secret", "-n", oc.Namespace()).Execute()
		createPullSecret(oc, oc.Namespace())

		g.By("Add the cluster admin role for the default sa")
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "cluster-admin", "-z", "default", "-n", oc.Namespace()).Execute()
		err1 := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "cluster-admin", "-z", "default", "-n", oc.Namespace()).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())

		imageSouceS := "--from=quay.io/openshift-release-dev/ocp-release:4.5.5-x86_64"
		imageToS := "--to=" + serInfo.serviceName + "/zhouytest/test-release"
		imageToReleaseS := "--apply-release-image-signature"
		imagePullSecretS := "-a " + "/etc/foo/" + ".dockerconfigjson"

		pod43034 := podMirror{
			name:            "mypod43034",
			namespace:       oc.Namespace(),
			cliImageID:      cliImage,
			imagePullSecret: imagePullSecretS,
			imageSource:     imageSouceS,
			imageTo:         imageToS,
			imageToRelease:  imageToReleaseS,
			template:        podMirrorT,
		}

		g.By("Trying to launch the mirror pod")
		defer oc.Run("delete").Args("pod/mypod43034", "-n", oc.Namespace()).Execute()
		pod43034.createPodMirror(oc)
		g.By("check the mirror pod status")
		err := wait.Poll(5*time.Second, 900*time.Second, func() (bool, error) {
			out, err := oc.Run("get").Args("-n", oc.Namespace(), "pod", pod43034.name, "-o=jsonpath={.status.phase}").Output()
			if err != nil {
				e2e.Logf("Fail to get pod: %s, error: %s and try again", pod43034.name, err)
			}
			if matched, _ := regexp.MatchString("Succeeded", out); matched {
				e2e.Logf("Mirror completed: %s", out)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Mirror is not completed")

		g.By("Get the created configmap")
		newConfigmapS, err := oc.Run("logs").Args("-n", oc.Namespace(), "pod/"+pod43034.name, "--tail=1").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newConfigmapN := strings.Split(newConfigmapS, " ")[0]
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-config-managed", newConfigmapN).Execute()

		g.By("Check the mirror result")
		mirrorOutFile, err := oc.Run("logs").Args("-n", oc.Namespace(), "pod/"+pod43034.name).OutputToFile(getRandomString() + "workload-mirror.txt")
		o.Expect(err).NotTo(o.HaveOccurred())

		reg := regexp.MustCompile(`(unable to retrieve signature)`)
		if reg == nil {
			e2e.Failf("regexp err")
		}
		b, err := ioutil.ReadFile(mirrorOutFile)
		if err != nil {
			e2e.Failf("failed to read the file ")
		}
		s := string(b)
		match := reg.FindString(s)
		if match != "" {
			e2e.Failf("Mirror failed %v", match)
		} else {
			e2e.Logf("Succeed with the apply-release-image-signature option")
		}

	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-Medium-33648-must gather pod should not schedule on windows node", func() {
		go checkMustgatherPodNode(oc)
		g.By("Create the must-gather pod")
		oc.AsAdmin().WithoutNamespace().Run("adm").Args("must-gather", "--timeout="+"30s", "--dest-dir=/tmp/mustgatherlog", "--", "/etc/resolv.conf").Execute()
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-Medium-48681-Could start debug pod using pod definition yaml", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		debugPodUsingDefinitionT := filepath.Join(buildPruningBaseDir, "debugpod_48681.yaml")

		g.By("create new namespace")
		oc.SetupProject()
		g.By("Get the cli image from openshift")
		cliImage := getCliImage(oc)

		pod48681 := debugPodUsingDefinition{
			name:       "pod48681",
			namespace:  oc.Namespace(),
			cliImageID: cliImage,
			template:   debugPodUsingDefinitionT,
		}

		g.By("Create test pod")
		pod48681.createDebugPodUsingDefinition(oc)
		defer oc.Run("delete").Args("pod/pod48681", "-n", oc.Namespace()).Execute()
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-NonPreRelease-Longduration-High-45307-Critical-45327-check oc adm prune deployments to prune RS [Slow][Disruptive]", func() {
		g.By("create new namespace")
		oc.SetupProject()

		g.By("Create deployments and trigger more times")
		createDeployment(oc, oc.Namespace(), "mydep45307")
		triggerSucceedDeployment(oc, oc.Namespace(), "mydep45307", 6, 20)
		triggerFailedDeployment(oc, oc.Namespace(), "mydep45307")

		g.By("get the completed rs infomation")
		totalCompletedRsList, totalCompletedRsListNum := getCompeletedRsInfo(oc, oc.Namespace(), "mydep45307")

		g.By("Dry run the prune deployments for RS")
		keepCompletedRsNum := 3
		pruneRsNumCMD := fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true  |grep %s |wc -l", keepCompletedRsNum, oc.Namespace())
		pruneRsDryCMD := fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true  |grep %s|awk '{print $2}'", keepCompletedRsNum, oc.Namespace())
		rsListFromPrune := getShouldPruneRSFromPrune(oc, pruneRsNumCMD, pruneRsDryCMD, (totalCompletedRsListNum - keepCompletedRsNum))
		shouldPruneRsList := getShouldPruneRSFromCreateTime(totalCompletedRsList, totalCompletedRsListNum, keepCompletedRsNum)
		if comparePrunedRS(shouldPruneRsList, rsListFromPrune) {
			e2e.Logf("Checked the pruned rs is expected")
		} else {
			e2e.Failf("Pruned the wrong RS with dry run")
		}

		g.By("Make sure never prune RS with replicas num >0")
		//before prune ,check the running rs list
		runningRsList := checkRunningRsList(oc, oc.Namespace(), "mydep45307")

		//checking the should prune rs list
		completedRsNum := 0
		pruneRsNumCMD = fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true  |grep %s |wc -l", completedRsNum, oc.Namespace())
		pruneRsDryCMD = fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true  |grep %s|awk '{print $2}'", completedRsNum, oc.Namespace())

		rsListFromPrune = getShouldPruneRSFromPrune(oc, pruneRsNumCMD, pruneRsDryCMD, (totalCompletedRsListNum - completedRsNum))
		shouldPruneRsList = getShouldPruneRSFromCreateTime(totalCompletedRsList, totalCompletedRsListNum, completedRsNum)
		if comparePrunedRS(shouldPruneRsList, rsListFromPrune) {
			e2e.Logf("dry run prune all completed rs is expected")
		} else {
			e2e.Failf("Pruned the wrong RS with dry run")
		}

		//prune all the completed rs list
		pruneCompletedRs(oc, "prune", "deployments", "--keep-complete=0", "--keep-younger-than=10s", "--replica-sets=true", "--confirm")

		//after prune , check the remaining rs list
		remainingRsList := getRemainingRs(oc, oc.Namespace(), "mydep45307")
		if comparePrunedRS(runningRsList, remainingRsList) {
			e2e.Logf("pruned all completed rs is expected")
		} else {
			e2e.Failf("Pruned the wrong")
		}
	})
	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-NonPreRelease-Longduration-High-45308-check oc adm prune deployments command with the orphans options works well [Slow][Disruptive]", func() {
		g.By("create new namespace")
		oc.SetupProject()

		g.By("Create deployments and trigger more times")
		createDeployment(oc, oc.Namespace(), "mydep45308")
		triggerSucceedDeployment(oc, oc.Namespace(), "mydep45308", 6, 20)
		triggerFailedDeployment(oc, oc.Namespace(), "mydep45308")

		g.By("get the completed rs infomation")
		totalCompletedRsList, totalCompletedRsListNum := getCompeletedRsInfo(oc, oc.Namespace(), "mydep45308")

		g.By("delete the deploy with ")
		err := oc.Run("delete").Args("-n", oc.Namespace(), "deploy", "mydep45308", "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("prune the rs with orphans=true")
		//before prune ,check the running rs list
		runningRsList := checkRunningRsList(oc, oc.Namespace(), "mydep45308")

		//checking the should prune rs list
		completedRsNum := 0
		pruneRsNumCMD := fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true --orphans=true |grep %s |wc -l", completedRsNum, oc.Namespace())
		pruneRsDryCMD := fmt.Sprintf("oc adm prune deployments --keep-complete=%v --keep-younger-than=10s --replica-sets=true --orphans=true |grep %s|awk '{print $2}'", completedRsNum, oc.Namespace())

		rsListFromPrune := getShouldPruneRSFromPrune(oc, pruneRsNumCMD, pruneRsDryCMD, (totalCompletedRsListNum - completedRsNum))
		shouldPruneRsList := getShouldPruneRSFromCreateTime(totalCompletedRsList, totalCompletedRsListNum, completedRsNum)
		if comparePrunedRS(shouldPruneRsList, rsListFromPrune) {
			e2e.Logf("dry run prune all completed rs is expected")
		} else {
			e2e.Failf("Pruned the wrong RS with dry run")
		}

		//prune all the completed rs list
		pruneCompletedRs(oc, "prune", "deployments", "--keep-complete=0", "--keep-younger-than=10s", "--replica-sets=true", "--confirm", "--orphans=true")

		//after prune , check the remaining rs list
		remainingRsList := getRemainingRs(oc, oc.Namespace(), "mydep45308")
		if comparePrunedRS(runningRsList, remainingRsList) {
			e2e.Logf("pruned all completed rs is expected")
		} else {
			e2e.Failf("Pruned the wrong")
		}
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-49859-should failed when oc import-image setting with Garbage values for --reference-policy", func() {
		g.By("create new namespace")
		oc.SetupProject()

		g.By("import image with garbage values set for reference-policy")
		out, err := oc.Run("import-image").Args("registry.redhat.io/openshift3/jenkins-2-rhel7", "--reference-policy=sdfsdfds", "--confirm").Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("reference policy values are source or local"))

		g.By("check should no imagestream created")
		out, err = oc.Run("get").Args("is").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("No resources found"))
	})

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-44061-Check the default registry credential path for oc", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		g.By("check the help info for the registry config locations")
		clusterImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		dockerCred := checkDockerCred()
		if dockerCred {
			e2e.Logf("there are default docker cred in the prow")
			err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage).Execute()
			if err != nil {
				err1 := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage, "--filter-by-os", "linux/amd64").Execute()
				o.Expect(err1).NotTo(o.HaveOccurred())
			}
		}

		podmanCred := checkPodmanCred()
		if podmanCred {
			e2e.Logf("there are default podman cred in the prow")
			err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage).Execute()
			if err != nil {
				err1 := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage, "--filter-by-os", "linux/amd64").Execute()
				o.Expect(err1).NotTo(o.HaveOccurred())
			}
		}

		g.By("Set podman registry config")
		dirname := "/tmp/case44061"
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage).Execute()
		if err != nil {
			err1 := oc.AsAdmin().WithoutNamespace().Run("image").Args("info", clusterImage, "--filter-by-os", "linux/amd64").Execute()
			o.Expect(err1).NotTo(o.HaveOccurred())
		}
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-50399-oc apply could update EgressNetworkPolicy resource", func() {
		networkType := checkNetworkType(oc)
		e2e.Logf("Network type is :%s", networkType)

		if strings.Contains(networkType, "ovn") || strings.Contains(networkType, "other") {
			g.Skip("Skip for ovn cluster Or the third party network setting !!!")
		}
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		egressnetworkP := filepath.Join(buildPruningBaseDir, "egressnetworkpolicy.yaml")
		updateegressnetworkP := filepath.Join(buildPruningBaseDir, "update_egressnetworkpolicy.yaml")

		g.By("create new namespace")
		oc.SetupProject()
		out, err := oc.AsAdmin().Run("apply").Args("-f", egressnetworkP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("default-egress-egressnetworkpolicy created"))
		out, err = oc.AsAdmin().Run("apply").Args("-f", updateegressnetworkP).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("default-egress-egressnetworkpolicy configured"))
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-NonPreRelease-Longduration-Author:yinzhou-High-42982-Describe quota output should always show units", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") || isBaselineCapsSet(oc, "v4.11") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		// Skip Hypershift external OIDC clusters against which all test cases run as the same (external) user
		isExternalOIDCCluster, err := exutil.IsExternalOIDCCluster(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		if isExternalOIDCCluster {
			g.Skip("Skipping the test as we are running against a Hypershift external OIDC cluster")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deploymentconfigF := filepath.Join(buildPruningBaseDir, "deploymentconfig_with_quota.yaml")
		clusterresourceF := filepath.Join(buildPruningBaseDir, "clusterresource_for_user.yaml")
		g.By("create new namespace")
		oc.SetupProject()
		err = oc.AsAdmin().Run("create").Args("quota", "compute-resources-42982", "--hard=requests.cpu=4,requests.memory=8Gi,pods=4,limits.cpu=4,limits.memory=8Gi").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", deploymentconfigF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		//wait for pod running
		checkPodStatus(oc, "deploymentconfig=hello-openshift", oc.Namespace(), "Running")
		checkPodStatus(oc, "openshift.io/deployer-pod-for.name=hello-openshift-1", oc.Namespace(), "Succeeded")
		output, err := oc.Run("describe").Args("quota", "compute-resources-42982").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("requests.memory.*Ki.*8Gi", output); matched {
			e2e.Logf("describe the quota with units:\n%s", output)
		}

		//check for clusterresourcequota
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterresourcequota", "for-user42982").Execute()
		userName, err := oc.Run("whoami").Args("").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", clusterresourceF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("current user name is %v", userName)
		patchPath := fmt.Sprintf("-p=[{\"op\": \"replace\", \"path\": \"/spec/selector/annotations\", \"value\":{ \"openshift.io/requester\": \"%s\" }}]", userName)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("clusterresourcequota", "for-user42982", "--type=json", patchPath).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("new-project").Args("p42982-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", "p42982-1").Execute()
		err = oc.WithoutNamespace().Run("create").Args("-f", deploymentconfigF, "-n", "p42982-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		//wait for pod running
		checkPodStatus(oc, "deploymentconfig=hello-openshift", "p42982-1", "Running")
		checkPodStatus(oc, "openshift.io/deployer-pod-for.name=hello-openshift-1", "p42982-1", "Succeeded")
		output, err = oc.AsAdmin().WithoutNamespace().Run("describe").Args("clusterresourcequota", "for-user42982").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("requests.memory.*Ki.*8Gi", output); matched {
			e2e.Logf("describe the quota with units:\n%s", output)
		}

	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Critical-51009-High-51017-oc adm release new support manifest list", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		exutil.SkipIfPlatformTypeNot(oc, "AWS")
		workloadsBaseDir := exutil.FixturePath("testdata", "workloads")
		manifestlistImagestream := filepath.Join(workloadsBaseDir, "12708358_4.11.0-0.nightly-multi-2022-04-18-120932-release-imagestream.yaml")
		ns := oc.Namespace()

		g.By("Trying to launch a registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:1.2.0",
			namespace:   ns,
		}
		defer registry.deleteregistry(oc)
		_ = registry.createregistry(oc)

		createEdgeRoute(oc, "registry", ns, "registry")

		registryHost := strings.ReplaceAll(getHostFromRoute(oc, "registry", ns), "'", "")
		e2e.Logf("registry route is %v", registryHost)

		secretFile, err := getPullSecret(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "new", "--registry-config="+secretFile, "--reference-mode=source", "--keep-manifest-list", "-f", manifestlistImagestream, "--name", "4.11.0-0.nightly", "--to-image="+registryHost+"/ocp-release:4.11.0-0.nightly", "--insecure").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The release new command failed with error %s", err))
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", registryHost+"/ocp-release:4.11.0-0.nightly", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", registryHost+"/ocp-release:4.11.0-0.nightly", "--filter-by-os", "linux/s390x", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("linux/s390x"))
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Medium-44928-oc image mirror support registry which authorization server's url is different from registry url", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		g.By("Skip if the cluster is AzureStackCloud")
		azureStackCloud, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureStackCloud == "AzureStackCloud" {
			g.Skip("Skip for cluster with AzureStackCloud!")
		}
		architecture.SkipArchitectures(oc, architecture.MULTI)
		dockerauthBaseDir := exutil.FixturePath("testdata", "workloads")
		dockerConfigDir := filepath.Join(dockerauthBaseDir, "config")
		dockerauthfile := filepath.Join(dockerauthBaseDir, "auth.json")
		ns := "p44928"
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", ns, "--ignore-not-found").Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		ssldir := "/tmp/case44982/ssl"
		defer os.RemoveAll(ssldir)
		createDir(ssldir)
		registryHost := createSpecialRegistry(oc, ns, ssldir, dockerConfigDir)
		exec.Command("bash", "-c", fmt.Sprintf("sed -i 's/testroute/%s/g' %s", registryHost, dockerauthfile)).Output()

		err = wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
			_, err1 := oc.WithoutNamespace().Run("image").Args("mirror", "--insecure", "-a", dockerauthfile, "quay.io/openshifttest/busybox:latest", registryHost+"/test/busybox:latest").Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "oc image mirror fails")

	})

	// author: knarra@redhat.com
	g.It("Longduration-ConnectedOnly-Author:knarra-VMonly-Critical-54787-Run sos report against  ocp cluster and verify that it works fine[Serial]", func() {
		sosTmpDirName := "/tmp/case54787"
		err := os.MkdirAll(sosTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(sosTmpDirName)
		command := fmt.Sprintf("sos collect --no-local --batch --preset=ocp --tmp-dir=%s", sosTmpDirName)
		collectionStatus, err := exec.Command("bash", "-c", command).Output()
		e2e.Logf("Sos output is: %v", string(collectionStatus))
		if err != nil {
			e2e.Failf("Error occured during sos report collect: %v", err.Error())
		}
		validationMessages := []string{
			"Successfully collected sos report",
			"The following archive has been created",
			"/tmp/case54787/sos-collector-",
		}
		for _, vmessage := range validationMessages {
			o.Expect(collectionStatus).To(o.ContainSubstring(vmessage))
		}

	})

	// author: knarra@redhat.com
	g.It("Longduration-ConnectedOnly-Author:knarra-VMonly-Critical-55971-Run sos collect against  ocp cluster to collect logs from master and workers and verify that it works fine[Serial]", func() {
		sosTmpDirName := "/tmp/case55971"
		err := os.MkdirAll(sosTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(sosTmpDirName)
		command := fmt.Sprintf("sos collect --no-local -c ocp.role=worker:master --batch --preset=ocp --tmp-dir=%s", sosTmpDirName)
		collectionStatus, err := exec.Command("bash", "-c", command).Output()
		e2e.Logf("Sos output is:", string(collectionStatus))
		if err != nil {
			e2e.Failf("Error occured during sos report collect: %v", err.Error())
		}
		validationMessages := []string{
			"Successfully collected sos report",
			"The following archive has been created",
			"/tmp/case55971/sos-collector-",
		}
		for _, vmessage := range validationMessages {
			o.Expect(collectionStatus).To(o.ContainSubstring(vmessage))
		}

	})

	// author: knarra@redhat.com
	g.It("ConnectedOnly-Author:knarra-VMonly-High-55972-Run sos collect against  ocp cluster to collect kernel logs from master nodes and verify that it works fine[Serial]", func() {
		sosTmpDirName := "/tmp/case55972"
		err := os.MkdirAll(sosTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(sosTmpDirName)
		command := fmt.Sprintf("sos collect --no-local -o kernel --batch --tmp-dir=%s", sosTmpDirName)
		collectionStatus, err := exec.Command("bash", "-c", command).Output()
		e2e.Logf("Sos output is:", string(collectionStatus))
		if err != nil {
			e2e.Failf("Error occured during sos report collect: %v", err.Error())
		}
		validationMessages := []string{
			"Successfully collected sos report",
			"The following archive has been created",
			"/tmp/case55972/sos-collector-",
		}
		for _, vmessage := range validationMessages {
			o.Expect(collectionStatus).To(o.ContainSubstring(vmessage))
		}

	})

	// author: knarra@redhat.com
	g.It("Longduration-ConnectedOnly-Author:knarra-VMonly-High-55973-Verify sos collect against worker nodes in ocp cluster and validate that it works fine[Serial]", func() {
		sosTmpDirName := "/tmp/case55973"
		err := os.MkdirAll(sosTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(sosTmpDirName)
		command := fmt.Sprintf("sos collect --no-local -c ocp.role=worker --batch --preset=ocp --tmp-dir=%s", sosTmpDirName)
		collectionStatus, err := exec.Command("bash", "-c", command).Output()
		e2e.Logf("Sos output is:", string(collectionStatus))
		if err != nil {
			e2e.Failf("Error occured during sos report collect: %v", err.Error())
		}
		validationMessages := []string{
			"Successfully collected sos report",
			"The following archive has been created",
			"/tmp/case55973/sos-collector-",
		}
		for _, vmessage := range validationMessages {
			o.Expect(collectionStatus).To(o.ContainSubstring(vmessage))
		}

	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-High-38178-oc should be able to debug init container", func() {
		podBaseDir := exutil.FixturePath("testdata", "workloads")
		initPodFile := filepath.Join(podBaseDir, "initContainer.yaml")

		g.By("Create pod with init container")
		err := oc.Run("create").Args("-f", initPodFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Make sure pod with init container running well")
		checkPodStatus(oc, "name=hello-pod", oc.Namespace(), "Running")
		g.By("Run debug command with init container")
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			output, err := oc.Run("debug").Args("pod/hello-pod", "-c", "wait").Output()
			if err != nil {
				e2e.Logf("debug failed with error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("sleep", output); matched {
				e2e.Logf("Check the debug pod with init container command succeeded\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get debug with init container"))
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Medium-51018-oc adm release extract support manifest list", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		extractTmpDirName := "/tmp/case51018"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pullSpec := getLatestPayload("https://multi.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable-multi/latest")
		e2e.Logf("The pullSpec is %s \n", pullSpec)
		if len(pullSpec) == 0 || strings.TrimSpace(pullSpec) == "" {
			g.Skip("pullSpec is empty, so skipping the test")
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "-a", extractTmpDirName+"/.dockerconfigjson", "--command=oc.rhel8", "--to="+extractTmpDirName, pullSpec).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Check oc executable to make sure match the platform")
		_, err = exec.Command("bash", "-c", "/tmp/case51018/oc version").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "-a", extractTmpDirName+"/.dockerconfigjson", "--command=oc", "--to="+extractTmpDirName+"/mac", pullSpec, "--command-os=mac/amd64").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		macocheckcmd := "file /tmp/case51018/mac/oc"
		output, err := exec.Command("bash", "-c", macocheckcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Mach-O"))
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "-a", extractTmpDirName+"/.dockerconfigjson", "--command=oc", "--to="+extractTmpDirName+"/macarm", pullSpec, "--command-os=mac/arm64").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		macocheckcmd = "file /tmp/case51018/macarm/oc"
		output, err = exec.Command("bash", "-c", macocheckcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Mach-O 64-bit arm64 executable"))
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "-a", extractTmpDirName+"/.dockerconfigjson", "--command=oc", "--to="+extractTmpDirName+"/windows", pullSpec, "--command-os=windows").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		windowocheckcmd := "file /tmp/case51018/windows/oc"
		output, err = exec.Command("bash", "-c", windowocheckcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Windows"))
	})

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Medium-61607-oc image mirror always copy blobs if the target is file", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		g.By("Skip if the cluster is AzureStackCloud")
		azureStackCloud, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureStackCloud == "AzureStackCloud" {
			g.Skip("Skip for cluster with AzureStackCloud!")
		}
		testBaseDir := exutil.FixturePath("testdata", "workloads")
		mappingFile := filepath.Join(testBaseDir, "testmapping.txt")
		mirrorFile := filepath.Join(testBaseDir, "mirror-from-filesystem.txt")

		extractTmpDirName := "/tmp/case61607"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("First mirror")
		defer os.RemoveAll("output61607")
		err = wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("mirror", "-f", mappingFile, "--dir", "output61607", "-a", extractTmpDirName+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Remove one blob")
		blobName, err := exec.Command("bash", "-c", `ls output61607/v2/openshifttest/hello-openshift/blobs/ |head  -n 1`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(blobName).NotTo(o.BeEmpty())
		_, err = exec.Command("bash", "-c", "rm -rf "+"output61607/v2/openshifttest/hello-openshift/blobs/"+string(blobName)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := exec.Command("bash", "-c", "ls output61607/v2/openshifttest/hello-openshift/blobs/").Output()
		o.Expect(output).NotTo(o.ContainSubstring(string(blobName)))
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Second mirror")
		err = wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("mirror", "-f", mappingFile, "--dir", "output61607", "-a", extractTmpDirName+"/.dockerconfigjson").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Mirror from file to registry")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, mirrorFile)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("mirror", "-f", mirrorFile, "--from-dir=output61607", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:yinzhou-High-51011-oc adm release mirror support manifest list[Serial]", func() {
		extractTmpDirName := "/tmp/case51011"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pullSpec := getLatestPayload("https://multi.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable-multi/latest")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:1.2.0",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		serInfo := registry.createregistry(oc)
		defer registry.deleteregistry(oc)

		g.By("Make sure mirror succeed")
		err = wait.PollImmediate(1200*time.Second, 3600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "mirror", "-a", extractTmpDirName+"/.dockerconfigjson", "--keep-manifest-list", "--from="+pullSpec, "--to="+serInfo.serviceName+"/openshift-release-dev/ocp-v4.0-art-dev", "--insecure").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "mirror failed")

		_, standerr, err := oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("info", "-a", extractTmpDirName+"/.dockerconfigjson", serInfo.serviceName+"/openshift-release-dev/ocp-v4.0-art-dev", "--insecure").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(standerr).To(o.ContainSubstring("use --filter-by-os to select"))
	})
	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-ConnectedOnly-Author:yinzhou-Medium-60499-oc with icsp mapping scope should match openshift icsp mapping scope", func() {
		extractTmpDirName := "/tmp/case60499"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		pullSpec := getLatestPayload("https://amd64.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable/latest")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:1.2.0",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		serInfo := registry.createregistry(oc)
		defer registry.deleteregistry(oc)
		g.By("Make sure mirror succeed")
		err = wait.PollImmediate(1200*time.Second, 3600*time.Second, func() (bool, error) {
			_, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "mirror", "-a", extractTmpDirName+"/.dockerconfigjson", "--from="+pullSpec, "--to="+serInfo.serviceName+"/openshift-release-dev/ocp-v4.0-art-dev", "--insecure").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "mirror failed")

		imageD, err := exec.Command("bash", "-c", "oc image info "+pullSpec+" | grep Digest |awk '{print $2}'").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageDigest := strings.Replace(string(imageD), "\n", "", -1)

		createEmptyAuth(extractTmpDirName + "/emptyauth.json")
		_, outErr, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "--command=oc", "--to="+extractTmpDirName, "--insecure", "--from="+serInfo.serviceName+"/openshift-release-dev/ocp-v4.0-art-dev@"+imageDigest, "-a", extractTmpDirName+"/emptyauth.json").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(outErr).To(o.ContainSubstring("access to the requested resource is not authorized"))

		ocBaseDir := exutil.FixturePath("testdata", "workloads")
		icspConfig := filepath.Join(ocBaseDir, "icsp60499.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/localhost:5000/%s/g' %s`, serInfo.serviceName, icspConfig)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "extract", "--command=oc", "--icsp-file="+icspConfig, "--to="+extractTmpDirName, "--insecure", "--from="+serInfo.serviceName+"/openshift-release-dev/ocp-v4.0-art-dev@"+imageDigest, "-a", extractTmpDirName+"/emptyauth.json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := exec.Command("bash", "-c", "stat "+extractTmpDirName+"/oc").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("No events in project: %v", string(output))
		o.Expect(strings.Contains(string(output), "File: /tmp/case60499/oc")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:knarra-Medium-66989-Workloads oc debug with or without init container for pod", func() {
		testBaseDir := exutil.FixturePath("testdata", "workloads")
		initContainerFile := filepath.Join(testBaseDir, "initContainer66989.yaml")

		g.By("Create pod with InitContainer")
		err := oc.Run("create").Args("-f", initContainerFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Make sure pod with init container running well")
		checkPodStatus(oc, "name=hello-pod", oc.Namespace(), "Running")
		g.By("Run debug command with init container")
		cmd, _, _, err := oc.Run("debug").Args("pod/hello-pod", "--keep-init-containers=true").Background()
		defer cmd.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(5*time.Second, 100*time.Second, func() (bool, error) {
			debugPodName, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace()).Output()
			if err != nil {
				e2e.Logf("debug failed with error: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("hello-pod-debug", debugPodName); matched {
				e2e.Logf("Check the debug pod command succeeded\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get debug with init container"))

		g.By("Check if Init Containers present in debug pod output")
		debugPodName, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", oc.Namespace(), "-o=jsonpath={.items[1].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		Output, err := oc.WithoutNamespace().Run("describe").Args("pods", debugPodName, "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if matched, _ := regexp.MatchString("Init Containers", Output); !matched {
			e2e.Failf("Init Containers are not seen in the output when run with keep init containers true")
		}
		_, err = oc.WithoutNamespace().Run("delete").Args("pods", debugPodName, "-n", oc.Namespace(), "--wait=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-LEVEL0-Critical-63002-oc new-app propagate containerPort information to the deployment if import-mode is PreserveOriginal", func() {
		g.By("create new namespace")
		oc.SetupProject()
		g.By("create new-app with import-mode as PreserveOrigin")
		err := oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", oc.Namespace(), "--name=example-preserveoriginal", "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.WithoutNamespace().Run("get").Args("svc", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "example-preserveoriginal")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-64920-High-63851-Verify oc adm release info and oc image extract --icsp-file flag still works with deprecated warning message", func() {
		// Skip the case if cluster is C2S/SC2S disconnected as external network cannot be accessed
		if strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: AWS C2S/SC2S disconnected clusters are not satisfied for this test case")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		icspFile64920 := filepath.Join(buildPruningBaseDir, "icspFile64920.yaml")
		var (
			image         string
			authContentAR string
		)

		g.By("Get desired image from ocp cluster")
		pullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pullSpec).NotTo(o.BeEmpty())
		e2e.Logf("pullspec is %v", pullSpec)

		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		// Replace localhost with retreived registry name from the cluster in icsp file
		sedCmd := fmt.Sprintf(`sed -i 's/localhost/%s/g' %s`, imageRegistryName, icspFile64920)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Replace target correctly in the icsp file
		sedCmdOne := fmt.Sprintf(`sed -i 's/target/%s/g' %s`, strings.Split(pullSpec, "/")[1], icspFile64920)
		_, err = exec.Command("bash", "-c", sedCmdOne).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Extract secret and store it
		extractTmpDirName := "/tmp/case64920"
		err = os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Retreive image digest
		imageDigest := strings.Split(pullSpec, "@")[1]
		e2e.Logf("imageDigest is %s", imageDigest)

		// Remove auth & run command oc adm release info with out --icsp-flag
		dockerTmpDirName := "/tmp/case64920/.dockerconfigjson"
		authContent, readErr := os.ReadFile(dockerTmpDirName)
		o.Expect(readErr).NotTo(o.HaveOccurred())
		if strings.Contains(pullSpec, "quay.io") {
			image = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@" + imageDigest
			authContentAR, err = sjson.Delete(string(authContent), `auths.quay\.io`)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else if strings.Contains(pullSpec, "registry.ci.openshift.org") {
			image = "registry.ci.openshift.org/ocp/release@" + imageDigest
			authContentAR, err = sjson.Delete(string(authContent), `auths.registry\.ci\.openshift\.org`)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			sourceImage := strings.Split(pullSpec, "/")[0]
			sourceImageNew := strings.Replace(sourceImage, ".", "\\.", -1)
			image = pullSpec
			authContentAR, err = sjson.Delete(string(authContent), `auths.`+sourceImageNew)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(os.WriteFile(dockerTmpDirName, []byte(authContentAR), 0640)).NotTo(o.HaveOccurred())

		//_, outErr, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image).Outputs()
		//o.Expect(err).Should(o.HaveOccurred())
		//o.Expect(outErr).To(o.ContainSubstring("error: unable to read image " + image))

		// Run command oc adm release info with --icsp-flag
		_, out, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image, "-a", dockerTmpDirName, "--icsp-file="+icspFile64920).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("Flag --icsp-file has been deprecated, support for it will be removed in a future release. Use --idms-file instead"))

		// Run command oc adm release info to get oc-mirror image
		ocMirrorImage, _, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image, "-a", dockerTmpDirName, "--icsp-file="+icspFile64920, `-ojsonpath={.references.spec.tags[?(@.name=="oc-mirror")].from.name}`).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("extractCmd output is %s", ocMirrorImage)

		// Run command oc image extract with --icsp-flag
		_, out, err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("extract", "-a", dockerTmpDirName, ocMirrorImage, "--path=/usr/bin/oc-mirror:"+extractTmpDirName, "--icsp-file="+icspFile64920, "--insecure", "--confirm").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring("Flag --icsp-file has been deprecated, support for it will be removed in a future release. Use --idms-file instead"))

		// Verify oc-mirror is present
		output, err := exec.Command("bash", "-c", "stat "+extractTmpDirName+"/oc-mirror").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), "File: /tmp/case64920/oc-mirror")).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-LEVEL0-Critical-64921-Critical-63854-Verify oc adm release info and oc image extract using --idms-file flag", func() {
		// Skip the case if cluster is C2S/SC2S disconnected as external network cannot be accessed
		if strings.HasPrefix(getClusterRegion(oc), "us-iso") {
			g.Skip("Skipped: AWS C2S/SC2S disconnected clusters are not satisfied for this test case")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		idmsFile64921 := filepath.Join(buildPruningBaseDir, "idmsFile64921.yaml")
		var (
			image         string
			authContentAR string
		)

		g.By("Get desired image from ocp cluster")
		pullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pullSpec).NotTo(o.BeEmpty())
		e2e.Logf("pullspec is %v", pullSpec)

		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		// Replace localhost with retreived registry name from the cluster in idms file
		sedCmd := fmt.Sprintf(`sed -i 's/localhost/%s/g' %s`, imageRegistryName, idmsFile64921)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Replace target correctly in the icsp file
		sedCmdOne := fmt.Sprintf(`sed -i 's/target/%s/g' %s`, strings.Split(pullSpec, "/")[1], idmsFile64921)
		_, err = exec.Command("bash", "-c", sedCmdOne).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Extract secret and store it
		extractTmpDirName := "/tmp/case64921"
		err = os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Retreive image digest
		imageDigest := strings.Split(pullSpec, "@")[1]
		e2e.Logf("imageDigest is %s", imageDigest)

		// Remove auth & run command oc adm release info with out --idms-flag
		dockerTmpDirName := "/tmp/case64921/.dockerconfigjson"
		authContent, readErr := os.ReadFile(dockerTmpDirName)
		o.Expect(readErr).NotTo(o.HaveOccurred())
		if strings.Contains(pullSpec, "quay.io") {
			image = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@" + imageDigest
			authContentAR, err = sjson.Delete(string(authContent), `auths.quay\.io`)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else if strings.Contains(pullSpec, "registry.ci.openshift.org") {
			image = "registry.ci.openshift.org/ocp/release@" + imageDigest
			authContentAR, err = sjson.Delete(string(authContent), `auths.registry\.ci\.openshift\.org`)
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			sourceImage := strings.Split(pullSpec, "/")[0]
			sourceImageNew := strings.Replace(sourceImage, ".", "\\.", -1)
			image = pullSpec
			authContentAR, err = sjson.Delete(string(authContent), `auths.`+sourceImageNew)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		o.Expect(os.WriteFile(dockerTmpDirName, []byte(authContentAR), 0640)).NotTo(o.HaveOccurred())

		//_, outErr, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image).Outputs()
		//o.Expect(err).Should(o.HaveOccurred())
		//o.Expect(outErr).To(o.ContainSubstring("error: unable to read image " + image))

		// Run command oc adm release info with --idms-flag
		o.Expect(oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image, "-a", dockerTmpDirName, "--idms-file="+idmsFile64921).Execute()).NotTo(o.HaveOccurred())

		// Run command oc adm release info to get oc-mirror image
		ocMirrorImage, _, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", image, "-a", dockerTmpDirName, "--idms-file="+idmsFile64921, `-ojsonpath={.references.spec.tags[?(@.name=="oc-mirror")].from.name}`).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("ocMirrorImage is %s", ocMirrorImage)

		// Run command oc image extract with --idms-flag
		o.Expect(oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("extract", "-a", dockerTmpDirName, ocMirrorImage, "--path=/usr/bin/oc-mirror:"+extractTmpDirName, "--idms-file="+idmsFile64921, "--insecure", "--confirm").Execute()).NotTo(o.HaveOccurred())

		// Verify oc-mirror is present
		output, err := exec.Command("bash", "-c", "stat "+extractTmpDirName+"/oc-mirror").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(string(output), "File: /tmp/case64921/oc-mirror")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-High-67013-oc image mirror with multi-arch images and --filter-by-os", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		g.By("Skip if the cluster is AzureStackCloud")
		azureStackCloud, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureStackCloud == "AzureStackCloud" {
			g.Skip("Skip for cluster with AzureStackCloud!")
		}
		g.By("create new namespace")
		oc.SetupProject()
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		err := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c"+"="+serInfo.serviceName+"/testimage:ppc64", "--filter-by-os=linux/ppc64le", "--insecure").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but mirror still falied"))
		out, err := oc.WithoutNamespace().Run("image").Args("info", serInfo.serviceName+"/testimage:ppc64", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "ppc64le")).To(o.BeTrue())
		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c"+"="+serInfo.serviceName+"/testimage:default", "--insecure").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but mirror still falied"))
		o.Expect(err).NotTo(o.HaveOccurred())
		imageInfo, err := oc.WithoutNamespace().Run("image").Args("info", serInfo.serviceName+"/testimage:default", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		architecture, err := exec.Command("bash", "-c", "uname -a").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		architectureStr := string(architecture)
		if o.Expect(strings.Contains(architectureStr, "x86_64")).To(o.BeTrue()) {
			if o.Expect(strings.Contains(imageInfo, "amd64")).To(o.BeTrue()) {
				e2e.Logf("Found the expected Arch amd64")
			} else {
				e2e.Failf("Failed to find the expected Arch for mirrored image")
			}
		} else if o.Expect(strings.Contains(architectureStr, "aarch64")).To(o.BeTrue()) {
			if o.Expect(strings.Contains(imageInfo, "arm64")).To(o.BeTrue()) {
				e2e.Logf("Found the expected Arch aarch64")
			} else {
				e2e.Failf("Failed to find the expected Arch for mirrored image")
			}
		} else if o.Expect(strings.Contains(architectureStr, "ppc64le")).To(o.BeTrue()) {
			if o.Expect(strings.Contains(imageInfo, "ppc64le")).To(o.BeTrue()) {
				e2e.Logf("Found the expected Arch ppc64le")
			} else {
				e2e.Failf("Failed to find the expected Arch for mirrored image")
			}
		} else {
			if o.Expect(strings.Contains(imageInfo, "s390x")).To(o.BeTrue()) {
				e2e.Logf("Found the expected Arch s390x")
			} else {
				e2e.Failf("Failed to find the expected Arch for mirrored image")
			}
		}

	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-High-66672-Disable build & DeploymentConfig capabilities during installation", func() {
		g.By("Verify if baseLineCapabilities is set to None, enabledCapabilities on the cluster")
		if !isBaselineCapsSet(oc, "None") {
			g.Skip("Skipping the test as baselinecaps have not been set to None")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		build66672yaml := filepath.Join(buildPruningBaseDir, "build_66672.yaml")
		dc66672yaml := filepath.Join(buildPruningBaseDir, "dc_66672.yaml")

		if !isEnabledCapability(oc, "Build") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.By("Try to reterive build resources and validate an error is shown")
			buildOutput, err := oc.WithoutNamespace().Run("get").Args("build.build.openshift.io", "-A").Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(strings.Contains(buildOutput, "error: the server doesn't have a resource type \"build\"")).To(o.BeTrue())

			g.By("Try to retreive dc resource and validate an error is shown")
			dcOutput, err := oc.WithoutNamespace().Run("get").Args("dc", "-A").Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(strings.Contains(dcOutput, "error: the server doesn't have a resource type \"dc\"")).To(o.BeTrue())

			g.By("create new namespace")
			oc.SetupProject()

			g.By("Create deploymentconfig and validate that it fails")
			dcCreation, err := oc.WithoutNamespace().Run("create").Args("-f", dc66672yaml, "-n", oc.Namespace()).Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(strings.Contains(dcCreation, "no matches for kind \"DeploymentConfig\" in version \"apps.openshift.io/v1\"")).To(o.BeTrue())
			o.Expect(strings.Contains(dcCreation, "ensure CRDs are installed first")).To(o.BeTrue())

			g.By("Create build and validate that it fails")
			buildCreation, err := oc.WithoutNamespace().Run("create").Args("-f", build66672yaml, "-n", oc.Namespace()).Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(strings.Contains(buildCreation, "no matches for kind \"Build\" in version \"build.openshift.io/v1\"")).To(o.BeTrue())
		} else {
			g.Skip("Build and DeploymentConfig have been enabled as part of additional caps, so skipping")
		}
	})

})

var _ = g.Describe("[sig-cli] Workloads sos reports on Microshift", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	// author: knarra@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:knarra-Critical-63850-Critical-64919-Verify oc image extract and oc adm release info -h contains --idms-file", func() {
		g.By("Check oc image extract and oc adm release info -h does not show --icsp-file flag")
		imageExtractOutput, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(imageExtractOutput, "--idms-file")).To(o.BeTrue())
		o.Expect(strings.Contains(imageExtractOutput, "--icsp-file")).To(o.BeFalse())
		releaseInfoOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(releaseInfoOutput, "--idms-file")).To(o.BeTrue())
		o.Expect(strings.Contains(releaseInfoOutput, "--icsp-file")).To(o.BeFalse())
	})

	// author: knarra@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:knarra-High-63855-Medium-64944-Verify oc image extract and oc adm release info throws error when both --icsp-file and -idms-file flag is used", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		idmsFile63855 := filepath.Join(buildPruningBaseDir, "idmsFile63855.yaml")
		icspFile63855 := filepath.Join(buildPruningBaseDir, "icspFile63855.yaml")
		g.By("Check oc image extract and oc adm release info throws error when both --icsp-file and --idms-file flag is used")
		imageExtractOutput, err := oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", "quay.io/openshift-release-dev/ocp-release:4.12.5-x86_64", "--path=/usr/bin/oc-mirror:.", "--idms-file="+idmsFile63855, "--icsp-file="+icspFile63855, "--insecure", "--confirm").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(imageExtractOutput, "error: icsp-file and idms-file are mutually exclusive")).To(o.BeTrue())
		releaseInfoOutput, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "quay.io/openshift-release-dev/ocp-release:4.12.5-x86_64", "--idms-file="+idmsFile63855, "--icsp-file="+icspFile63855).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(releaseInfoOutput, "error: icsp-file and idms-file are mutually exclusive")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-High-12387-Check race condition in port forward connection handling logic [Serial]", func() {
		exutil.By("Check if a cluster is Microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		project12387 := "project12387"
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			oc.SetupProject()
			project12387 = oc.Namespace()
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", project12387).Execute()
			createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", project12387).Execute()
			o.Expect(createNSErr).NotTo(o.HaveOccurred())
		}

		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, project12387)

		g.By("Create pod")
		err = oc.Run("run").Args("pod12387", "--image", "quay.io/openshifttest/hello-openshift@sha256:b6296396b632d15daf9b5e62cf26da20d76157161035fefddbd0e7f7749f4167", "-n", project12387).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Make sure pod running well")
		checkPodStatus(oc, "run=pod12387", project12387, "Running")

		defer exec.Command("kill", "-9", `lsof -t -i:40032`).Output()
		cmd1, _, _, err := oc.Run("port-forward").Args("pod12387", "40032:8081").Background()
		defer cmd1.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check if port forward succeed")
		err = wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
			checkOutput, err := exec.Command("bash", "-c", "curl http://127.0.0.1:40032 --noproxy \"127.0.0.1\"").Output()
			if err != nil {
				e2e.Logf("failed to execute the curl: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("Hello OpenShift", string(checkOutput)); matched {
				e2e.Logf("Check the port-forward command succeeded\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot get the port-forward result"))
		g.By("check concurrency request")
		var wg sync.WaitGroup
		for i := 0; i < 30; i++ {
			wg.Add(1)
			go func() {
				defer g.GinkgoRecover()
				defer wg.Done()
				_, err := exec.Command("bash", "-c", "curl http://127.0.0.1:40032 --noproxy \"127.0.0.1\"").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
			}()
		}
		wg.Wait()
	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-28007-Checking oc version show clean as gitTreeState value", func() {
		out, err := oc.Run("version").Args("-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		versionInfo := &VersionInfo{}
		if err := json.Unmarshal([]byte(out), &versionInfo); err != nil {
			e2e.Failf("unable to decode version with error: %v", err)
		}
		if match, _ := regexp.MatchString("clean", versionInfo.ClientInfo.GitTreeState); !match {
			e2e.Failf("varification GitTreeState with error: %v", err)
		}

	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-High-43030-oc get events always show the timestamp as LAST SEEN", func() {
		// Check if cluster is microshift or OCP
		exutil.By("Check if cluster is microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		exutil.By("Get all the namespaces")
		var output string
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			output, err = oc.AsAdmin().Run("get").Args("projects", "-o=custom-columns=NAME:.metadata.name", "--no-headers").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			output, err = oc.AsAdmin().Run("get").Args("ns", "-o=custom-columns=NAME:.metadata.name", "--no-headers").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		projectList := strings.Fields(output)

		g.By("check the events per project")
		for _, projectN := range projectList {
			output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", projectN).Output()
			if match, _ := regexp.MatchString("No resources found", string(output)); match {
				e2e.Logf("No events in project: %v", projectN)
			} else {
				result, _ := exec.Command("bash", "-c", "cat "+output+" | awk '{print $1}'").Output()
				if match, _ := regexp.MatchString("unknown", string(result)); match {
					e2e.Failf("Does not show timestamp as expected: %v", result)
				}
			}
		}

	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-VMonly-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-42983-always delete the debug pod when the oc debug node command exist [Flaky]", func() {
		exutil.By("Check if a cluster is Microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		project42983 := "project42983"
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			oc.SetupProject()
			project42983 = oc.Namespace()
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", project42983).Execute()
			createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", project42983).Execute()
			o.Expect(createNSErr).NotTo(o.HaveOccurred())
		}

		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, project42983)

		g.By("Get all the node name list")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeList := strings.Fields(out)

		g.By("Run debug node")
		for _, nodeName := range nodeList {
			err = oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+nodeName, "-n", project42983, "--", "chroot", "/host", "date").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Make sure debug pods have been deleted")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("pods", "-n", project42983).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matched, _ := regexp.MatchString("No resources found", output); !matched {
				e2e.Logf("pods still not deleted :\n%s, try again ", output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "pods still not deleted")

	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-34155-oc get events sorted by lastTimestamp", func() {
		g.By("Get events sorted by lastTimestamp")
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", "openshift-operator-lifecycle-manager", "--sort-by="+".lastTimestamp").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-47555-Should not update data when use oc set data with dry-run as server", func() {
		exutil.By("Check if cluster is microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		project47555 := "project47555"
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			oc.SetupProject()
			project47555 = oc.Namespace()
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", project47555).Execute()
			createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", project47555).Execute()
			o.Expect(createNSErr).NotTo(o.HaveOccurred())
		}

		g.By("Create new configmap")
		err = oc.Run("create").Args("configmap", "cm-47555", "--from-literal=name=abc", "-n", project47555).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Save the data for configmap")
		beforeSetcm, err := oc.Run("get").Args("cm", "cm-47555", "-o=jsonpath={.data.name}", "-n", project47555).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run the set with server dry-run")
		err = oc.Run("set").Args("data", "cm", "cm-47555", "--from-literal=name=def", "--dry-run=server", "-n", project47555).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		afterSetcm, err := oc.Run("get").Args("cm", "cm-47555", "-o=jsonpath={.data.name}", "-n", project47555).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if match, _ := regexp.MatchString(beforeSetcm, afterSetcm); !match {
			e2e.Failf("Should not persistent update configmap with server dry-run")
		}
		g.By("Create new secret")
		err = oc.Run("create").Args("secret", "generic", "secret-47555", "--from-literal=name=abc", "-n", project47555).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Save the data for secret")
		beforeSetse, err := oc.Run("get").Args("secret", "secret-47555", "-o=jsonpath={.data.name}", "-n", project47555).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run the set with server dry-run")
		err = oc.Run("set").Args("data", "secret", "secret-47555", "--from-literal=name=def", "--dry-run=server", "-n", project47555).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		afterSetse, err := oc.Run("get").Args("secret", "secret-47555", "-o=jsonpath={.data.name}", "-n", project47555).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if match, _ := regexp.MatchString(beforeSetse, afterSetse); !match {
			e2e.Failf("Should not persistent update secret with server dry-run")
		}

	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-49116-oc debug should remove startupProbe when create debug pod", func() {
		exutil.By("Check if cluster is microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		project49116 := "project49116"
		_, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			oc.SetupProject()
			project49116 = oc.Namespace()
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", project49116).Execute()
			createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", project49116).Execute()
			o.Expect(createNSErr).NotTo(o.HaveOccurred())
		}

		g.By("Create the deploy")
		err = oc.Run("create").Args("deploy", "d49116", "--image", "quay.io/openshifttest/hello-openshift@sha256:56c354e7885051b6bb4263f9faa58b2c292d44790599b7dde0e49e7c466cf339", "-n", project49116).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("patch the deploy with startupProbe")
		patchS := `[{"op": "add", "path": "/spec/template/spec/containers/0/startupProbe", "value":{ "exec": {"command": [ "false" ]}}}]`
		err = oc.Run("patch").Args("deploy", "d49116", "--type=json", "-p", patchS, "-n", project49116).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("run the debug with jsonpath")
		out, err := oc.Run("debug").Args("deploy/d49116", "-o=jsonpath='{.spec.containers[0].startupProbe}'", "-n", project49116).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if out != "''" {
			e2e.Failf("The output should be empty, but not: %v", out)
		}
	})

	// author: knarra@redhat.com
	g.It("MicroShiftBoth-Author:knarra-Medium-28018-Workloads Custom label for pvc in statefulsets", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		deployStatefulSet := filepath.Join(buildPruningBaseDir, "stable-storage.yaml")

		g.By("Check if default sc exists, if not, skip the test")
		allSC, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", "-o", "json").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defaultSC := gjson.Get(allSC, "items.#(metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class=true)#.metadata.name")
		e2e.Logf("The default storageclass list: %s", defaultSC)

		g.By("Skip the test if length of defaultsc is less than one")
		defaultSCCount := len(defaultSC.Array())
		if defaultSCCount != 1 {
			g.Skip("Skip for unexpected default storageclass!")
		}

		exutil.By("Check if cluster is microshift or OCP")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		project28018 := "project28018"
		_, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "microshift version")
		if err != nil {
			oc.SetupProject()
			project28018 = oc.Namespace()
		} else {
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", project28018).Execute()
			createNSErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", project28018).Execute()
			o.Expect(createNSErr).NotTo(o.HaveOccurred())
		}

		g.By("Create stable storage stateful set")
		creationErr := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", deployStatefulSet, "-n", project28018).Execute()
		o.Expect(creationErr).NotTo(o.HaveOccurred())

		if defaultSC.Array()[0].String() == "filestore-csi" || strings.Contains(defaultSC.Array()[0].String(), "powervs") {
			waitForPvcStatus(oc, oc.Namespace(), "www-hello-statefulset-0")
		}

		g.By("Check if pod is ready")
		exutil.AssertPodToBeReady(oc, "hello-statefulset-0", project28018)

		g.By("Check if the pvc is ready")
		pvcOutput, pvcCreationErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "www-hello-statefulset-0", "-n", project28018, "--template={{.metadata.labels}}").Output()
		o.Expect(pvcCreationErr).NotTo(o.HaveOccurred())
		o.Expect(pvcOutput).NotTo(o.BeEmpty())
		o.Expect(pvcOutput).To(o.ContainSubstring("app:hello-pod"))
	})

	// author: knarra@redhat.com
	g.It("MicroShiftOnly-Author:knarra-Critical-60924-Critical-60929-High-68257-Verify sos report -l lists enabled microshift plugins and Verify running sos report -p microshift collects microshift and microshift config related information [Serial]", func() {

		g.By("Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		g.By("List all the available plugins for microshift")
		pluginList, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo sos report -l")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Available plugins for microshift is: %v", string(pluginList))
		if err != nil {
			e2e.Failf("Error occured during listing of microshift plugins: %v", err.Error())
		}
		o.Expect(strings.Contains(pluginList, "microshift")).To(o.BeTrue())
		o.Expect(strings.Contains(pluginList, "microshift_ovn")).To(o.BeTrue())

		// Create a directory in /tmp folder to collect microshift sos report
		g.By("Create a folder in tmp to collect sos report")
		defer exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "rm -rf /tmp/test60929")
		_, creationErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "mkdir /tmp/test60929")
		o.Expect(creationErr).NotTo(o.HaveOccurred())

		g.By("Verify running sos report -p works fine")
		sosreportStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo sos report --batch --clean --all-logs --profile microshift --tmp-dir=/tmp/test60929")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(sosreportStatus, "Your sosreport has been generated and saved in")).To(o.BeTrue())
		o.Expect(strings.Contains(sosreportStatus, "/tmp/test60929/sosreport")).To(o.BeTrue())

		// Code to extract the sosreport & it's name
		extractSosReportName := strings.Split(sosreportStatus, "Your sosreport has been generated and saved in")
		sosreportNames := strings.Split(extractSosReportName[1], "\n")
		sosreportName := strings.Split(sosreportNames[1], "/")
		sosreportnameaExtraction := strings.Split(sosreportName[3], "-obfuscated")
		e2e.Logf("sosreportnameaExtraction is %v", sosreportnameaExtraction)
		if err != nil {
			e2e.Failf("Error occured running with microshift profile: %v", err.Error())
		}
		extrattarCmd := fmt.Sprintf(`
cat > /tmp/extracttarfile.sh << EOF
sudo tar -xvf %v -C /tmp/test60929`, sosreportNames[1])

		g.By("Execute the /tmp/extracttarfile.sh to extract the generated sosreport")
		_, executeConfigErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", extrattarCmd)
		o.Expect(executeConfigErr).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sh -x /tmp/extracttarfile.sh")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Read sos.txt from the extracted sos report & verify it ran commands related to `oc adm inspect` & `ovs-appctl`")
		readSOSTextCmd := fmt.Sprintf(`cat /tmp/test60929/%v/sos_reports/sos.txt`, sosreportnameaExtraction[0])
		outputFromExtractedSosText, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", readSOSTextCmd)
		e2e.Logf("outputfromextractedsostext is %v", outputFromExtractedSosText)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(outputFromExtractedSosText, "oc adm inspect")).To(o.BeTrue())
		o.Expect(strings.Contains(outputFromExtractedSosText, "ovs-appctl -t /var/run/ovn/ovn-controller.*.ctl ct-zone-list")).To(o.BeTrue())

		g.By("Verify microshift config files are collected in sos report")
		mConfigFileStatus, mConfigFileErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo ls -l  /etc/microshift | awk '{print $NF}' | tail -n+2")
		o.Expect(mConfigFileErr).NotTo(o.HaveOccurred())
		e2e.Logf("mConfigFileStatus is \n %s", mConfigFileStatus)

		readMConfigCmd := fmt.Sprintf(`ls -l /tmp/test60929/%v/etc/microshift | awk '{print $NF}' | tail -n+2`, sosreportnameaExtraction[0])
		extractedMConfigOut, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", readMConfigCmd)
		e2e.Logf("ExtractedMicroshiftConfigOutput is \n %v", extractedMConfigOut)
		if extractedMConfigOut != mConfigFileStatus {
			e2e.Failf("Not all microshift config files are collected in sosreport")
		}

		manifestList, manifestListErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo ls -l  /etc/microshift/manifests")
		o.Expect(manifestListErr).NotTo(o.HaveOccurred())
		e2e.Logf("manifestList is \n %s", manifestList)

		readManifestsCmd := fmt.Sprintf(`ls -l /tmp/test60929/%v/etc/microshift/manifests`, sosreportnameaExtraction[0])
		extractedManifestsOut, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", readManifestsCmd)
		e2e.Logf("ExtractedManifestOut is \n %v", extractedManifestsOut)
		if extractedManifestsOut != manifestList {
			e2e.Failf("Files inside /etc/microshift/manifest directory does not match system and sosreport")
		}

		manifestListD, manifestListErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo ls -l  /etc/microshift/manifests.d/")
		o.Expect(manifestListErr).NotTo(o.HaveOccurred())
		e2e.Logf("manifestListerror is \n %s", manifestListD)

		readManifestsDCmd := fmt.Sprintf(`ls -l /tmp/test60929/%v/etc/microshift/manifests.d/`, sosreportnameaExtraction[0])
		extractedManifestsDOut, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", readManifestsDCmd)
		e2e.Logf("ExtractedManifestOut is \n %v", extractedManifestsDOut)
		if extractedManifestsDOut != manifestListD {
			e2e.Failf("Files inside /etc/microshift/manifests.d directory does not match between system and sosreport")
		}

	})

	// author: knarra@redhat.com
	g.It("MicroShiftOnly-Author:knarra-High-60930-Verify running sos report collect microshift related information [Serial]", func() {
		g.By("Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		g.By("Verify running sos report collects microshift related information")
		sosreportStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo sos report --batch")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("sosreport status is: %v", string(sosreportStatus))
		o.Expect(strings.Contains(sosreportStatus, "Your sosreport has been generated and saved in")).To(o.BeTrue())
		o.Expect(strings.Contains(sosreportStatus, "/var/tmp/sosreport")).To(o.BeTrue())
		if err != nil {
			e2e.Failf("Error occured running with sos report: %v", err.Error())
		}

	})

	// author: knarra@redhat.com
	g.It("MicroShiftOnly-Author:knarra-High-68256-Verify greenboot logs reside in a separate directory and easy to access [Serial]", func() {
		g.By("Get microshift node")
		masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
		o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
		o.Expect(masterNodes).NotTo(o.BeEmpty())

		g.By("Verify running sos report -p system collects greenbot logs")
		sosreportStatus, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sudo sos report --batch --clean --all-logs --profile system")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(sosreportStatus, "Your sosreport has been generated and saved in")).To(o.BeTrue())
		o.Expect(strings.Contains(sosreportStatus, "/var/tmp/sosreport")).To(o.BeTrue())

		// Code to extract the sosreport & it's name
		extractSosReportName := strings.Split(sosreportStatus, "Your sosreport has been generated and saved in")
		sosreportNames := strings.Split(extractSosReportName[1], "\n")
		sosreportName := strings.Split(sosreportNames[1], "/")
		sosreportnameaExtraction := strings.Split(sosreportName[3], "-obfuscated")
		e2e.Logf("sosreportnameaExtraction is %v", sosreportnameaExtraction)
		if err != nil {
			e2e.Failf("Error occured running with microshift profile: %v", err.Error())
		}
		defer exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "rm -rf /tmp/test68256")
		extrattarCmd := fmt.Sprintf(`
cat > /tmp/extracttarfile.sh << EOF
mkdir /tmp/test68256
sudo tar -xvf %v -C /tmp/test68256`, sosreportNames[1])

		g.By("Execute the /tmp/extracttarfile.sh to extract the generated sosreport")
		_, executeConfigErr := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", extrattarCmd)
		o.Expect(executeConfigErr).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", "sh -x /tmp/extracttarfile.sh")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check if greenbot direcorty and it's related files are present in the extracted sos report")
		checkGreenBootFilesCmd := fmt.Sprintf(`ls -l /tmp/test68256/%v/sos_commands/greenboot`, sosreportnameaExtraction[0])
		greenbootFilesOut, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", checkGreenBootFilesCmd)
		e2e.Logf("outputfromgreenbot dir is %v", greenbootFilesOut)
		o.Expect(err).NotTo(o.HaveOccurred())
		greenBotFileList := []string{"journalctl_--no-pager_--unit_greenboot-healthcheck", "journalctl_--no-pager_--unit_greenboot-task-runner", "journalctl_--no-pager_--unit_redboot-task-runner", "systemctl_status_greenboot-healthcheck", "systemctl_status_greenboot-task-runner", "systemctl_status_redboot-task-runner"}
		for _, gbFile := range greenBotFileList {
			o.Expect(strings.Contains(greenbootFilesOut, gbFile)).To(o.BeTrue())
		}

		g.By("Check if greenbot.conf file is present in the extracted sos report")
		greenBootConfFileCmd := fmt.Sprintf(`ls -l /tmp/test68256/%v/etc/greenboot`, sosreportnameaExtraction[0])
		greenbootConfFileOut, err := exutil.DebugNodeWithOptionsAndChroot(oc, masterNodes[0], []string{"-q"}, "bash", "-c", greenBootConfFileCmd)
		e2e.Logf("outputfrometcgreenboot dir is %v", greenbootConfFileOut)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(greenbootConfFileOut, "greenboot.conf")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("MicroShiftBoth-ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-66724-oc explain should be work for all the clusterresource [Serial]", func() {
		clusterResourceFile, err := oc.AsAdmin().WithoutNamespace().Run("api-resources").Args("--no-headers").OutputToFile("apiresourceout.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterResourceList, err := getClusterResourceName(clusterResourceFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, resource := range clusterResourceList {
			_, explainErr, _ := oc.AsAdmin().WithoutNamespace().Run("explain").Args(resource).Outputs()
			if explainErr != "" {
				if strings.Contains(explainErr, "couldn't find resource") || strings.Contains(explainErr, "not found") {
					e2e.Logf("Could not get the current crd %v, will skip and continue", resource)
				} else {
					e2e.Failf("Explain failed with the current resource ")
				}
			}
		}
	})

})

var _ = g.Describe("[sig-cli] Workloads client test", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Low-12021-Return description with cli describe with invalid parameter", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}
		// Skip the test if baselinecaps is set to none, v4.12 or v4.13
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") || isBaselineCapsSet(oc, "v4.11") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns12021 := oc.Namespace()

		g.By("Create the build")
		err := oc.WithoutNamespace().Run("new-build").Args("-D", "FROM must-gather", "-n", ns12021).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the deploy app")
		err = oc.WithoutNamespace().Run("new-app").Args("--image", "quay.io/openshifttest/deployment-example@sha256:9d29ff0fdbbec33bb4eebb0dbe0d0f3860a856987e5481bb0fc39f3aba086184", "-n", ns12021, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err := oc.WithoutNamespace().Run("describe").Args("services", "-n", ns12021).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "deployment-example")).To(o.BeTrue())
		out, err = oc.WithoutNamespace().Run("describe").Args("bc", "-n", ns12021).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "ImageStreamTag openshift/must-gather:latest")).To(o.BeTrue())
		o.Expect(strings.Contains(out, "ImageStreamTag must-gather:latest")).To(o.BeTrue())
		out, err = oc.WithoutNamespace().Run("describe").Args("build", "-n", ns12021).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "buildconfig=must-gather")).To(o.BeTrue())
		out, err = oc.WithoutNamespace().Run("describe").Args("builds", "abc", "-n", ns12021).Output()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(out, "not found")).To(o.BeTrue())

	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Medium-54406-Medium-54407-Medium-11564-oc rsh should work behind authenticated proxy", func() {
		var httpOriginProxy, httpsOriginProxy string
		httpOriginProxy = os.Getenv("http_proxy")
		httpsOriginProxy = os.Getenv("https_proxy")
		e2e.Logf("httpOriginProxy is %v", httpOriginProxy)
		e2e.Logf("httpsOriginProxy is %v", httpsOriginProxy)
		if httpOriginProxy == "" && httpsOriginProxy == "" {
			g.Skip("Skipping the test as no porxy setting")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns54406 := oc.Namespace()
		g.By("Create the test pod")
		err := oc.WithoutNamespace().Run("run").Args("mypod54406", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns54406).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertPodOutput(oc, "run=mypod54406", ns54406, "Running")
		g.By("Run rsh command")
		err = oc.WithoutNamespace().Run("rsh").Args("-n", ns54406, "mypod54406").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run exec command")
		err = oc.WithoutNamespace().Run("exec").Args("-n", ns54406, "mypod54406", "--", "date").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run port-forward command")
		cmd2, _, _, err := oc.Run("port-forward").Args("-n", ns54406, "mypod54406", "40032:8081").Background()
		defer cmd2.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Low-66124-Check deprecate DeploymentConfigs in 4.14", func() {
		// Skip the test if baselinecaps is set to None or v4.13 or v4.12
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") || isBaselineCapsSet(oc, "v4.11") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns66124 := oc.Namespace()
		_, warningOut, err := oc.WithoutNamespace().Run("create").Args("deploymentconfig", "dc66124-1", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOut, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutGet, err := oc.WithoutNamespace().Run("get").Args("deploymentconfig", "dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutGet, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutSet, err := oc.WithoutNamespace().Run("set").Args("env", "deploymentconfig", "dc66124-1", "keyname=keyvalue", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutSet, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutExp, err := oc.WithoutNamespace().Run("expose").Args("deploymentconfig", "dc66124-1", "--port=40032", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutExp, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutLab, err := oc.WithoutNamespace().Run("label").Args("deploymentconfig", "dc66124-1", "test=label", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutLab, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutAnn, err := oc.WithoutNamespace().Run("annotate").Args("deploymentconfig", "dc66124-1", "test=annotate", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutAnn, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutSca, err := oc.WithoutNamespace().Run("scale").Args("deploymentconfig", "dc66124-1", "--replicas=2", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutSca, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutApp, err := oc.WithoutNamespace().Run("apply").Args("view-last-applied", "deploymentconfig", "dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForDeploymentconfigToBeReady(oc, ns66124, "dc66124-1")
		o.Expect(strings.Contains(warningOutApp, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutLog, err := oc.WithoutNamespace().Run("logs").Args("deploymentconfig/dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutLog, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutExe, err := oc.WithoutNamespace().Run("exec").Args("deploymentconfig/dc66124-1", "-n", ns66124, "--", "date").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutExe, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutDeb, err := oc.WithoutNamespace().Run("debug").Args("deploymentconfig/dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutDeb, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutDes, err := oc.WithoutNamespace().Run("describe").Args("deploymentconfig", "dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutDes, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutPat, err := oc.WithoutNamespace().Run("patch").Args("deploymentconfig", "dc66124-1", "-n", ns66124, "--type=merge", "-p", "{\"spec\":{\"replicas\":5}}").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutPat, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutRol, err := oc.WithoutNamespace().Run("rollout").Args("pause", "deploymentconfig", "dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutRol, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutRoll, err := oc.WithoutNamespace().Run("rollout").Args("resume", "deploymentconfig", "dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutRoll, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutNew, err := oc.WithoutNamespace().Run("new-app").Args("--as-deployment-config", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutNew, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
		_, warningOutDel, err := oc.WithoutNamespace().Run("delete").Args("dc/dc66124-1", "-n", ns66124).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutDel, "DeploymentConfig is deprecated in v4.14")).To(o.BeTrue())
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-LEVEL0-High-67387-oc new-app propagate containerPort information to the deployment if import-mode is default", func() {
		// Skip case on multi-arch cluster
		architecture.SkipArchitectures(oc, architecture.MULTI)
		// Skip case on cluster without imageRegistry
		if !isEnabledCapability(oc, "ImageRegistry") {
			g.Skip("Skipped: cluster does not have imageRegistry installed")
		}
		ocPlatform := checkOcPlatform(oc)
		serverPlatform := architecture.ClusterArchitecture(oc)
		if ocPlatform != serverPlatform.String() {
			g.Skip(fmt.Sprintf("Skip for oc and cluster platform mismatch : %s  %s", ocPlatform, serverPlatform.String()))
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns67387 := oc.Namespace()

		err := oc.WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns67387, "--name=example-app67387").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.WithoutNamespace().Run("get").Args("svc", "-n", ns67387).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "example-app67387")).To(o.BeTrue())
		waitForDeploymentPodsToBeReady(oc, ns67387, "example-app67387")
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:yinzhou-Medium-49395-oc debug node should exit when timeout", func() {
		workerNodeList, err := exutil.GetClusterNodesBy(oc, "worker")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Create new namespace")
		oc.SetupProject()
		ns49395 := oc.Namespace()

		exutil.SetNamespacePrivileged(oc, ns49395)

		cmd := fmt.Sprintf("oc debug --to-namespace %s node/%s", ns49395, workerNodeList[0])
		e2e.Logf("cmd: %v", cmd)
		e, _, err := expect.Spawn(cmd, -1, expect.Verbose(true))
		o.Expect(err).NotTo(o.HaveOccurred())
		defer e.Close()
		err = wait.Poll(10*time.Second, 600*time.Second, func() (bool, error) {
			output, err1 := oc.AsAdmin().Run("get").Args("pod", "-n", ns49395).Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			if matched, _ := regexp.MatchString("debug", output); matched {
				e2e.Logf("Check the debug pod in own namespace\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Cannot find the debug pod in own namespace"))
		err = wait.Poll(30*time.Second, 960*time.Second, func() (bool, error) {
			output, err1 := oc.AsAdmin().Run("get").Args("pod", "-n", ns49395).Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			if matched, _ := regexp.MatchString("debug", output); !matched {
				e2e.Logf("Check the debug pod disappeared in own namespace\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Still find the debug pod in own namespace even wait for 15 mins"))
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-High-37363-High-38859-Check oc image mirror with multi-arch images", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		g.By("Skip if the cluster is AzureStackCloud")
		azureStackCloud, azureErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.azure.cloudName}").Output()
		o.Expect(azureErr).NotTo(o.HaveOccurred())
		if azureStackCloud == "AzureStackCloud" {
			g.Skip("Skip for cluster with AzureStackCloud!")
		}
		g.By("Create new namespace")
		oc.SetupProject()
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Checkpoint for OCP-38859")
		err := wait.Poll(30*time.Second, 800*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().Run("image").Args("mirror", "--insecure", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", serInfo.serviceName+"/busyboxmulti:latest", "--filter-by-os=.*").Execute()
			if err != nil {
				if apierrors.IsServiceUnavailable(err) {
					e2e.Logf("Registry route not available, retrying...")
					return false, nil
				}
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Mirror failed")
		_, warningOutput, err := oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/busyboxmulti:latest").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutput, "the image is a manifest list and contains multiple images - use --filter-by-os to select from")).To(o.BeTrue())
		g.By("Checkpoint for OCP-37363")
		err = wait.Poll(30*time.Second, 800*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().Run("image").Args("mirror", "--insecure", "quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", serInfo.serviceName+"/busyboxmultilist:latest", "--keep-manifest-list=true").Execute()
			if err != nil {
				if apierrors.IsServiceUnavailable(err) {
					e2e.Logf("Registry route not available, retrying...")
					return false, nil
				}
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Mirror failed")
		_, warningOutput2, err := oc.WithoutNamespace().Run("image").Args("info", "--insecure", serInfo.serviceName+"/busyboxmultilist:latest").Outputs()
		o.Expect(err).Should(o.HaveOccurred())
		o.Expect(strings.Contains(warningOutput2, "the image is a manifest list and contains multiple images - use --filter-by-os to select from")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-68405-oc process works well for cross-namespace template", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}
		g.By("Create new namespace")
		oc.SetupProject()
		nsName1 := oc.Namespace()
		g.By("Create template in the first project")
		temFile, err := oc.WithoutNamespace().Run("adm").Args("create-bootstrap-project-template", "-o", "yaml").OutputToFile("projectT.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("create").Args("-f", temFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Create the second  namespace")
		oc.SetupProject()
		g.By("Process the templete in the first namespace")
		err = oc.AsAdmin().Run("process").Args(nsName1 + "//project-request").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		//checkpoint for OCPBUGS-24375, oc process command succeed while running it with a template file cross namespace
		oc.SetupProject()
		_, templateErr, _ := oc.WithoutNamespace().Run("get").Args("template", "httpd-example", "-n", "openshift").Outputs()
		if strings.Contains(templateErr, "not found") {
			g.Skip("Can't find the template, skip checkpoint for OCPBUGS-24375.")
		}

		tmeFile2, err := oc.WithoutNamespace().Run("get").Args("template", "httpd-example", "-n", "openshift", "-o", "yaml").OutputToFile("httpdexampleT.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().Run("process").Args("-f", tmeFile2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Low-68670-oc whoami works well with oauth operator", func() {
		g.By("Create new namespace")
		oc.SetupProject()
		err := oc.Run("whoami").Args("").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("whoami").Args("").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Low-11147-Show RC information and indicate bad secrets reference in oc status", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}

		// Skip the test if baselinecaps is set to v4.13 or v4.14
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.11") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}
		g.By("Create new namespace")
		oc.SetupProject()
		workloadsBaseDir := exutil.FixturePath("testdata", "workloads")
		rcFile := filepath.Join(workloadsBaseDir, "only-rc.yaml")
		templateFile := filepath.Join(workloadsBaseDir, "application-template-stibuild-with-mount-secret.json")
		rcSecretFile := filepath.Join(workloadsBaseDir, "rc-match-service.yaml")

		g.By("Check standalone RC info is dispalyed in oc status output")
		err := oc.Run("create").Args("-f", rcFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, _, err := oc.Run("status").Args().Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "rc/stdalonerc")).To(o.BeTrue())
		output, _, err = oc.Run("status").Args("--suggest").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "rc/stdalonerc is attempting to mount a missing secret secret/mysecret")).To(o.BeTrue())
		err = oc.Run("delete").Args("-f", rcFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check DC info when has missing/bad secret reference")
		err = oc.Run("create").Args("-f", templateFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("new-app").Args("--template=ruby-helloworld-sample").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, _, err = oc.Run("status").Args("--suggest").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "dc/frontend is attempting to mount a missing secret secret/my-secret")).To(o.BeTrue())

		g.By("Show RCs for services in oc status")
		err = oc.Run("create").Args("-f", rcSecretFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, _, err = oc.Run("status").Args("--suggest").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "svc/database")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "  rc/rcmatchse runs")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "    rc/rcmatchse created")).To(o.BeTrue())
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Low-11202-Use oc explain to see detailed documentation of resources", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") || isBaselineCapsSet(oc, "v4.11") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}
		g.By("Check if baremetal cluster")
		iaasPlatform := exutil.CheckPlatform(oc)
		if iaasPlatform == "baremetal" {
			e2e.Logf("Cluster is: %s", iaasPlatform)
			g.Skip("For baremetal cluster , this is something wrong for proxy setting, so skip it for temp!")
		}
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.Nutanix, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud)
		out, err := oc.Run("explain").Args("po").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "Pod is a collection of containers")).To(o.BeTrue())
		out, err = oc.Run("explain").Args("pods.spec.containers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "securityContext")).To(o.BeTrue())
		err = oc.Run("explain").Args("rc.spec.selector").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("explain").Args("none-exist").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		err = oc.Run("explain").Args("rc,no").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		out, err = oc.Run("explain").Args("dc.apiVersion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		contextOutList := strings.Fields(strings.ReplaceAll(out, "\n\n", "\n"))
		docResource := contextOutList[len(contextOutList)-1]
		e2e.Logf("The detailed documentation resource url is %v", docResource)
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			resp, err := http.Get(docResource)
			if err != nil {
				e2e.Logf("Err Occurred: %v, try next time", err)
				return false, nil
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 302 {
				e2e.Logf("Could get the detailed documentation of resources url")
				return true, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to get assert the detailed document resource url")
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Low-21115-Use kubelet explain to see detailed documentation of resources", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") || isBaselineCapsSet(oc, "v4.11") || isBaselineCapsSet(oc, "v4.14") || isBaselineCapsSet(oc, "v4.15") && !isEnabledCapability(oc, "DeploymentConfig") {
			g.Skip("Skipping the test as baselinecaps have been set and some of API capabilities are not enabled!")
		}
		clusterinfra.SkipTestIfSupportedPlatformNotMatched(oc, clusterinfra.AWS, clusterinfra.Azure, clusterinfra.GCP, clusterinfra.VSphere, clusterinfra.Nutanix, clusterinfra.IBMCloud, clusterinfra.AlibabaCloud)
		out, err := oc.WithKubectl().Run("explain").Args("po").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "Pod is a collection of containers")).To(o.BeTrue())
		out, err = oc.WithKubectl().Run("explain").Args("pods.spec.containers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "securityContext")).To(o.BeTrue())
		err = oc.WithKubectl().Run("explain").Args("rc.spec.selector").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithKubectl().Run("explain").Args("none-exist").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		err = oc.WithKubectl().Run("explain").Args("rc,no").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		out, err = oc.WithKubectl().Run("explain").Args("dc.apiVersion").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		contextOutList := strings.Fields(strings.ReplaceAll(out, "\n\n", "\n"))
		docResource := contextOutList[len(contextOutList)-1]
		e2e.Logf("The detailed documentation resource url is %v", docResource)
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			resp, err := http.Get(docResource)
			if err != nil {
				e2e.Logf("Err Occurred: %v, try next time", err)
				return false, nil
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 302 {
				e2e.Logf("Could get the detailed documentation of resources url")
				return true, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to get assert the detailed document resource url")
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Low-54411-Low-21060-kubectl exec should work behind authenticated proxy", func() {
		var httpOriginProxy, httpsOriginProxy string
		httpOriginProxy = os.Getenv("http_proxy")
		httpsOriginProxy = os.Getenv("https_proxy")
		e2e.Logf("httpOriginProxy is %v", httpOriginProxy)
		e2e.Logf("httpsOriginProxy is %v", httpsOriginProxy)
		if httpOriginProxy == "" && httpsOriginProxy == "" {
			g.Skip("Skipping the test as no porxy setting")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns54406 := oc.Namespace()
		g.By("Create the test pod")
		err := oc.WithoutNamespace().Run("run").Args("mypod54406", "--image=quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns54406).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertPodOutput(oc, "run=mypod54406", ns54406, "Running")
		g.By("Run exec command")
		err = oc.WithKubectl().Run("exec").Args("mypod54406", "--", "date").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Run port-forward command")
		defer exec.Command("kill", "-9", `lsof -t -i:40035`).Output()
		cmd2, _, _, err := oc.WithKubectl().Run("port-forward").Args("mypod54406", "40035:8081").Background()
		defer cmd2.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Longduration-NonPreRelease-Author:yinzhou-High-68647-oc whoami must work without oauth-apiserver [Slow][Disruptive]", func() {
		exutil.By("Create new namespace")
		oc.SetupProject()
		workloadsBaseDir := exutil.FixturePath("testdata", "workloads")
		keycloakFile := filepath.Join(workloadsBaseDir, "keycloak.yaml")
		exutil.By("Create keycloak app")
		err1 := nonAdminApplyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", keycloakFile, "-p", "KEYCLOAK_ADMIN=admin", "KEYCLOAK_ADMIN_PASSWORD="+exutil.GetRandomString(), "NAMESPACE=="+oc.Namespace())
		o.Expect(err1).NotTo(o.HaveOccurred())
		if ok := waitForAvailableRsRunning(oc, "deployment", "keycloak", oc.Namespace(), "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			err := oc.Run("get").Args("pod", "-n", "-o", "yaml").Execute()
			if err != nil {
				e2e.Logf("Failed to get pod info with err %v", err)
			}
			e2e.Failf("Keycloak pods are not running as expected")
		}

		kc_Host, kcerr := oc.Run("get").Args("route", "keycloak", "-o=jsonpath={.spec.host}").Output()
		o.Expect(kc_Host).NotTo(o.BeEmpty())
		o.Expect(kcerr).NotTo(o.HaveOccurred())
		kc_URL := fmt.Sprintf("https://%s/realms/master", kc_Host)

		exutil.By("Set the cluster with external OIDC issuer")
		privateCert, certErr := oc.AsAdmin().Run("patch").Args("cm", "-n", "openshift-config-managed", "default-ingress-cert", "-p", "{\"metadata\":{\"namespace\":\"openshift-config\",\"name\":\"default-ingress-cert-68647\"}}", "--dry-run=client", "-o", "yaml").OutputToFile(getRandomString() + "workload-cm.yaml")
		o.Expect(certErr).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("delete").Args("-f", privateCert, "-n", "openshift-config").Execute()
		cmErr := oc.AsAdmin().Run("apply").Args("-f", privateCert, "-n", "openshift-config").Execute()
		o.Expect(cmErr).NotTo(o.HaveOccurred())

		defaultCert, defaultCertErr := oc.AsAdmin().Run("get").Args("proxy", "cluster", "-o=jsonpath={.spec.trustedCA.name}").Output()
		o.Expect(defaultCertErr).NotTo(o.HaveOccurred())

		var proxyrestorePatch = `[{"op": "replace", "path": "/spec/trustedCA/name", "value": ""}]`
		if len(defaultCert) != 0 {
			proxyrestorePatch = fmt.Sprintf(`[{"op": "replace", "path": "/spec/trustedCA/name", "value": %s}]`, defaultCert)
		}

		defer oc.AsAdmin().Run("patch").Args("proxy", "cluster", "--type=json", "-p", proxyrestorePatch).Execute()
		patch := `[{"op": "replace", "path": "/spec/trustedCA/name", "value": default-ingress-cert-68647}]`
		patchProxyErr := oc.AsAdmin().Run("patch").Args("proxy", "cluster", "-p", patch, "--type=json").Execute()
		o.Expect(patchProxyErr).NotTo(o.HaveOccurred())
		defer os.RemoveAll("/tmp/oauthMetadata")
		curlCMD := fmt.Sprintf(`curl -o /tmp/oauthMetadata -k %s/.well-known/openid-configuration`, kc_URL)
		_, curlErr := exec.Command("bash", "-c", curlCMD).Output()
		o.Expect(curlErr).NotTo(o.HaveOccurred())

		defer oc.AsAdmin().Run("delete").Args("cm", "oauth-meta-68647", "-n", "openshift-config").Execute()
		cmErr = oc.AsAdmin().Run("create").Args("configmap", "oauth-meta-68647", "--from-file", "/tmp/oauthMetadata", "-n", "openshift-config").Execute()
		o.Expect(cmErr).NotTo(o.HaveOccurred())

		originApiunsupportedConfigOverrides, err := oc.AsAdmin().Run("get").Args("kubeapiserver", "cluster", "-o=jsonpath={.spec.unsupportedConfigOverrides}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(originApiunsupportedConfigOverrides)
		originoauthMetadataName, err := oc.AsAdmin().Run("get").Args("authentication", "cluster", "-o=jsonpath={.spec.oauthMetadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originoauthType, err := oc.AsAdmin().Run("get").Args("authentication", "cluster", "-o=jsonpath={.spec.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			e2e.Logf("Restoring the apiserver cluster's configure")
			apiReStoreYaml := ""
			if originApiunsupportedConfigOverrides == "<nil>" {
				apiReStoreYaml = `[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": null}]`
			} else {
				apiReStoreYaml = fmt.Sprintf(`[{"op": "replace", "path": "/spec/unsupportedConfigOverrides", "value": %v}]`, originApiunsupportedConfigOverrides)
			}
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("kubeapiserver", "cluster", "--type=json", "-p", apiReStoreYaml).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			authRestore := `{"spec":{"oauthMetadata":{"name":""},"type":""}}`
			if len(originoauthMetadataName) != 0 && len(originoauthType) != 0 {
				authRestore = fmt.Sprintf(`{"spec":{"oauthMetadata":{"name":"%s"},"type":"%s"}}`, originoauthMetadataName, originoauthType)
			} else if len(originoauthMetadataName) != 0 && len(originoauthType) == 0 {
				authRestore = fmt.Sprintf(`{"spec":{"oauthMetadata":{"name":"%s"},"type":""}}`, originoauthMetadataName)
			} else if len(originoauthMetadataName) == 0 && len(originoauthType) != 0 {
				authRestore = fmt.Sprintf(`{"spec":{"oauthMetadata":{"name":""},"type":"%s"}}`, originoauthType)
			}

			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("authentication", "cluster", "--type=merge", "-p", authRestore).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			e2e.Logf("Checking kube-apiserver operator should be in Progressing in 100 seconds")
			expectedStatus := map[string]string{"Progressing": "True"}
			err = waitCoBecomes(oc, "kube-apiserver", 120, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 120 seconds")
			e2e.Logf("Checking kube-apiserver operator should be restored in 1800 seconds")
			expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
			err = waitCoBecomes(oc, "kube-apiserver", 1800, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not restored in 1800 seconds")
			err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
			exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 240 seconds")

		}()
		authenticationErr := oc.AsAdmin().Run("patch").Args("authentication", "cluster", "-p", "{\"spec\":{\"oauthMetadata\":{\"name\":\"oauth-meta-68647\"},\"type\":\"None\"}}", "--type=merge").Execute()
		o.Expect(authenticationErr).NotTo(o.HaveOccurred())
		caBundle := "/etc/kubernetes/static-pod-certs/configmaps/trusted-ca-bundle/ca-bundle.crt"
		apiserverPatch := fmt.Sprintf(`{"spec":{"unsupportedConfigOverrides":{"apiServerArguments":{"oidc-ca-file":["%s"],"oidc-client-id":["admin-cli"], "oidc-issuer-url":["%s"]}}}}`, caBundle, kc_URL)
		apiserverErr := oc.AsAdmin().Run("patch").Args("kubeapiserver", "cluster", "-p", apiserverPatch, "--type=merge").Execute()
		o.Expect(apiserverErr).NotTo(o.HaveOccurred())

		e2e.Logf("Checking kube-apiserver operator should be in Progressing in 120 seconds")
		expectedStatus := map[string]string{"Progressing": "True"}
		err = waitCoBecomes(oc, "kube-apiserver", 120, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 120 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1800 seconds")
		expectedStatus = map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
		err = waitCoBecomes(oc, "kube-apiserver", 1800, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1800 seconds")
		err = waitCoBecomes(oc, "authentication", 240, expectedStatus)
		exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 240 seconds")
		err = oc.AsAdmin().Run("whoami").Args("").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create new project to make sure that openshift-apiserver still functions well.")
		projectName := "ocp-68647" + exutil.GetRandomString()
		err = oc.AsAdmin().WithoutNamespace().Run("new-project").Args(projectName, "--skip-config-write").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", projectName).Execute()

		exutil.By("Create new app")
		err = oc.AsAdmin().WithoutNamespace().Run("new-app").Args("quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", projectName, "--import-mode=PreserveOriginal").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Waiting for all pods of hello-openshift application to be ready ...")
		var poderr error
		errPod := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			podOutput, poderr := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", projectName, "--no-headers").Output()
			if poderr == nil && strings.Contains(podOutput, "Running") {
				e2e.Logf("Pod %v succesfully", podOutput)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errPod, fmt.Sprintf("Pod not running :: %v", poderr))
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-High-10136-Project should only watch its owned cache events", func() {
		exutil.By("Create the first namespace")
		oc.SetupProject()
		ns1 := oc.Namespace()
		exutil.By("Create deployment in the first namespace")
		deployCreationErr := oc.WithoutNamespace().Run("create").Args("deployment", "deploy10136-1", "-n", ns1, "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(deployCreationErr).NotTo(o.HaveOccurred())
		if ok := waitForAvailableRsRunning(oc, "deployment", "deploy10136-1", ns1, "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("deploy10136-1 pods are not running as expected")
		}

		exutil.By("Create the second namespace")
		oc.SetupProject()
		ns2 := oc.Namespace()
		exutil.By("Get deployment under the second project with watch")
		cmd2, backgroundBufNs2, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", ns2, "-o", "name", "-w").Background()
		defer cmd2.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Create deployment in the second namespace")
		deployCreationErr2 := oc.WithoutNamespace().Run("create").Args("deployment", "deploy10136-2", "-n", ns2, "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83").Execute()
		o.Expect(deployCreationErr2).NotTo(o.HaveOccurred())
		if ok := waitForAvailableRsRunning(oc, "deployment", "deploy10136-2", ns2, "1"); ok {
			e2e.Logf("All pods are runnnig now\n")
		} else {
			e2e.Failf("deploy10136-2 pods are not running as expected")
		}

		exutil.By("Get deployment in the first namespace with watch")
		cmd1, backgroundBuf, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", ns1, "-o", "name", "-w").Background()
		defer cmd1.Process.Kill()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Delete the deployment in the second namespace")
		deleteDeploymentErr := oc.WithoutNamespace().Run("delete").Args("deployment", "deploy10136-2", "-n", ns2).Execute()
		o.Expect(deleteDeploymentErr).NotTo(o.HaveOccurred())

		exutil.By("Get deployment in the first namespace again")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", ns1, "-o", "name").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Make sure the watch events matched")
		deploymentWatchOut := strings.Replace(backgroundBuf.String(), "\n", "", -1)
		if matched, _ := regexp.MatchString(deploymentWatchOut, out); matched {
			e2e.Logf("All deployment events matched\n")
		} else {
			e2e.Failf("Deployment events not matched")
		}

		exutil.By("Make sure no trace under the second project for the resource under the first project")
		if matched, _ := regexp.MatchString(backgroundBufNs2.String(), "deploy10136-1"); matched {
			e2e.Failf("Should not see any trace for the resource under the first project in the second project\n")
		}
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-High-71178-Make sure no mismatch for sha256sum of openshift install for mac version", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}

		extractTmpDirName := "/tmp/d71178"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		secretFile, secretErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/pull-secret", "-n", "openshift-config", `--template={{index .data ".dockerconfigjson" | base64decode}}`).OutputToFile("auth.dockerconfigjson")
		o.Expect(secretErr).NotTo(o.HaveOccurred())
		exutil.By("Get the payload")
		payloadPullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(payloadPullSpec).NotTo(o.BeEmpty())
		e2e.Logf("pullspec is %v", payloadPullSpec)

		exutil.By("Extract the darwin tools")
		os.RemoveAll("/tmp/d71178")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", payloadPullSpec, "--registry-config="+secretFile, "--command-os=darwin/arm64", "--tools", "--to=/tmp/d71178", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Make sure no mismatch for sha256sum")
		files := getSpecificFileName("/tmp/d71178", "openshift-install")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("%v", files)
		fileSum, err := sha256File("/tmp/d71178/" + files[0])
		e2e.Logf("%v", fileSum)
		o.Expect(err).NotTo(o.HaveOccurred())
		fileSumFromResult := getSha256SumFromFile("/tmp/d71178/sha256sum.txt")
		e2e.Logf("%v", fileSumFromResult)
		if match, _ := regexp.MatchString(fileSum, fileSumFromResult); !match {
			e2e.Failf("File sum not matched")
		}
	})

	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-ConnectedOnly-Author:yinzhou-Medium-71273-Medium-71275-Validate user is able to extract rhel8 and rhel9 oc from the ocp payload", func() {
		if !assertPullSecret(oc) {
			g.Skip("The cluster does not have pull secret for public registry hence skipping...")
		}
		extractTmpDirName := "/tmp/case71273"
		defer os.RemoveAll(extractTmpDirName)
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Get desired image from ocp cluster")
		pullSpec, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o", "jsonpath={..desired.image}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pullSpec).NotTo(o.BeEmpty())

		exutil.By("Extract oc.rhel8 from ocp payload")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc.rhel8", pullSpec, "-a", extractTmpDirName+"/.dockerconfigjson", "--to", extractTmpDirName, "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if _, statErr := os.Stat(extractTmpDirName + "/oc"); os.IsNotExist(statErr) {
			e2e.Failf("Get extracted oc failed")
		}
		removeErr := os.Remove(extractTmpDirName + "/oc")
		o.Expect(removeErr).NotTo(o.HaveOccurred())

		exutil.By("Extract oc.rhel9 from ocp payload")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc.rhel9", pullSpec, "-a", extractTmpDirName+"/.dockerconfigjson", "--to", extractTmpDirName, "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if _, statErr := os.Stat(extractTmpDirName + "/oc"); os.IsNotExist(statErr) {
			e2e.Failf("Get extracted oc failed")
		}
		removeErr = os.Remove(extractTmpDirName + "/oc")
		o.Expect(removeErr).NotTo(o.HaveOccurred())

		exutil.By("Extract oc from ocp payload")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", pullSpec, "-a", extractTmpDirName+"/.dockerconfigjson", "--to", extractTmpDirName, "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if _, statErr := os.Stat(extractTmpDirName + "/oc"); os.IsNotExist(statErr) {
			e2e.Failf("Get extracted oc failed")
		}
		removeErr = os.Remove(extractTmpDirName + "/oc")
		o.Expect(removeErr).NotTo(o.HaveOccurred())

		exutil.By("Get the oc-mirror image from ocp payload")
		ocMirrorImage, _, err := oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "info", pullSpec, "-a", extractTmpDirName+"/.dockerconfigjson", "--insecure", `-ojsonpath={.references.spec.tags[?(@.name=="oc-mirror")].from.name}`).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Extract oc-mirror.rhel8")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("extract", ocMirrorImage, "-a", extractTmpDirName+"/.dockerconfigjson", "--path=/usr/bin/oc-mirror.rhel8:"+extractTmpDirName, "--confirm", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if _, statErr := os.Stat(extractTmpDirName + "/oc-mirror.rhel8"); os.IsNotExist(statErr) {
			e2e.Failf("Get extracted oc-mirror.rhel8 failed")
		}
		removeErr = os.Remove(extractTmpDirName + "/oc-mirror.rhel8")
		o.Expect(removeErr).NotTo(o.HaveOccurred())

		exutil.By("Extract oc-mirror.rhel9")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("image").Args("extract", ocMirrorImage, "-a", extractTmpDirName+"/.dockerconfigjson", "--path=/usr/bin/oc-mirror.rhel9:"+extractTmpDirName, "--confirm", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if _, statErr := os.Stat(extractTmpDirName + "/oc-mirror.rhel9"); os.IsNotExist(statErr) {
			e2e.Failf("Get extracted oc-mirror.rhel9 failed")
		}
		removeErr = os.Remove(extractTmpDirName + "/oc-mirror.rhel9")
		o.Expect(removeErr).NotTo(o.HaveOccurred())
	})
	// author: yinzhou@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-72217-Should get warning when there is an identical short name for two or more resources", func() {
		customResourceBaseDir := exutil.FixturePath("testdata", "workloads/case72217")
		cronTabCRDF := filepath.Join(customResourceBaseDir, "crd-crontab-72217.yaml")
		cronCRF := filepath.Join(customResourceBaseDir, "cr-cron-72217.yaml")
		customTaskCRDF := filepath.Join(customResourceBaseDir, "crd-customtask-72217.yaml")
		customCRF := filepath.Join(customResourceBaseDir, "cr-custom-72217.yaml")
		catToyCRDF := filepath.Join(customResourceBaseDir, "crd-cattoy-72217.yaml")
		catCRF := filepath.Join(customResourceBaseDir, "cr-cat-72217.yaml")

		g.By("Create new namespace")
		oc.SetupProject()
		ns72217 := oc.Namespace()

		exutil.By("Create the first CRD and get by short name should no warning")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", cronTabCRDF).Execute()
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", cronTabCRDF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCRDAvailable(oc, "crontabs72217.stable.example.com")
		exutil.AssertWaitPollNoErr(err, "The crd crontabs72217.stable.example.com is not available in 60 seconds")
		err = waitCreateCr(oc, cronCRF, ns72217)
		exutil.AssertWaitPollNoErr(err, "The cr of  crontabs is not created in 120 seconds")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ct72217", "-n", ns72217).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Create the second CRD and get by short name should see warning")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", customTaskCRDF).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", customTaskCRDF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCRDAvailable(oc, "customtasks72217.example.com")
		exutil.AssertWaitPollNoErr(err, "The crd customtasks72217.example.com is not available in 60 seconds")
		err = waitCreateCr(oc, customCRF, ns72217)
		exutil.AssertWaitPollNoErr(err, "The cr of custometask is not created in 120 seconds")
		_, outputWarning, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ct72217", "-n", ns72217).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputWarning).To(o.ContainSubstring("could also match lower priority resource"))

		exutil.By("Create the third CRD and get by short name should see warning")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", catToyCRDF).Execute()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", catToyCRDF).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = waitCRDAvailable(oc, "cattoys72217.bate.example.com")
		exutil.AssertWaitPollNoErr(err, "The crd cattoys72217.bate.example.com is not available in 60 seconds")
		err = waitCreateCr(oc, catCRF, ns72217)
		exutil.AssertWaitPollNoErr(err, "The cr of cattoy is not created in 120 seconds")
		_, outputWarning, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ct72217", "-n", ns72217).Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputWarning).To(o.ContainSubstring("could also match lower priority resource customtasks72217.example.com"))
		o.Expect(outputWarning).To(o.ContainSubstring("could also match lower priority resource crontabs72217.stable.example.com"))
	})

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-High-75997-Make sure images with different tag but same layers could be mirrored correctly", func() {
		customResourceBaseDir := exutil.FixturePath("testdata", "workloads")
		imageMirrorList := filepath.Join(customResourceBaseDir, "config-images-75997.txt")

		exutil.By("Create new namespace")
		oc.SetupProject()
		ns75997 := oc.Namespace()

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   ns75997,
		}

		exutil.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		sedCmd := fmt.Sprintf(`sed -i 's/localhost:5000/%s/g' %s`, serInfo.serviceName, imageMirrorList)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		extractTmpDirName := "/tmp/case75997"
		err = os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, 120*time.Second, func() (bool, error) {
			output, err1 := oc.WithoutNamespace().Run("image").Args("mirror", "--insecure", "-a", extractTmpDirName+"/.dockerconfigjson", "-f", imageMirrorList).Output()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			if !strings.Contains(output, "hello-openshift:arm-amd-latest") && !strings.Contains(output, "hello-openshift:arm-amd-1.2.0") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "oc image mirror fails even after waiting for about 120 seconds")
	})
	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-Medium-76150-Make sure oc debug node has set HOST env var", func() {
		mnodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Create new namespace")
		oc.SetupProject()
		project76150 := oc.Namespace()
		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, project76150)
		filePath, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("node/"+mnodeName, "-n", project76150, "-o=yaml").OutputToFile(getRandomString() + "workload-debug.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		regV1 := checkFileContent(filePath, "name: HOST")
		regV2 := checkFileContent(filePath, "value: /host")
		if regV1 && regV2 {
			e2e.Logf("Found the expected host env setting for debug pod")
		} else {
			e2e.Failf("Don't find the host env set for debug pod")
		}
	})

	g.It("Author:yinzhou-ROSA-OSD_CCS-ARO-High-76116-Make sure oc could run on rhel with fips on", func() {
		mnodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.By("Create new namespace")
		oc.SetupProject()
		project76116 := oc.Namespace()
		exutil.By("Set namespace as privileged namespace")
		exutil.SetNamespacePrivileged(oc, project76116)
		exutil.By("Check if fips enable")
		efips, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", project76116, "node/"+mnodeName, "--", "chroot", "/host", "fips-mode-setup", "--check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(efips, "FIPS mode is enabled.") {
			g.Skip("Fips mode is disabled, skip it.")
		}

		exutil.By("Check if oc could run with fips on")
		clientVersion, err := oc.AsAdmin().WithoutNamespace().Run("debug").Args("-n", project76116, "node/"+mnodeName, "--", "chroot", "/host", "oc", "version").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(clientVersion, "Client Version") {
			e2e.Failf("Failed to run oc client with fips on")
		}
	})
})

// ClientVersion ...
type ClientVersion struct {
	BuildDate    string `json:"buildDate"`
	Compiler     string `json:"compiler"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	GitVersion   string `json:"gitVersion"`
	GoVersion    string `json:"goVersion"`
	Major        string `json:"major"`
	Minor        string `json:"minor"`
	Platform     string `json:"platform"`
}

// ServerVersion ...
type ServerVersion struct {
	BuildDate    string `json:"buildDate"`
	Compiler     string `json:"compiler"`
	GitCommit    string `json:"gitCommit"`
	GitTreeState string `json:"gitTreeState"`
	GitVersion   string `json:"gitVersion"`
	GoVersion    string `json:"goVersion"`
	Major        string `json:"major"`
	Minor        string `json:"minor"`
	Platform     string `json:"platform"`
}

// VersionInfo ...
type VersionInfo struct {
	ClientInfo       ClientVersion `json:"ClientVersion"`
	OpenshiftVersion string        `json:"openshiftVersion"`
	ServerInfo       ServerVersion `json:"ServerVersion"`
}
