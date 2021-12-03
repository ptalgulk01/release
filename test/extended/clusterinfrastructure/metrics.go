package clusterinfrastructure

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("metrics", exutil.KubeConfigPath())
	)

	// author: zhsun@redhat.com
	g.It("Author:zhsun-Medium-45499-mapi_current_pending_csr should reflect real pending CSR count", func() {
		g.By("Check the pending csr count")
		csrStatuses, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csr", "-o=jsonpath={.items[*].status.conditions[0].type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		csrStatusList := strings.Split(csrStatuses, " ")
		pending := 0
		for _, status := range csrStatusList {
			if status == "Pending" {
				pending++
			}
		}
		g.By("Get machine-approver-controller pod name")
		machineApproverPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", machineApproverNamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check the value of mapi_current_pending_csr")
		token := getPrometheusSAToken(oc)
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(machineApproverPodName, "-c", "machine-approver-controller", "-n", machineApproverNamespace, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://localhost:9192/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		o.Expect(metrics).To(o.ContainSubstring("mapi_current_pending_csr " + strconv.Itoa(pending)))
	})

	// author: zhsun@redhat.com
	g.It("NonPreRelease-Author:zhsun-Medium-43764-MachineHealthCheckUnterminatedShortCircuit alert should be fired when a MHC has been in a short circuit state [Serial][Slow][Disruptive]", func() {
		g.By("Create a new machineset")
		clusterinfra.SkipConditionally(oc)
		ms := clusterinfra.MachineSetDescription{"machineset-43764", 1}
		defer ms.DeleteMachineSet(oc)
		ms.CreateMachineSet(oc)

		g.By("Create a MachineHealthCheck")
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		mhcBaseDir := exutil.FixturePath("testdata", "clusterinfrastructure", "mhc")
		mhcTemplate := filepath.Join(mhcBaseDir, "mhc.yaml")
		mhc := mhcDescription{
			clusterid:       clusterID,
			maxunhealthy:    "0%",
			machineset_name: "machineset-43764",
			name:            "mhc-43764",
			template:        mhcTemplate,
		}
		defer mhc.deleteMhc(oc)
		mhc.createMhc(oc)

		g.By("Delete the node attached to the machine")
		machineName := clusterinfra.GetMachinesFromMachineSet(oc, "machineset-43764")[0]
		nodeName := clusterinfra.GetNodeNameFromMachine(oc, machineName)
		err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("node", nodeName).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get machine-api-controller pod name")
		machineAPIControllerPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-l", "api=clusterapi", "-n", machineAPINamespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check metrics mapi_machinehealthcheck_short_circuit")
		token := getPrometheusSAToken(oc)
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(machineAPIControllerPodName, "-c", "machine-healthcheck-controller", "-n", machineAPINamespace, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", token), "https://localhost:8444/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		o.Expect(metrics).To(o.ContainSubstring("mapi_machinehealthcheck_short_circuit{name=\"" + mhc.name + "\",namespace=\"openshift-machine-api\"} " + strconv.Itoa(1)))

		g.By("Check alert MachineHealthCheckUnterminatedShortCircuit is raised")
		checkAlertRaised(oc, "MachineHealthCheckUnterminatedShortCircuit")
	})
})
