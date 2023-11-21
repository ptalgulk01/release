package workloads

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLIWithoutNamespace("default")
	)

	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-10618-Prune old builds by admin command [Serial]", func() {
		if checkOpenshiftSamples(oc) {
			g.Skip("Can't find the cluster operator openshift-samples, skip it.")
		}

		// Skip the test if baselinecaps is set to v4.13 or v4.14
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}
		if !checkMustgatherImagestreamTag(oc) {
			g.Skip("Skipping the test as can't find the imagestreamtag for must-gather")
		}

		g.By("create new namespace")
		oc.SetupProject()
		ns10618 := oc.Namespace()

		g.By("create the build")
		err := oc.WithoutNamespace().Run("new-build").Args("-D", "FROM must-gather", "-n", ns10618).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		for i := 0; i < 4; i++ {
			err := oc.Run("start-build").Args("bc/must-gather", "-n", ns10618).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			time.Sleep(30 * time.Second)
		}
		out, err := oc.AsAdmin().Run("adm").Args("prune", "builds", "-h").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "Prune old completed and failed builds")).To(o.BeTrue())

		for j := 1; j < 6; j++ {
			checkBuildStatus(oc, "must-gather-"+strconv.Itoa(j), ns10618, "Complete")
		}

		keepCompletedRsNum := 2
		expectedPrunebuildcmdDryRun := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1  |grep %s |awk '{print $2}'", keepCompletedRsNum, ns10618)
		pruneBuildCMD := fmt.Sprintf("oc adm prune builds --keep-complete=%v --keep-younger-than=1s --keep-failed=1 --confirm  |grep %s|awk '{print $2}'", keepCompletedRsNum, ns10618)

		g.By("Get the expected prune build list from dry run")
		buildbeforedryrun, err := oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildListPre := strings.Fields(buildbeforedryrun)
		e2e.Logf("the remain build list is %v", buildListPre)
		expectedPruneRsName := getPruneResourceName(expectedPrunebuildcmdDryRun)

		g.By("Get the pruned build list")
		buildbeforeprune, err := oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildListPre2 := strings.Fields(buildbeforeprune)
		e2e.Logf("the remain build list is %v", buildListPre2)
		prunedBuildName := getPruneResourceName(pruneBuildCMD)
		if comparePrunedRS(expectedPruneRsName, prunedBuildName) {
			e2e.Logf("Checked the pruned resources is expected")
		} else {
			e2e.Failf("Pruned the wrong build")
		}

		g.By("Get the remain build and completed should <=2")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Complete\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		buildNameList := strings.Fields(out)
		e2e.Logf("the remain build list is %v", buildNameList)
		e2e.Logf("the remain build list len is %v", len(buildNameList))
		o.Expect(len(buildNameList) < 3).To(o.BeTrue())

		g.By("Get the remain build and failed should <=1")
		out, err = oc.Run("get").Args("build", "-n", ns10618, "-o=jsonpath={.items[?(@.status.phase == \"Failed\")].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		failedBuildNameList := strings.Fields(out)
		o.Expect(len(failedBuildNameList) < 2).To(o.BeTrue())

		err = oc.Run("delete").Args("bc", "must-gather", "-n", ns10618, "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("adm").Args("prune", "builds", "--keep-younger-than=1s", "--keep-complete=2", "--keep-failed=1", "--confirm", "--orphans").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.Run("get").Args("build", "-n", ns10618).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(out, "No resources found")).To(o.BeTrue())
	})

	g.It("ARO-Author:yinzhou-Medium-62956-oc adm node-logs works for nodes logs api", func() {
		windowNodeList, err := exutil.GetAllNodesbyOSType(oc, "windows")
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(windowNodeList) < 1 {
			e2e.Logf("No windows nodes support to test output")
			_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet", "-o", "short").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			e2e.Logf("With windows nodes not support to test output")
			_, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--unit=kubelet", "-g", "crio").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--since=-5m", "--until=-1m").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--tail", "10").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		now := time.Now()
		sinceT := now.Add(time.Minute * -2).Format("2006-01-02 15:04:05")
		untilT := now.Add(time.Minute * -10).Format("2006-01-02 15:04:05")
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--since", sinceT).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "worker", "--until", untilT).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("ROSA-OSD_CCS-ARO-Author:yinzhou-Medium-11112-Prune old deploymentconfig by admin command", func() {
		if isBaselineCapsSet(oc, "None") || isBaselineCapsSet(oc, "v4.13") || isBaselineCapsSet(oc, "v4.12") {
			g.Skip("Skipping the test as baselinecaps have been set to None and some of API capabilities are not enabled!")
		}

		g.By("Create new namespace")
		oc.SetupProject()
		ns11112 := oc.Namespace()

		err := oc.WithoutNamespace().Run("create").Args("deploymentconfig", "mydc11112", "--image", "quay.io/openshifttest/hello-openshift@sha256:4200f438cf2e9446f6bcff9d67ceea1f69ed07a2f83363b7fb52529f7ddd8a83", "-n", ns11112).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		assertSpecifiedPodStatus(oc, "mydc11112-1-deploy", ns11112, "Succeeded")

		g.By("Trigger more deployment and wait for succeed")
		for i := 0; i < 3; i++ {
			err := oc.Run("rollout").Args("latest", "dc/mydc11112", "-n", ns11112).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			assertSpecifiedPodStatus(oc, "mydc11112-"+strconv.Itoa(i+2)+"-deploy", ns11112, "Succeeded")
		}

		g.By("Add pre hook to make sure new deployment failed")
		err = oc.Run("set").Args("deployment-hook", "dc/mydc11112", "--pre", "-c=default-container", "--failure-policy=abort", "--", "/bin/false").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		for i := 0; i < 2; i++ {
			err := oc.Run("rollout").Args("latest", "dc/mydc11112", "-n", ns11112).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			assertSpecifiedPodStatus(oc, "mydc11112-"+strconv.Itoa(i+5)+"-deploy", ns11112, "Failed")
		}

		g.By("Dry run prune the DC and wait for pruned DC is expected")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			output, warning, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-complete=2", "--keep-failed=1", "--keep-younger-than=1m").Outputs()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The warning %v", warning)
			if strings.Contains(output, "mydc11112-1") && strings.Contains(output, "mydc11112-5") {
				e2e.Logf("Found the expected prune output %v", output)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "timeout wait for prune deploymentconfig dry run")
		rcOutput, err := oc.Run("get").Args("rc", "-n", ns11112).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(rcOutput, "mydc11112-1")).To(o.BeTrue())
		o.Expect(strings.Contains(rcOutput, "mydc11112-5")).To(o.BeTrue())

		g.By("Prune the DC and check the result is only prune the first completed and first failed DC")
		output, _, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-complete=2", "--keep-failed=1", "--keep-younger-than=1m", "--confirm").Outputs()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "mydc11112-1")).To(o.BeTrue())
		o.Expect(strings.Contains(output, "mydc11112-5")).To(o.BeTrue())

		err = oc.Run("delete").Args("dc/mydc11112", "-n", ns11112, "--cascade=orphan").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Prune the DC with orphans and make sure all the non-running DC are all pruned")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("prune", "deployments", "--keep-younger-than=1m", "--confirm", "--orphans").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		waitForResourceDisappear(oc, "rc", "mydc11112-2", ns11112)
		waitForResourceDisappear(oc, "rc", "mydc11112-3", ns11112)
		waitForResourceDisappear(oc, "rc", "mydc11112-6", ns11112)
		rcOutput, err = oc.Run("get").Args("rc", "-n", ns11112).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(rcOutput, "mydc11112-4")).To(o.BeTrue())
	})
	g.It("ROSA-OSD_CCS-ARO-NonPreRelease-Author:yinzhou-Medium-68242-oc adm release mirror works fine with multi-arch image to image stream", func() {
		extractTmpDirName := "/tmp/case68242"
		err := os.MkdirAll(extractTmpDirName, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(extractTmpDirName)
		g.By("Create new namespace")
		oc.SetupProject()
		ns68242 := oc.Namespace()

		_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", fmt.Sprintf("--to=%s", extractTmpDirName), "--confirm").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		payloadImage := getLatestPayload("https://multi.ocp.releases.ci.openshift.org/api/v1/releasestream/4-stable-multi/latest")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("adm").Args("release", "mirror", "-a", extractTmpDirName+"/.dockerconfigjson", "--from="+payloadImage, "--to-image-stream=release", "--keep-manifest-list=true", "-n", ns68242).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		imageMediaType, err := oc.Run("get").Args("istag", "release:installer", "-n", ns68242, "-o=jsonpath={.image.dockerImageManifestMediaType}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The output %v", imageMediaType)
		o.Expect(strings.Contains(imageMediaType, "application/vnd.docker.distribution.manifest.list.v2+json")).To(o.BeTrue())
	})
})
