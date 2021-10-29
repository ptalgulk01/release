package workloads

import (
	"encoding/json"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("oc", exutil.KubeConfigPath())
	)

	g.It("Author:yinzhou-Medium-28007-Checking oc version show clean as gitTreeState value", func() {
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
	g.It("Author:yinzhou-High-43030-oc get events always show the timestamp as LAST SEEN", func() {
		g.By("Get all the namespace")
		output, err := oc.AsAdmin().Run("get").Args("projects", "-o=custom-columns=NAME:.metadata.name", "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
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
	g.It("Author:yinzhou-Medium-42983-always delete the debug pod when the oc debug node command exist", func() {
		g.By("Get all the node name list")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		nodeList := strings.Fields(out)

		g.By("Create a new namespace")
		oc.SetupProject()

		g.By("Run debug node")
		for _, nodeName := range nodeList {
			err = oc.AsAdmin().Run("debug").Args("node/"+nodeName, "--", "chroot", "/host", "date").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Make sure debug pods have been deleted")
		err = wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
			output, err := oc.Run("get").Args("pods", "-n", oc.Namespace()).Output()
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
	g.It("Author:yinzhou-High-43032-oc adm release mirror generating correct imageContentSources when using --to and --to-release-image [Slow]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		podMirrorT := filepath.Join(buildPruningBaseDir, "pod_mirror.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:2",
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
		imageToS := "--to=" + serInfo.serviceUrl + "/zhouytest/test-release"
		imageToReleaseS := "--to-release-image=" + serInfo.serviceUrl + "/zhouytest/ocptest-release:4.5.8-x86_64"
		imagePullSecretS := "-a " + "/etc/foo/" + ".dockerconfigjson"

		pod43032 := podMirror{
			name:            "mypod43032",
			namespace:       oc.Namespace(),
			cliImageId:      cliImage,
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
		err := wait.Poll(5*time.Second, 600*time.Second, func() (bool, error) {
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
        g.It("Author:yinzhou-High-44797-Could define a Command for DC", func() {
		g.By("create new namespace")
		oc.SetupProject()

		g.By("Create the dc with define command")
		err := oc.WithoutNamespace().Run("create").Args("deploymentconfig","-n", oc.Namespace(), "dc44797", "--image="+"quay.io/openshifttest/busybox@sha256:afe605d272837ce1732f390966166c2afff5391208ddd57de10942748694049d", "--", "tail", "-f", "/dev/null").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the command should be defined")
		comm, err := oc.Run("get").WithoutNamespace().Args("dc/dc44797","-n", oc.Namespace(), "-o=jsonpath={.spec.template.spec.containers[0].command[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.ExpectEqual("tail", comm)

		g.By("Create the deploy with define command")
		err = oc.WithoutNamespace().Run("create").Args("deployment","-n", oc.Namespace(), "deploy44797", "--image="+"quay.io/openshifttest/busybox@sha256:afe605d272837ce1732f390966166c2afff5391208ddd57de10942748694049d", "--", "tail", "-f", "/dev/null").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the command should be defined")
		comm1, err := oc.Run("get").WithoutNamespace().Args("deploy/deploy44797","-n", oc.Namespace(), "-o=jsonpath={.spec.template.spec.containers[0].command[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.ExpectEqual("tail", comm1)

	})

	// author: yinzhou@redhat.com
	g.It("Author:yinzhou-High-43034-should not show signature verify error msgs while trying to mirror OCP image repository to", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		podMirrorT := filepath.Join(buildPruningBaseDir, "pod_mirror.yaml")
		g.By("create new namespace")
		oc.SetupProject()

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry:2",
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
		imageToS := "--to=" + serInfo.serviceUrl + "/zhouytest/test-release"
		imageToReleaseS := "--apply-release-image-signature"
		imagePullSecretS := "-a " + "/etc/foo/" + ".dockerconfigjson"

		pod43034 := podMirror{
			name:            "mypod43034",
			namespace:       oc.Namespace(),
			cliImageId:      cliImage,
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
		err := wait.Poll(5*time.Second, 600*time.Second, func() (bool, error) {
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
})

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

type VersionInfo struct {
	ClientInfo       ClientVersion `json:"ClientVersion"`
	OpenshiftVersion string        `json:"openshiftVersion"`
	ServerInfo       ServerVersion `json:"ServerVersion"`
}
