package operators

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	olmv1util "github.com/openshift/openshift-tests-private/test/extended/util/olmv1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM v1 oprun should", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("olmv1-oprun-"+getRandomString(), exutil.KubeConfigPath())
	)

	g.BeforeEach(func() {
		if !exutil.IsTechPreviewNoUpgrade(oc) {
			g.Skip("OLMv1 is supported in TP only currently, so skip it")
		}
	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68903-BundleDeployment Health resource unhealthy pod api crd ds", func() {

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthyPod              = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-pod-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-podunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyPodChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyApiservice = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-apis-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-apisunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyApiserviceChild = []olmv1util.ChildResource{
				{Kind: "APIService", Ns: ""},
			}
			unhealthyCRD = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-crd-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-crdunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyDS = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-ds-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-dsunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyDSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
		)

		exutil.By("Create unhealthy pod")
		defer unhealthyPod.DeleteWithoutCheck(oc)
		unhealthyPod.CreateWithoutCheck(oc)
		unhealthyPod.AssertHealthyWithConsistent(oc, "false")
		unhealthyPod.Delete(oc, unhealthyPodChild)

		exutil.By("Create unhealthy APIService")
		defer unhealthyApiservice.DeleteWithoutCheck(oc)
		unhealthyApiservice.CreateWithoutCheck(oc)
		unhealthyApiservice.AssertHealthyWithConsistent(oc, "false")
		unhealthyApiservice.Delete(oc, unhealthyApiserviceChild)

		exutil.By("Create unhealthy CRD")
		defer unhealthyCRD.DeleteWithoutCheck(oc)
		unhealthyCRD.CreateWithoutCheck(oc)
		unhealthyCRD.AssertHealthyWithConsistent(oc, "false")
		unhealthyCRD.DeleteWithoutCheck(oc)

		exutil.By("Create unhealthy DS")
		defer unhealthyDS.DeleteWithoutCheck(oc)
		unhealthyDS.CreateWithoutCheck(oc)
		unhealthyDS.AssertHealthyWithConsistent(oc, "false")
		unhealthyDS.Delete(oc, unhealthyDSChild)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68936-BundleDeployment Health resource healthy and install fail", func() {

		var (
			baseDir                      = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate    = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			basicBdRegistryImageTemplate = filepath.Join(baseDir, "basic-bd-registry-image.yaml")
			healthBd                     = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-healthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-healthy",
				Template: basicBdPlainImageTemplate,
			}
			healthChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "pod", Ns: "olmv1-68903-healthy"},
				{Kind: "APIService", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyDp = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-deployment-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:registry-68903-deployunhealthy",
				Template: basicBdRegistryImageTemplate,
			}
			unhealthyDpChild = []olmv1util.ChildResource{
				{Kind: "CustomResourceDefinition", Ns: ""},
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRC = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-rc-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-rcunhealth",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyRCChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyInstall = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-install-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-installunhealthy",
				Template: basicBdPlainImageTemplate,
			}
		)

		exutil.By("Create health bundledeployment")
		defer healthBd.DeleteWithoutCheck(oc)
		healthBd.Create(oc)
		healthBd.Delete(oc, healthChild)

		exutil.By("Create unhealthy deployment")
		defer unhealthyDp.DeleteWithoutCheck(oc)
		unhealthyDp.CreateWithoutCheck(oc)
		unhealthyDp.AssertHealthyWithConsistent(oc, "false")
		unhealthyDp.Delete(oc, unhealthyDpChild)

		exutil.By("Create unhealthy RC")
		defer unhealthyRC.DeleteWithoutCheck(oc)
		unhealthyRC.CreateWithoutCheck(oc)
		unhealthyRC.AssertHealthy(oc, "true") // here is possible issue
		unhealthyRC.Delete(oc, unhealthyRCChild)

		exutil.By("install fails")
		defer unhealthyInstall.DeleteWithoutCheck(oc)
		unhealthyInstall.CreateWithoutCheck(oc)
		unhealthyInstall.AssertHealthyWithConsistent(oc, "false")
		unhealthyInstall.DeleteWithoutCheck(oc)

	})

	// author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-68937-BundleDeployment Health resource unhealthy ss rs unspport", func() {

		var (
			baseDir                   = exutil.FixturePath("testdata", "olm", "v1")
			basicBdPlainImageTemplate = filepath.Join(baseDir, "basic-bd-plain-image.yaml")
			unhealthySS               = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-ss-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-ssunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthySSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
			unhealthyRS = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-rs-unhealthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-rsunhealthy",
				Template: basicBdPlainImageTemplate,
			}
			unhealthyRSChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}

			healthUnspport = olmv1util.BundleDeploymentDescription{
				BdName:   "68903-unspport-healthy",
				Address:  "quay.io/olmqe/olmv1bundle:plain-68903-unsupporthealthy",
				Template: basicBdPlainImageTemplate,
			}
			healthUnspportChild = []olmv1util.ChildResource{
				{Kind: "namespace", Ns: ""},
			}
		)

		exutil.By("Create unhealthy SS")
		defer unhealthySS.DeleteWithoutCheck(oc)
		unhealthySS.CreateWithoutCheck(oc)
		unhealthySS.AssertHealthyWithConsistent(oc, "false")
		unhealthySS.Delete(oc, unhealthySSChild)

		exutil.By("Create unhealthy RS")
		defer unhealthyRS.DeleteWithoutCheck(oc)
		unhealthyRS.CreateWithoutCheck(oc)
		unhealthyRS.AssertHealthyWithConsistent(oc, "false")
		unhealthyRS.Delete(oc, unhealthyRSChild)

		exutil.By("unsupport health")
		defer healthUnspport.DeleteWithoutCheck(oc)
		healthUnspport.CreateWithoutCheck(oc)
		healthUnspport.AssertHealthy(oc, "true")
		healthUnspport.Delete(oc, healthUnspportChild)

	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-68821-OLMv1 Supports Version Ranges during Installation", func() {
		var (
			baseDir                                       = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate                               = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate                      = filepath.Join(baseDir, "clusterextension.yaml")
			clusterextensionWithoutChannelTemplate        = filepath.Join(baseDir, "clusterextensionWithoutChannel.yaml")
			clusterextensionWithoutChannelVersionTemplate = filepath.Join(baseDir, "clusterextensionWithoutChannelVersion.yaml")
			catalog                                       = olmv1util.CatalogDescription{
				Name:     "catalog-68821",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm68821",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-68821",
				PackageName: "nginx68821",
				Channel:     "candidate-v0.0",
				Version:     ">=0.0.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v0.0, version >=0.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundleResource).To(o.ContainSubstring("v0.0.3"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.x")
		clusterextension.Channel = "candidate-v1.0"
		clusterextension.Version = "1.0.x"
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version >=0.0.1 !=1.1.0 <1.1.2")
		clusterextension.Channel = ""
		clusterextension.Version = ">=0.0.1 !=1.1.0 <1.1.2"
		clusterextension.Template = clusterextensionWithoutChannelTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundleResource).To(o.ContainSubstring("v1.0.2"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with channel empty, version empty")
		clusterextension.Channel = ""
		clusterextension.Version = ""
		clusterextension.Template = clusterextensionWithoutChannelVersionTemplate
		clusterextension.Create(oc)
		o.Expect(clusterextension.ResolvedBundleResource).To(o.ContainSubstring("v1.1.0"))
		clusterextension.Delete(oc)

		exutil.By("Create clusterextension with invalid version")
		clusterextension.Version = "!1.0.1"
		clusterextension.Template = clusterextensionTemplate
		err := clusterextension.CreateWithoutCheck(oc)
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-69196-OLMv1 Supports Version Ranges during clusterextension upgrade", func() {
		var (
			baseDir                  = exutil.FixturePath("testdata", "olm", "v1")
			catalogTemplate          = filepath.Join(baseDir, "catalog.yaml")
			clusterextensionTemplate = filepath.Join(baseDir, "clusterextension.yaml")
			catalog                  = olmv1util.CatalogDescription{
				Name:     "catalog-69196",
				Imageref: "quay.io/olmqe/olmtest-operator-index:nginxolm69196",
				Template: catalogTemplate,
			}
			clusterextension = olmv1util.ClusterExtensionDescription{
				Name:        "clusterextension-69196",
				PackageName: "nginx69196",
				Channel:     "candidate-v1.0",
				Version:     "1.0.1",
				Template:    clusterextensionTemplate,
			}
		)
		exutil.By("Create catalog")
		defer catalog.Delete(oc)
		catalog.Create(oc)

		exutil.By("Create clusterextension with channel candidate-v1.0, version 1.0.1")
		defer clusterextension.Delete(oc)
		clusterextension.Create(oc)
		o.Expect(clusterextension.InstalledBundleResource).To(o.ContainSubstring("v1.0.1"))

		exutil.By("update version to be >=1.0.1")
		clusterextension.Patch(oc, `{"spec":{"version":">=1.0.1"}}`)
		errWait := wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.0.2") {
				e2e.Logf("clusterextension.resolvedBundleResource is %s, not v1.0.2, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundleResource is not v1.0.2")
		}

		exutil.By("update channel to be candidate-v1.1")
		clusterextension.Patch(oc, `{"spec":{"channel":"candidate-v1.1"}}`)
		errWait = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 150*time.Second, false, func(ctx context.Context) (bool, error) {
			resolvedBundleResource, _ := olmv1util.GetNoEmpty(oc, "clusterextension", clusterextension.Name, "-o", "jsonpath={.status.resolvedBundleResource}")
			if !strings.Contains(resolvedBundleResource, "v1.1.0") {
				e2e.Logf("clusterextension.resolvedBundleResource is %s, not v1.1.0, and try next", resolvedBundleResource)
				return false, nil
			}
			return true, nil
		})
		if errWait != nil {
			olmv1util.GetNoEmpty(oc, "clusterextensiono", clusterextension.Name, "-o=jsonpath-as-json={.status}")
			exutil.AssertWaitPollNoErr(errWait, "clusterextension resolvedBundleResource is not v1.1.0")
		}
	})

})
