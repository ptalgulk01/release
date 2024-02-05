package operators

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/util/olmv1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 opeco should", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-opeco"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("OLMv1 is supported in TP only currently, so skip it")
		}
	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-VMonly-Author:jitli-High-69758-Catalogd Polling remote registries for update to images content", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			quayCLI         = container.NewQuayCLI()
			imagev1         = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v1"
			imagev2         = "quay.io/olmqe/olmtest-operator-index:nginxolm69758v2"

			catalog = olmv1util.CatalogDescription{
				Name:     "catalog-69758",
				Imageref: "quay.io/olmqe/olmtest-operator-index:test69758",
				Template: catalogTemplate,
			}
		)

		exutil.By("Get v1 v2 digestID")
		manifestDigestv1, err := quayCLI.GetImageDigest(strings.Replace(imagev1, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifestDigestv1).NotTo(o.BeEmpty())
		manifestDigestv2, err := quayCLI.GetImageDigest(strings.Replace(imagev2, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(manifestDigestv2).NotTo(o.BeEmpty())

		exutil.By("Check default digestID is v1")
		indexImageDigest, err := quayCLI.GetImageDigest(strings.Replace(catalog.Imageref, "quay.io/", "", 1))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(indexImageDigest).NotTo(o.BeEmpty())
		if indexImageDigest != manifestDigestv1 {
			//tag v1 to testrun image
			tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(catalog.Imageref, "quay.io/", "", 1), manifestDigestv1)
			if !tagResult {
				e2e.Logf("Error: %v", tagErr)
				e2e.Failf("Change tag failed on quay.io")
			}
			e2e.Logf("Successful init tag v1")
		}

		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Add image pollInterval time")
		err = oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"pollInterval":"20s"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		pollInterval, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.spec.source.image.pollInterval}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(pollInterval)).To(o.ContainSubstring("20s"))
		catalog.WaitCatalogStatus(oc, "Unpacked", 0)

		exutil.By("Collect the initial image status information")
		lastPollAttempt, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		v1bundlesDataOut, err := catalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v1bundlesImage := olmv1util.GetBundlesImageTag(v1bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the image and check for changes")
		//tag v2 to testrun image
		tagResult, tagErr := quayCLI.ChangeTag(strings.Replace(catalog.Imageref, "quay.io/", "", 1), manifestDigestv2)
		if !tagResult {
			e2e.Logf("Error: %v", tagErr)
			e2e.Failf("Change tag failed on quay.io")
		}
		e2e.Logf("Successful tag v2")

		errWait := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			lastPollAttempt2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.lastPollAttempt}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			resolvedRef2, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			if lastPollAttempt == lastPollAttempt2 || resolvedRef == resolvedRef2 {
				e2e.Logf("lastPollAttempt:%v,lastPollAttempt2:%v", lastPollAttempt, lastPollAttempt2)
				e2e.Logf("resolvedRef:%v,resolvedRef2:%v", resolvedRef, resolvedRef2)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error lastPollAttempt or resolvedRef are same")

		exutil.By("check the index content changes")
		v2bundlesDataOut, err := catalog.UnmarshalContent(oc, "bundle")
		o.Expect(err).NotTo(o.HaveOccurred())
		v2bundlesImage := olmv1util.GetBundlesImageTag(v2bundlesDataOut.Bundles)
		o.Expect(err).NotTo(o.HaveOccurred())

		if reflect.DeepEqual(v1bundlesImage, v2bundlesImage) {
			e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)
			e2e.Failf("Failed, The index content no changes")
		}
		e2e.Logf("v1bundlesImage%v, v2bundlesImage%v", v1bundlesImage, v2bundlesImage)

		exutil.By("Update use the digest image and check it")
		output, err := oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index@`+manifestDigestv1+`"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("cannot specify PollInterval while using digest-based image"))

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69123-Catalogd catalog offer the operator content through http server", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69123",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69123",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("get the index content through http service on cluster")
		unmarshalContent, err := catalog.UnmarshalContent(oc, "all")
		o.Expect(err).NotTo(o.HaveOccurred())

		allPackageName := olmv1util.ListPackagesName(unmarshalContent.Packages)
		o.Expect(allPackageName[0]).To(o.ContainSubstring("nginx69123"))

		channelData := olmv1util.GetChannelByPakcage(unmarshalContent.Channels, "nginx69123")
		o.Expect(channelData[0].Name).To(o.ContainSubstring("candidate-v0.0"))

		bundlesName := olmv1util.GetBundlesNameByPakcage(unmarshalContent.Bundles, "nginx69123")
		o.Expect(bundlesName[0]).To(o.ContainSubstring("nginx69123.v0.0.1"))

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69124-check the catalog source type before created", func() {
		var (
			baseDir             = exutil.FixturePath("testdata", "olm", "v1")
			catalogPollTemplate = filepath.Join(baseDir, "catalog-secret.yaml")
			catalog             = olmv1util.CatalogDescription{
				Name:         "catalog-69124",
				Imageref:     "quay.io/olmqe/olmtest-operator-index:nginxolm69124",
				PollInterval: "1m",
				Template:     catalogPollTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Check image pollInterval time")
		errMsg, err := oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"pollInterval":"1mm"}}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Invalid value: \"1mm\": spec.source.image.pollInterval in body")).To(o.BeTrue())

		exutil.By("Check type value")
		errMsg, err = oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"type":"redhat"}}}`, "--type=merge").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(strings.Contains(errMsg, "Unsupported value: \"redhat\": supported values: \"image\"")).To(o.BeTrue())

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69242-Catalogd deprecated package/bundlemetadata/catalogmetadata from catalog CR", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69242",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69242",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("get the old related crd package/bundlemetadata/bundledeployment")
		packageOutput, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("package").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(packageOutput)).To(o.ContainSubstring("error: the server doesn't have a resource type \"package\""))

		bundlemetadata, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("bundlemetadata").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(bundlemetadata)).To(o.ContainSubstring("error: the server doesn't have a resource type \"bundlemetadata\""))

		catalogmetadata, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalogmetadata").Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(string(catalogmetadata)).To(o.ContainSubstring("error: the server doesn't have a resource type \"catalogmetadata\""))

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69069-Replace pod-based image unpacker with an image registry client", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69069",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69069",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		initresolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Update the index image with different tag , but the same digestID")
		err = oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v1"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("Check the image is updated without wait but the resolvedSource is still the same and won't unpack again")
		statusOutput, err := olmv1util.GetNoEmpty(oc, "catalog", catalog.Name, "-o", "jsonpath={.status.phase}")
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(statusOutput, "Unpacked") {
			e2e.Failf("status is %v, not Unpacked", statusOutput)
		}
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if img != "quay.io/olmqe/olmtest-operator-index:nginxolm69069v1" {
				e2e.Logf("image: %v", img)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error image wrong or resolvedRef are same")
		v1resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if initresolvedRef != v1resolvedRef {
			e2e.Failf("initresolvedRef:%v,v1resolvedRef:%v", initresolvedRef, v1resolvedRef)
		}

		exutil.By("Update the index image with different tag and digestID")
		err = oc.AsAdmin().Run("patch").Args("catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"ref":"quay.io/olmqe/olmtest-operator-index:nginxolm69069v2"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		errWait = wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 90*time.Second, false, func(ctx context.Context) (bool, error) {
			img, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.ref}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			v2resolvedRef, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("catalog", catalog.Name, "-o=jsonpath={.status.resolvedSource.image.resolvedRef}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if initresolvedRef == v2resolvedRef || img != "quay.io/olmqe/olmtest-operator-index:nginxolm69069v2" {
				e2e.Logf("image: %v,v2resolvedRef: %v", img, v2resolvedRef)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "Error image wrong or resolvedRef are same")

	})

	// author: jitli@redhat.com
	g.It("ConnectedOnly-Author:jitli-High-69869-Catalogd Add metrics to the Storage implementation", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69869",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69869",
				Template: catalogTemplate,
			}
			metricsMsg string
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Get http content")
		packageDataOut, err := catalog.UnmarshalContent(oc, "package")
		o.Expect(err).NotTo(o.HaveOccurred())
		packageName := olmv1util.ListPackagesName(packageDataOut.Packages)
		o.Expect(packageName[0]).To(o.ContainSubstring("nginx69869"))

		exutil.By("Get token and clusterIP")
		promeEp, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("service", "-n", "openshift-catalogd", "catalogd-controller-manager-metrics-service", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(promeEp).NotTo(o.BeEmpty())

		metricsToken, err := exutil.GetSAToken(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metricsToken).NotTo(o.BeEmpty())

		catalogPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catalogPodname).NotTo(o.BeEmpty())

		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 30*time.Second, false, func(ctx context.Context) (bool, error) {
			queryContent := "https://" + promeEp + ":8443/metrics"
			metricsMsg, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catalogPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", metricsToken), queryContent).Output()
			e2e.Logf("err:%v", err)
			if strings.Contains(metricsMsg, "catalogd_http_request_duration_seconds_bucket{code=\"200\"") {
				e2e.Logf("found catalogd_http_request_duration_seconds_bucket{code=\"200\"")
				return true, nil
			}
			return false, nil
		})
		if errWait != nil {
			e2e.Logf("%v", metricsMsg)
			exutil.AssertWaitPollNoErr(errWait, "catalogd_http_request_duration_seconds_bucket{code=\"200\" not found.")
		}

	})

	// author: xzha@redhat.com
	g.It("VMonly-ConnectedOnly-Author:xzha-High-70817-catalogd support setting a pull secret", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog-secret.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:         "catalog-70817-quay",
				Imageref:     "quay.io/olmqe/olmtest-operator-index-private:nginxolm70817",
				PullSecret:   "fake-secret-70817",
				PollInterval: "1m",
				Template:     catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-70817",
				PackageName: "nginx70817",
				Template:    clusterextensionTemplate,
			}
		)

		exutil.By("1) Create secret")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-catalogd", "secret", "secret-70817-quay").Output()
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-catalogd", "secret", "generic", "secret-70817-quay", "--from-file=.dockerconfigjson=/home/cloud-user/.docker/config.json", "--type=kubernetes.io/dockerconfigjson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		exutil.By("2) Create catalog")
		defer catalog.Delete(oc)
		catalog.CreateWithoutCheck(oc)
		catalog.WaitCatalogStatus(oc, "Failing", 30)
		conditions, _ := olmv1util.GetNoEmpty(oc, "catalog", catalog.Name, "-o", "jsonpath={.status.conditions}")
		o.Expect(conditions).To(o.ContainSubstring("error fetching image"))
		o.Expect(conditions).To(o.ContainSubstring("401 Unauthorized"))

		exutil.By("3) Patch the catalog")
		patchResource(oc, asAdmin, withoutNamespace, "catalog", catalog.Name, "-p", `{"spec":{"source":{"image":{"pullSecret":"secret-70817-quay"}}}}`, "--type=merge")
		catalog.WaitCatalogStatus(oc, "Unpacked", 0)

		exutil.By("4) install clusterextension")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundleResource).To(o.ContainSubstring("v1.0.1"))
	})

	// author: jfan@redhat.com
	g.It("VMonly-ConnectedOnly-Author:jfan-High-69202-Catalogd catalog offer the operator content through http server off cluster", func() {
		var (
			baseDir         = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate = filepath.Join(baseDir, "catalog.yaml")
			catalog         = olmv1util.CatalogDescription{
				Name:     "catalog-69202",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69202",
				Template: catalogTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("port-forward the catalogd-catalogserver")
		cmd1, _, _, err := oc.AsAdmin().WithoutNamespace().Run("port-forward").Args("svc/catalogd-catalogserver", "6920:80", "-n", "openshift-catalogd").Background()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer cmd1.Process.Kill()

		exutil.By("get the index content through http service off cluster")
		errWait := wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 100*time.Second, false, func(ctx context.Context) (bool, error) {
			checkOutput, err := exec.Command("bash", "-c", "curl http://127.0.0.1:6920/catalogs/catalog-69202/all.json").Output()
			if err != nil {
				e2e.Logf("failed to execute the curl: %s. Trying again", err)
				return false, nil
			}
			if matched, _ := regexp.MatchString("nginx69202", string(checkOutput)); matched {
				e2e.Logf("Check the content off cluster success\n")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, fmt.Sprintf("Cannot get the port-forward result"))
	})
})
