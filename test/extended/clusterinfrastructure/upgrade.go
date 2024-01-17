package clusterinfrastructure

import (
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("cluster-infrastructure-upgrade", exutil.KubeConfigPath())
	)
	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PreChkUpgrade-Author:zhsun-Medium-41804-[Upgrade]Spot/preemptible instances should not block upgrade - Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		randomMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}

		g.By("Create a spot instance on azure")
		ms := exutil.MachineSetDescription{"machineset-41804", 0}
		ms.CreateMachineSet(oc)
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, "machineset-41804", "-n", "openshift-machine-api", "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"spotVMOptions":{}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, "machineset-41804")

		g.By("Check machine and node were labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Longduration-NonPreRelease-PstChkUpgrade-Author:zhsun-Medium-41804-[Upgrade]Spot/preemptible instances should not block upgrade - Azure [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "azure")
		randomMachinesetName := exutil.GetRandomMachineSetName(oc)
		region, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", "openshift-machine-api", "-o=jsonpath={.spec.template.spec.providerSpec.value.location}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if region == "northcentralus" || region == "westus" || region == "usgovvirginia" {
			g.Skip("Skip this test scenario because it is not supported on the " + region + " region, because this region doesn't have zones")
		}
		ms := exutil.MachineSetDescription{"machineset-41804", 0}
		defer exutil.WaitForMachinesDisapper(oc, "machineset-41804")
		defer ms.DeleteMachineSet(oc)

		g.By("Check machine and node were still be labelled `interruptible-instance`")
		machine, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(machine).NotTo(o.BeEmpty())
		node, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l", "machine.openshift.io/interruptible-instance=").Output()
		o.Expect(node).NotTo(o.BeEmpty())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-Medium-61086-[Upgrade] Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		g.By("Create a new machineset")
		machinesetName := "machineset-61086"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Update machineset with imds required")
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(mapiMachineset, machinesetName, "-n", machineAPINamespace, "-p", `{"spec":{"replicas":1,"template":{"spec":{"providerSpec":{"value":{"metadataServiceOptions":{"authentication":"Required"}}}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.WaitForMachinesRunning(oc, 1, machinesetName)
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachine, "-n", machineAPINamespace, "-l", "machine.openshift.io/cluster-api-machineset="+machinesetName, "-o=jsonpath={.items[0].spec.providerSpec.value.metadataServiceOptions.authentication}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.ContainSubstring("Required"))
	})

	// author: huliu@redhat.com
	g.It("NonHyperShiftHOST-PstChkUpgrade-Author:huliu-Medium-62265-[Upgrade] Ensure controlplanemachineset is generated automatically after upgrade", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "nutanix", "openstack")
		cpmsOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("controlplanemachineset/cluster", "-n", machineAPINamespace).Output()
		e2e.Logf("cpmsOut:%s", cpmsOut)
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-Critical-22612-[Upgrade] Cluster could scale up/down after upgrade [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "vsphere", "ibmcloud", "alibabacloud", "nutanix", "openstack")
		g.By("Create a new machineset")
		machinesetName := "machineset-22612"
		ms := exutil.MachineSetDescription{machinesetName, 0}
		defer exutil.WaitForMachinesDisapper(oc, machinesetName)
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Scale up machineset")
		exutil.ScaleMachineSet(oc, machinesetName, 1)

		g.By("Scale down machineset")
		exutil.ScaleMachineSet(oc, machinesetName, 0)
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-PstChkUpgrade-Author:zhsun-Critical-70626-[Upgrade] Service of type LoadBalancer can be created successful after upgrade [Disruptive][Slow]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "azure", "gcp", "ibmcloud", "alibabacloud")
		ccmBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "ccm")
		loadBalancer := filepath.Join(ccmBaseDir, "svc-loadbalancer.yaml")
		loadBalancerService := loadBalancerServiceDescription{
			template:  loadBalancer,
			name:      "svc-loadbalancer",
			namespace: "default",
		}
		g.By("Create loadBalancerService")
		defer loadBalancerService.deleteLoadBalancerService(oc)
		loadBalancerService.createLoadBalancerService(oc)

		g.By("Check External-IP assigned")
		getLBSvcIP(oc, loadBalancerService)
	})
})
