package workloads

import (
	"context"
	"fmt"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
)

var _ = g.Describe("[sig-cli] Workloads", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("ocmirror", exutil.KubeConfigPath())
	)
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-46517-List operator content with different options", func() {
		dirname := "/tmp/case46517"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		dockerCreFile, homePath, err := locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			os.RemoveAll(dockerCreFile)
			_, err = os.Stat(homePath + "/.docker/config.json.back")
			if err == nil {
				copyFile(homePath+"/.docker/config.json.back", homePath+"/.docker/config.json")
			}
		}()

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage := []string{
			"registry.redhat.io/redhat/redhat-operator-index:v4.11",
			"registry.redhat.io/redhat/certified-operator-index:v4.11",
			"registry.redhat.io/redhat/community-operator-index:v4.11",
			"registry.redhat.io/redhat/redhat-marketplace-index:v4.11",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		out, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--version=4.11", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		checkMessage = []string{
			"3scale-operator",
			"amq-online",
			"amq-streams",
			"amq7-interconnect-operator",
			"ansible-automation-platform-operator",
			"ansible-cloud-addons-operator",
			"apicast-operator",
			"businessautomation-operator",
			"cincinnati-operator",
			"cluster-logging",
			"compliance-operator",
			"container-security-operator",
			"costmanagement-metrics-operator",
			"cryostat-operator",
			"datagrid",
			"devworkspace-operator",
			"eap",
			"elasticsearch-operator",
			"external-dns-operator",
			"file-integrity-operator",
			"fuse-apicurito",
			"fuse-console",
			"fuse-online",
			"gatekeeper-operator-product",
			"jaeger-product",
			"jws-operator",
			"kiali-ossm",
			"kubevirt-hyperconverged",
			"mcg-operator",
			"mtc-operator",
			"mtv-operator",
			"node-healthcheck-operator",
			"node-maintenance-operator",
			"ocs-operator",
			"odf-csi-addons-operator",
			"odf-lvm-operator",
			"odf-multicluster-orchestrator",
			"odf-operator",
			"odr-cluster-operator",
			"odr-hub-operator",
			"openshift-cert-manager-operator",
			"openshift-gitops-operator",
			"openshift-pipelines-operator-rh",
			"openshift-secondary-scheduler-operator",
			"opentelemetry-product",
			"quay-bridge-operator",
			"quay-operator",
			"red-hat-camel-k",
			"redhat-oadp-operator",
			"rh-service-binding-operator",
			"rhacs-operator",
			"rhpam-kogito-operator",
			"rhsso-operator",
			"sandboxed-containers-operator",
			"serverless-operator",
			"service-registry-operator",
			"servicemeshoperator",
			"skupper-operator",
			"submariner",
			"web-terminal",
		}

		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging", "--channel=stable").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "operators", "--catalog=registry.redhat.io/redhat/redhat-operator-index:v4.11", "--package=cluster-logging").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

	})
	g.It("ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-46818-Low-46523-check the User Agent for oc-mirror", func() {
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "catlog-loggings.yaml")

		dirname := "/tmp/case46523"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("/tmp/case46523/oc-mirror-workspace")
		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/case46523", "-v", "7", "--dry-run").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		//check user-agent and dry-run should write mapping file
		checkMessage := []string{
			"User-Agent: oc-mirror",
			"Writing image mapping",
		}
		for _, v := range checkMessage {
			o.Expect(out).To(o.ContainSubstring(v))
		}
		_, err = os.Stat("/tmp/case46523/oc-mirror-workspace/mapping.txt")
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-Medium-46770-Low-46520-Local backend support for oc-mirror", func() {
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "ocmirror-localbackend.yaml")

		dirname := "/tmp/46770test"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(60*time.Second, 300*time.Second, func() (bool, error) {

			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46770test", "--continue-on-error", "-v", "3").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if !strings.Contains(out, "Using local backend at location") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but the oc-mirror still failed"))

		_, err = os.Stat("/tmp/46770test/publish/.metadata.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("describe", "/tmp/46770test/mirror_seq1_000000.tar").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46506-High-46817-Mirror a single image works well [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorS := filepath.Join(ocmirrorBaseDir, "config_singleimage.yaml")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Mirror to registry")
		err := wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if strings.Contains(out, "using stateless mode") {
				return true, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can't see the stateless mode log with %s", err))
		g.By("Mirror to localhost")
		dirname := "/tmp/46506test"
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", operatorS, "file:///tmp/46506test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
		_, err = os.Stat("/tmp/46506test/mirror_seq1_000000.tar")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirror to registry from archive")
		defer os.RemoveAll("oc-mirror-workspace")
		out2, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "/tmp/46506test/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName+"/mirrorachive", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out2, "using stateless mode") {
			e2e.Failf("Can't see the stateless mode log")
		}
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-Low-51093-oc-mirror init", func() {
		if !assertPullSecret(oc) {
			g.Skip("the cluster do not has all pull-secret for public registry")
		}
		g.By("Set podman registry config")
		dirname := "/tmp/case51093"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		out, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out, "local") {
			e2e.Failf("Can't find the storageconfig of local")
		}
		out1, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init", "--registry", "localhost:5000/test:latest").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(out1, "registry") {
			e2e.Failf("Can't find the storageconfig of registry")
		}
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("init", "--registry", "localhost:5000/test:latest", "--output", "json").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-High-46769-Critical-46515-High-registry backend test [Serial]", func() {
		architecture.SkipArchitectures(oc, architecture.MULTI)
		g.By("Set podman registry config")
		dirname := "/tmp/case46769"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_operator_helm.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, operatorConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Mirroring selected operator and helm image")
		defer os.RemoveAll("oc-mirror-workspace")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--continue-on-error").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "oc-mirror command still falied")
	})
	g.It("NonHyperShiftHOST-Author:yinzhou-NonPreRelease-Longduration-Medium-37372-High-40322-oc adm release extract pull from localregistry when given a localregistry image [Disruptive]", func() {
		var imageDigest string
		g.By("Set podman registry config")
		dirname := "/tmp/case37372"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ocpPlatformConfigS := filepath.Join(ocmirrorBaseDir, "registry_backend_ocp_latest.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, ocpPlatformConfigS)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer removeOcMirrorLog()
		g.By("Create the mapping file by oc-mirror dry-run command")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Checkpoint for 40322, mirror with mapping")
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "-f", "oc-mirror-workspace/mapping.txt", "--max-per-registry", "1", "--skip-multiple-scopes=true", "--insecure").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check for the mirrored image and get the image digest")
		imageDigest = getDigestFromImageInfo(oc, serInfo.serviceName)

		g.By("Run oc-mirror to create ICSP file")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ocpPlatformConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 37372")
		g.By("Remove the podman Cred")
		os.RemoveAll(dirname)
		g.By("Try to extract without icsp file, will failed")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", serInfo.serviceName+"/openshift/release-images"+imageDigest, "--insecure").Execute()
		o.Expect(err).Should(o.HaveOccurred())
		g.By("Try to extract with icsp file, will extract from localregisty")
		imageContentSourcePolicy := findImageContentSourcePolicy()
		waitErr := wait.Poll(120*time.Second, 600*time.Second, func() (bool, error) {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "extract", "--command=oc", "--to=oc-mirror-workspace/", "--icsp-file="+imageContentSourcePolicy, serInfo.serviceName+"/openshift/release-images"+"@"+imageDigest, "--insecure").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the mirror still failed"))
	})
	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Longduration-Medium-46518-List ocp release content with different options", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case46518"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("List releases for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List release channels for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channels").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("List available releases from channel candidate for ocp 4.11 and specify arch arm64")
		err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("list", "releases", "--version=4.11", "--channel=candidate-4.11", "--filter-by-archs=arm64").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Author:yinzhou-NonPreRelease-Longduration-Medium-60594-ImageSetConfig containing OCI FBC and release platform and additionalImages works well with --include-local-oci-catalogs flag [Serial]", func() {
		err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("version").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry config")
		dirname := "/tmp/case60594"
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFullConfig := filepath.Join(ocmirrorBaseDir, "config-oci-all.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFullConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Longduration-Author:yinzhou-NonPreRelease-Medium-60597-Critical-60595-oc-mirror support for TargetCatalog field for operator[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60597"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)

		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads/config-60597")
		normalTargetConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal-target.yaml")
		ociTargetTagConfig := filepath.Join(ocmirrorBaseDir, "config-60597-oci-target-tag.yaml")
		normalConfig := filepath.Join(ocmirrorBaseDir, "config-60597-normal.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalTargetConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString(serInfo.serviceName+"/abc/redhat-operator-index:v4.13", output); matched {
				return true, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can't find the expect target catalog %s", err))
		output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("/mno/redhat-operator-index:v5", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociTargetTagConfig, "docker://"+serInfo.serviceName+"/ocit", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("/ocit/mno/redhat-operator-index:v5", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(serInfo.serviceName+"/redhat/redhat-operator-index:v4.13", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", normalConfig, "docker://"+serInfo.serviceName+"/testname", "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(serInfo.serviceName+"/testname/redhat/redhat-operator-index:v4.13", output); !matched {
			e2e.Failf("Can't find the expect target catalog\n")
		}
		g.By("Checkpoint for 60595")
		ocmirrorDir := exutil.FixturePath("testdata", "workloads")
		ociFirstConfig := filepath.Join(ocmirrorDir, "config-oci-f.yaml")
		ociSecondConfig := filepath.Join(ocmirrorDir, "config-oci-s.yaml")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFirstConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociSecondConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", output); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-Longduration-Author:yinzhou-NonPreRelease-Medium-60607-oc mirror purne for mirror2disk and mirror2mirror with and without skip-pruning[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60607"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads/config-60603")
		configFirst := filepath.Join(buildPruningBaseDir, "config-normal-first.yaml")
		configSecond := filepath.Join(buildPruningBaseDir, "config-normal-second.yaml")
		configThird := filepath.Join(buildPruningBaseDir, "config-normal-third.yaml")

		fileList := []string{configFirst, configSecond, configThird}
		for _, file := range fileList {
			sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, file)
			_, err = exec.Command("bash", "-c", sedCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")

		defer os.RemoveAll("mirror_seq1_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configFirst, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		g.By("Check the tag for mirrored image")
		checkCmd := fmt.Sprintf(`curl -k 'https://%s/v2/openshift4/ose-cluster-kube-descheduler-operator/tags/list'`, serInfo.serviceName)
		output, err := exec.Command("bash", "-c", checkCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq2_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configSecond, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq2_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); !matched {
			e2e.Failf("Can't find the prune log\n")
		}
		g.By("Check the tag again, should be null")
		outputNew, err := exec.Command("bash", "-c", checkCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(outputNew).To(o.ContainSubstring("null"))
		defer os.RemoveAll("mirror_seq3_000000.tar")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", configThird, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		outputMirror, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq3_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls", "--skip-pruning").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString("Deleting manifest", outputMirror); matched {
			e2e.Failf("Should not find the prune log\n")
		}
	})

	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-60611-Medium-62694-oc mirror for oci fbc catalogs should work fine with registries.conf[Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60611"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, _, err = locateDockerCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the skopeo copy still failed")

		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}
		g.By("Trying to launch the first registry app")
		serInfo := registry.createregistry(oc)
		g.By("Trying to launch the second registry app")
		secondSerInfo := registry.createregistrySpecifyName(oc, "secondregistry")
		g.By("Prepare test data to first registry")
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads/case60611")
		ociConfig := filepath.Join(ocmirrorBaseDir, "config.yaml")
		registryConfig := filepath.Join(ocmirrorBaseDir, "registry.conf")
		digestConfig := filepath.Join(ocmirrorBaseDir, "config-62694.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, serInfo.serviceName, registryConfig)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, 900*time.Second, func() (bool, error) {
			_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociConfig, "docker://"+serInfo.serviceName, "--dest-skip-tls", "--dry-run").Output()
			if err != nil {
				e2e.Logf("the err:%v, and try next round", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Image mirror failed with error %s", err))
		_, err = oc.WithoutNamespace().Run("image").Args("mirror", "-f", "oc-mirror-workspace/mapping.txt", "--insecure").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Use oc-mirror with registry.conf")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociConfig, "docker://"+secondSerInfo.serviceName, "--dest-skip-tls", "--oci-registries-config", registryConfig).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Make sure data forword from local registry")
		logOut, err := oc.Run("logs").Args("deploy/registry", "--tail=50").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(logOut, "http.request.method=GET")).To(o.BeTrue())

		g.By("Checkpoint for 62694")
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", digestConfig, "file://").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "mirror_seq1_000000.tar", "docker://"+serInfo.serviceName, "--dest-skip-tls").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Medium-60601-Medium-60602-oc mirror support to filter operator by channels on oci fbc catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case60601"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "-o=jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)
		if registry == "" || strings.Contains(registry, "brew.registry.redhat.io") {
			g.Skip("There is no public registry, skip.")
		}

		publicRegistry, _, _ := strings.Cut(registry, "/")

		g.By("Copy the registry as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-filter.yaml")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, publicRegistry, ociFilterConfig)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "docker://"+publicRegistry, "--dest-skip-tls", "--ignore-history").Execute()
			if err != nil {
				e2e.Logf("mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checkpoint for 60602")
		defer removeCSAndISCP(oc)
		createCSAndISCP(oc, "cs-case60601-redhat-operator-index", "openshift-marketplace", "Running", 1)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Hign-65149-mirror2disk and disk2mirror workflow for local oci catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case65149"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "-o=jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)
		if registry == "" || strings.Contains(registry, "brew.registry.redhat.io") {
			g.Skip("There is no public registry, skip.")
		}

		publicRegistry, _, _ := strings.Cut(registry, "/")

		g.By("Copy the catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/oci-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65149.yaml")
		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Starting mirror2disk ....")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "file://"+dirname).Execute()
			if err != nil {
				e2e.Logf("mirror to disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		g.By("Starting disk2mirror  ....")
		mirrorErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+publicRegistry, "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("disk to registry failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(mirrorErr, "max time reached but the disk to registry still failed")

		defer removeCSAndISCP(oc)
		createCSAndISCP(oc, "cs-test", "openshift-marketplace", "Running", 2)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Critical-65150-mirror2disk and disk2mirror workflow for local multi oci catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case65150"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "-o=jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)
		if registry == "" || strings.Contains(registry, "brew.registry.redhat.io") {
			g.Skip("There is no public registry, skip.")
		}

		publicRegistry, _, _ := strings.Cut(registry, "/")

		g.By("Copy the multi-arch catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all --format v2s2 docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/oci-multi-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))

		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65150.yaml")
		g.By("update the operator mirror config file")
		sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, publicRegistry, ociFilterConfig)
		e2e.Logf("Check sed cmd %s description:", sedCmd)
		_, err = exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Starting mirror2disk ....")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "file://"+dirname, "--ignore-history").Execute()
			if err != nil {
				e2e.Logf("mirror to disk failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")
		g.By("Starting disk2mirror  ....")
		mirrorErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+publicRegistry, "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("disk to registry failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(mirrorErr, "max time reached but the disk to registry still failed")
		defer removeCSAndISCP(oc)
		createCSAndISCP(oc, "cs-case65150-oci-multi-index", "openshift-marketplace", "Running", 1)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-High-65151-mirror2disk and disk2mirror workflow for local oci catalog incremental  and prune testing [Serial]", func() {
		g.By("Set registry config")
		homePath := os.Getenv("HOME")
		dirname := homePath + "/case5151"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "-o=jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)
		if registry == "" || strings.Contains(registry, "brew.registry.redhat.io") {
			g.Skip("There is no public registry, skip.")
		}

		publicRegistry, _, _ := strings.Cut(registry, "/")

		defer os.RemoveAll("/tmp/redhat-operator-index")
		g.By("Copy the catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", "/tmp/redhat-operator-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but the skopeo copy still failed"))
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFirstConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65151-1.yaml")
		ociSecondConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65151-2.yaml")
		for _, filename := range []string{ociFirstConfig, ociSecondConfig} {
			sedCmd := fmt.Sprintf(`sed -i 's/registryroute/%s/g' %s`, publicRegistry, filename)
			_, err = exec.Command("bash", "-c", sedCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Start mirror2disk for the first time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFirstConfig, "file://"+dirname).Execute()
			if err != nil {
				e2e.Logf("The first mirror2disk  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
		g.By("Start disk2mirror for the first time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq1_000000.tar", "docker://"+publicRegistry, "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("The first disk2mirror  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the disk2mirror still failed")

		g.By("Start mirror2disk for the second time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociSecondConfig, "file://"+dirname).Execute()
			if err != nil {
				e2e.Logf("The second mirror2disk  failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror2disk still failed")
		g.By("Start disk2mirror for the second time")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			output, err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", dirname+"/mirror_seq2_000000.tar", "docker://"+publicRegistry, "--dest-skip-tls").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if err != nil {
				e2e.Logf("The second disk2mirror  failed, retrying...")
				return false, nil
			}
			if !strings.Contains(output, "Deleting manifest") || strings.Contains(output, "secondary-scheduler-operator") {
				e2e.Failf("Don't find the prune logs and should not see logs about sso for incremental test")
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the second disk2mirror still failed")
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-NonPreRelease-Longduration-Critical-65202-Verify user is able to mirror multi payload via oc-mirror [Serial]", func() {
		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetConfig65202 := filepath.Join(buildPruningBaseDir, "imageSetConfig65202.yaml")

		dirname, err := os.MkdirTemp("", "case65202-*")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		// Replace localhost with retreived registry name from the cluster in imageSetConfigFile
		f, err := os.Open(imageSetConfig65202)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()

		content, err := io.ReadAll(f)
		o.Expect(err).NotTo(o.HaveOccurred())
		yamlData := make(map[string]interface{})
		err = yaml.Unmarshal(content, &yamlData)
		o.Expect(err).NotTo(o.HaveOccurred())

		sc := yamlData["storageConfig"].(map[string]interface{})
		registry := sc["registry"].(map[string]interface{})
		registry["imageURL"] = fmt.Sprintf("%s:5000/oc-mirror-%s", imageRegistryName, uuid.NewString()[:8])
		modifiedYAML, err := yaml.Marshal(yamlData)
		o.Expect(err).NotTo(o.HaveOccurred())

		imageSetConfigFile65202, err := os.CreateTemp("", "case65202-imagesetconfig-*.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer imageSetConfigFile65202.Close()

		imageSetConfigFilePath65202 := imageSetConfigFile65202.Name()
		defer os.Remove(imageSetConfigFilePath65202)

		_, err = imageSetConfigFile65202.Write(modifiedYAML)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = imageSetConfigFile65202.Close()
		o.Expect(err).NotTo(o.HaveOccurred())

		defer os.RemoveAll("oc-mirror-workspace")
		// Start mirroring the payload
		g.By("Start mirroring the multi payload")
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", imageSetConfigFilePath65202, "docker://"+imageRegistryName+":5000", "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("The first multi payload mirroring failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the multipayload mirror still failed")

		// Validate if multi arch payload has been mirrored
		g.By("Validate if multi arch payload has been mirrored")
		ref, err := docker.ParseReference("//" + imageRegistryName + ":5000/openshift/release-images:4.13.6-multi")
		o.Expect(err).NotTo(o.HaveOccurred())
		sys := &types.SystemContext{
			AuthFilePath:                dirname + "/.dockerconfigjson",
			OCIInsecureSkipTLSVerify:    true,
			DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		}
		ctx := context.Background()
		src, err := ref.NewImageSource(ctx, sys)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func(src types.ImageSource) {
			err := src.Close()
			o.Expect(err).NotTo(o.HaveOccurred())
		}(src)
		rawManifest, _, err := src.GetManifest(ctx, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest))).To(o.BeTrue())
	})

	// author: knarra@redhat.com
	g.It("ROSA-OSD_CCS-ARO-Author:knarra-NonPreRelease-Longduration-Critical-65203-Verify user is able to mirror multi payload along with single arch via oc-mirror [Serial]", func() {
		g.By("Check if imageContentSourcePolicy image-policy-aosqe exists, if not skip the case")
		existingIcspOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !(strings.Contains(existingIcspOutput, "image-policy-aosqe")) {
			g.Skip("Image-policy-aosqe icsp not found, skipping the case")
		}

		buildPruningBaseDir := exutil.FixturePath("testdata", "workloads")
		imageSetConfig65203 := filepath.Join(buildPruningBaseDir, "imageSetConfig65203.yaml")

		dirname, err := os.MkdirTemp("", "case65203-*")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Retreive image registry name
		imageRegistryName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "image-policy-aosqe", "-o=jsonpath={.spec.repositoryDigestMirrors[0].mirrors[0]}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		imageRegistryName = strings.Split(imageRegistryName, ":")[0]
		e2e.Logf("ImageRegistryName is %s", imageRegistryName)

		// Replace localhost with retreived registry name from the cluster in imageSetConfigFile
		f, err := os.Open(imageSetConfig65203)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer f.Close()

		content, err := io.ReadAll(f)
		o.Expect(err).NotTo(o.HaveOccurred())
		yamlData := make(map[string]interface{})
		err = yaml.Unmarshal(content, &yamlData)
		o.Expect(err).NotTo(o.HaveOccurred())

		sc := yamlData["storageConfig"].(map[string]interface{})
		registry := sc["registry"].(map[string]interface{})
		registry["imageURL"] = fmt.Sprintf("%s:5000/oc-mirror-%s", imageRegistryName, uuid.NewString()[:8])
		modifiedYAML, err := yaml.Marshal(yamlData)
		o.Expect(err).NotTo(o.HaveOccurred())

		imageSetConfigFile65203, err := os.CreateTemp("", "case65203-imagesetconfig-*.yaml")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer imageSetConfigFile65203.Close()

		imageSetConfigFilePath65203 := imageSetConfigFile65203.Name()
		defer os.Remove(imageSetConfigFilePath65203)

		_, err = imageSetConfigFile65203.Write(modifiedYAML)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = imageSetConfigFile65203.Close()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Start mirroring the payload
		g.By("Start mirroring the multi payload")
		cwd, err := os.Getwd()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = os.Chdir(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func(dir string) {
			err := os.Chdir(dir)
			o.Expect(err).NotTo(o.HaveOccurred())
		}(cwd)
		waitErr := wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--config", imageSetConfigFilePath65203, "docker://"+imageRegistryName+":5000", "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("The first multi payload mirroring failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the multipayload mirror still failed")

		// Validate if multi arch payload has been mirrored
		g.By("Validate if multi arch payload has been mirrored")
		archList := []architecture.Architecture{architecture.AMD64, architecture.ARM64, architecture.PPC64LE,
			architecture.S390X, architecture.MULTI}
		for _, arch := range archList {
			ref, err := docker.ParseReference(fmt.Sprintf(
				"//%s:5000/openshift/release-images:4.13.6-%s", imageRegistryName, arch.GNUString()))
			o.Expect(err).NotTo(o.HaveOccurred())
			sys := &types.SystemContext{
				AuthFilePath:                dirname + "/.dockerconfigjson",
				OCIInsecureSkipTLSVerify:    true,
				DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
			}
			ctx := context.Background()
			src, err := ref.NewImageSource(ctx, sys)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer func(src types.ImageSource) {
				err := src.Close()
				o.Expect(err).NotTo(o.HaveOccurred())
			}(src)
			rawManifest, _, err := src.GetManifest(ctx, nil)
			o.Expect(err).NotTo(o.HaveOccurred())
			matcher := o.BeFalse()
			if arch == architecture.MULTI {
				matcher = o.BeTrue()
			}
			o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest))).To(matcher)
		}

	})

	//author yinzhou@redhat.com
	g.It("NonHyperShiftHOST-ConnectedOnly-NonPreRelease-Longduration-Author:yinzhou-Medium-66194-High-66195-oc-mirror support multi-arch catalog for docker format [Serial]", func() {
		g.By("Set podman registry config")
		dirname := "/tmp/case66194"
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer os.RemoveAll(dirname)
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Set registry app")
		registry := registry{
			dockerImage: "quay.io/openshifttest/registry@sha256:1106aedc1b2e386520bc2fb797d9a7af47d651db31d8e7ab472f2352da37d1b3",
			namespace:   oc.Namespace(),
		}

		g.By("Trying to launch a registry app")
		defer registry.deleteregistry(oc)
		serInfo := registry.createregistry(oc)

		g.By("Starting mirror2mirror")
		defer os.RemoveAll("oc-mirror-workspace")
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		operatorConfigS := filepath.Join(ocmirrorBaseDir, "config-66194.yaml")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "docker://"+serInfo.serviceName, "--dest-skip-tls").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "oc-mirror command still falied")
		g.By("Check the mirrored image should still with multi-arch")
		ref, err := docker.ParseReference("//" + serInfo.serviceName + "/cpopen/ibm-zcon-zosconnect-catalog:6f02ec")
		o.Expect(err).NotTo(o.HaveOccurred())
		sys := &types.SystemContext{
			AuthFilePath:                dirname + "/.dockerconfigjson",
			OCIInsecureSkipTLSVerify:    true,
			DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		}
		ctx := context.Background()
		src, err := ref.NewImageSource(ctx, sys)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func(src types.ImageSource) {
			err := src.Close()
			o.Expect(err).NotTo(o.HaveOccurred())
		}(src)
		rawManifest, _, err := src.GetManifest(ctx, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest))).To(o.BeTrue())

		g.By("Starting mirror2disk")
		defer os.RemoveAll("66195out")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", operatorConfigS, "file://66195out").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "mirror2disk command still falied")

		g.By("Starting disk2mirror")
		err = wait.Poll(30*time.Second, 150*time.Second, func() (bool, error) {
			err1 := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("--from", "66195out/mirror_seq1_000000.tar", "docker://"+serInfo.serviceName+"/disktomirror", "--dest-skip-tls").Execute()
			if err1 != nil {
				e2e.Logf("the err:%v, and try next round", err1)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "disk2mirror command still falied")

		g.By("Check the mirrored image should still with multi-arch")
		ref2, err2 := docker.ParseReference("//" + serInfo.serviceName + "/disktomirror/cpopen/ibm-zcon-zosconnect-catalog:6f02ec")
		o.Expect(err2).NotTo(o.HaveOccurred())
		ctx2 := context.Background()
		src2, err2 := ref2.NewImageSource(ctx2, sys)
		o.Expect(err2).NotTo(o.HaveOccurred())
		defer func(src types.ImageSource) {
			err := src.Close()
			o.Expect(err).NotTo(o.HaveOccurred())
		}(src2)
		rawManifest2, _, err := src2.GetManifest(ctx2, nil)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifest.MIMETypeIsMultiImage(manifest.GuessMIMEType(rawManifest2))).To(o.BeTrue())
	})

	//author: yinzhou@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:yinzhou-Critical-65152-mirror2mirror workflow for local  multi-oci catalog [Serial]", func() {
		g.By("Set registry config")
		dirname := "/tmp/case65152"
		defer os.RemoveAll(dirname)
		err := os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = locatePodmanCred(oc, dirname)
		o.Expect(err).NotTo(o.HaveOccurred())

		registry, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ImageContentSourcePolicy", "-o=jsonpath={.items[0].spec.repositoryDigestMirrors[0].mirrors[0]}", "--ignore-not-found").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Registry is %s", registry)
		if registry == "" || strings.Contains(registry, "brew.registry.redhat.io") {
			g.Skip("There is no public registry, skip.")
		}

		publicRegistry, _, _ := strings.Cut(registry, "/")

		g.By("Copy the multi-arch catalog as OCI FBC")
		command := fmt.Sprintf("skopeo copy --all --format oci docker://registry.redhat.io/redhat/redhat-operator-index:v4.13 oci://%s  --remove-signatures", dirname+"/oci-multi-index")
		waitErr := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			_, err := exec.Command("bash", "-c", command).Output()
			if err != nil {
				e2e.Logf("copy failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the skopeo copy still failed")
		ocmirrorBaseDir := exutil.FixturePath("testdata", "workloads")
		ociFilterConfig := filepath.Join(ocmirrorBaseDir, "config-oci-65152.yaml")

		defer os.RemoveAll("oc-mirror-workspace")
		defer os.RemoveAll("olm_artifacts")
		g.By("Starting mirror2mirror ....")
		waitErr = wait.PollImmediate(300*time.Second, 3600*time.Second, func() (bool, error) {
			err := oc.WithoutNamespace().WithoutKubeconf().Run("mirror").Args("-c", ociFilterConfig, "docker://"+publicRegistry, "--dest-skip-tls").Execute()
			if err != nil {
				e2e.Logf("mirror2mirror failed, retrying...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "max time reached but the mirror still failed")

		defer removeCSAndISCP(oc)
		createCSAndISCP(oc, "cs-case65152-oci-multi-index", "openshift-marketplace", "Running", 1)
		deschedulerSub, deschedulerOG := getOperatorInfo(oc, "cluster-kube-descheduler-operator", "openshift-kube-descheduler-operator", "registry.redhat.io/redhat/redhat-operator-index:v4.13", "cs-case65152-oci-multi-index")
		defer removeOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator")
		installOperatorFromCustomCS(oc, deschedulerSub, deschedulerOG, "openshift-kube-descheduler-operator", "descheduler-operator")
	})

})
