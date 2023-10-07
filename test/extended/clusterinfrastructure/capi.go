package clusterinfrastructure

import (
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("cluster-api-operator", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-Author:zhsun-High-51061-Enable cluster API with feature gate [Disruptive]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")

		// There is a bug for GCP private cluster enable TechPreviewNoUpgrade https://issues.redhat.com/browse/OCPBUGS-5755
		publicZone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns", "cluster", "-n", "openshift-dns", "-o=jsonpath={.spec.publicZone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if exutil.CheckPlatform(oc) == "gcp" && publicZone == "" {
			g.Skip("GCP private cluster if enable TechPreviewNoUpgrade will hit https://issues.redhat.com/browse/OCPBUGS-5755, skip this case!!")
		}

		g.By("Check if cluster api is deployed, if no, enable it")
		capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(capi) == 0 {
			g.By("Enable cluster api with feature gate")
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("featuregate/cluster", "-p", `{"spec":{"featureSet": "TechPreviewNoUpgrade"}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("Check cluster is still healthy")
			waitForClusterHealthy(oc)
		}

		g.By("Check if cluster api is deployed")
		// Need to give more time in case cluster is private , we have seen it takes time , even after cluster becomes healthy , this happens only when publicZone is not present
		g.By("if publicZone is {id:qe} or any other value it implies not a private set up, so no need to wait")
		if publicZone == "" {
			time.Sleep(360)
		}
		err = wait.Poll(20*time.Second, 6*time.Minute, func() (bool, error) {
			capi, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(capi, "cluster-capi-operator") {
				return true, nil
			}
			e2e.Logf("cluster-capi-operator pod hasn't been deployed, continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "cluster-capi-operator pod deploy failed")

		g.By("Check if machine approver is deployed")
		approver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", machineApproverNamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(approver).To(o.ContainSubstring("machine-approver-capi"))

		g.By("Check if providers are deployed ")
		coreproviders, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("coreproviders", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(coreproviders).To(o.ContainSubstring("cluster-api"))
		infrastructureproviders, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructureproviders", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(infrastructureproviders).To(o.ContainSubstring(exutil.CheckPlatform(oc)))

		g.By("Check user data secret is copied from openshift-machine-api namespace to openshift-cluster-api")
		secret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(secret).To(o.ContainSubstring("worker-user-data"))
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-medium-51088-[CAPI] Prevent users from deleting providers [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")
		skipForCAPINotExist(oc)

		g.By("Delete coreprovider and infrastructureprovider")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("coreprovider", "cluster-api", "-n", clusterAPINamespace).Execute()
		o.Expect(err).To(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("infrastructureprovider", "--all", "-n", clusterAPINamespace).Execute()
		o.Expect(err).To(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-51141-[CAPI] worker-user-data secret should be synced up [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp")
		skipForCAPINotExist(oc)

		g.By("Delete worker-user-data in openshift-cluster-api namespace")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "worker-user-data", "-n", clusterAPINamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check user-data secret is synced up from openshift-machine-api to openshift-cluster-api")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			userData, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret", "-n", clusterAPINamespace).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(userData, "worker-user-data") {
				return true, nil
			}
			e2e.Logf("Continue to next round")
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "user-data secret isn't synced up from openshift-machine-api to openshift-cluster-api")
	})

	// author: dtobolik@redhat.com
	g.It("NonHyperShiftHOST-Author:dtobolik-Medium-61980-[CAPI] Workload annotation missing from deployments", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")
		skipForCAPINotExist(oc)

		deployments, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", clusterAPINamespace, "-oname").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		deploymentList := strings.Split(deployments, "\n")

		for _, deployment := range deploymentList {
			result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(deployment, "-n", clusterAPINamespace, `-o=jsonpath={.spec.template.metadata.annotations.target\.workload\.openshift\.io/management}`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).To(o.Equal(`{"effect": "PreferredDuringScheduling"}`))
		}
	})
})
