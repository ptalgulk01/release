package clusterinfrastructure

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type clusterDescription struct {
	name      string
	namespace string
	kind      string
	template  string
}

type awsClusterDescription struct {
	name      string
	namespace string
	region    string
	host      string
	template  string
}

type gcpClusterDescription struct {
	name      string
	namespace string
	region    string
	host      string
	network   string
	template  string
}

type awsMachineTemplateDescription struct {
	name         string
	namespace    string
	profile      string
	instanceType string
	zone         string
	ami          string
	subnetName   string
	subnetID     string
	sgName       string
	template     string
}

type gcpMachineTemplateDescription struct {
	name        string
	namespace   string
	region      string
	image       string
	machineType string
	clusterID   string
	subnetwork  string
	template    string
}

type capiMachineSetAWSDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
}

type capiMachineSetgcpDescription struct {
	name                string
	namespace           string
	clusterName         string
	kind                string
	replicas            int
	machineTemplateName string
	template            string
	failureDomain       string
}

// skipForCAPINotExist skip the test if capi doesn't exist
func skipForCAPINotExist(oc *exutil.CLI) {
	capi, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil || len(capi) == 0 {
		g.Skip("Skip for cluster api is not deployed!")
	}
}

func (cluster *clusterDescription) createCluster(oc *exutil.CLI) {
	e2e.Logf("Creating cluster ...")
	err := applyResourceFromTemplate(oc, "-f", cluster.template, "-p", "NAME="+cluster.name, "NAMESPACE="+clusterAPINamespace, "KIND="+cluster.kind)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsCluster *awsClusterDescription) createAWSCluster(oc *exutil.CLI) {
	e2e.Logf("Creating awsCluster ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsCluster.template, "-p", "NAME="+awsCluster.name, "NAMESPACE="+clusterAPINamespace, "REGION="+awsCluster.region, "HOST="+awsCluster.host)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsCluster *awsClusterDescription) deleteAWSCluster(oc *exutil.CLI) error {
	e2e.Logf("Deleting a awsCluster ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("awscluster", awsCluster.name, "-n", clusterAPINamespace).Execute()
}

func (gcpCluster *gcpClusterDescription) createGCPCluster(oc *exutil.CLI) {
	e2e.Logf("Creating gcpCluster ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", gcpCluster.template, "-p", "NAME="+gcpCluster.name, "NAMESPACE="+clusterAPINamespace, "REGION="+gcpCluster.region, "HOST="+gcpCluster.host, "NETWORK="+gcpCluster.network)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (gcpCluster *gcpClusterDescription) deleteGCPCluster(oc *exutil.CLI) error {
	e2e.Logf("Deleting a gcpCluster ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("gcpCluster", gcpCluster.name, "-n", clusterAPINamespace).Execute()
}

func (awsMachineTemplate *awsMachineTemplateDescription) createAWSMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineTemplate ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", awsMachineTemplate.template, "-p", "NAME="+awsMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "PROFILE="+awsMachineTemplate.profile, "INSTANCETYPE="+awsMachineTemplate.instanceType, "ZONE="+awsMachineTemplate.zone, "AMI="+awsMachineTemplate.ami, "SUBNETNAME="+awsMachineTemplate.subnetName, "SUBNETID="+awsMachineTemplate.subnetID, "SGNAME="+awsMachineTemplate.sgName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (awsMachineTemplate *awsMachineTemplateDescription) deleteAWSMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("awsmachinetemplate", awsMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}

func (gcpMachineTemplate *gcpMachineTemplateDescription) createGCPMachineTemplate(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineTemplate ...")
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", gcpMachineTemplate.template, "-p", "NAME="+gcpMachineTemplate.name, "NAMESPACE="+clusterAPINamespace, "IMAGE="+gcpMachineTemplate.image, "REGION="+gcpMachineTemplate.region, "CLUSTERID="+gcpMachineTemplate.clusterID, "MACHINETYPE="+gcpMachineTemplate.machineType, "SUBNETWORK="+gcpMachineTemplate.subnetwork)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (gcpMachineTemplate *gcpMachineTemplateDescription) deleteGCPMachineTemplate(oc *exutil.CLI) error {
	e2e.Logf("Deleting gcpMachineTemplate ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("gcpmachinetemplate", gcpMachineTemplate.name, "-n", clusterAPINamespace).Execute()
}

func (capiMachineSetAWS *capiMachineSetAWSDescription) createCapiMachineSet(oc *exutil.CLI) {
	e2e.Logf("Creating awsMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSetAWS.template, "-p", "NAME="+capiMachineSetAWS.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSetAWS.clusterName, "MACHINETEMPLATENAME="+capiMachineSetAWS.machineTemplateName, "KIND="+capiMachineSetAWS.kind, "REPLICAS="+strconv.Itoa(capiMachineSetAWS.replicas)); err != nil {
		capiMachineSetAWS.deleteCapiMachineSet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSetAWS.replicas, capiMachineSetAWS.name)
	}
}

func (capiMachineSetAWS *capiMachineSetAWSDescription) deleteCapiMachineSet(oc *exutil.CLI) error {
	e2e.Logf("Deleting awsMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSetAWS.name, "-n", clusterAPINamespace).Execute()
}

func (capiMachineSetgcp *capiMachineSetgcpDescription) createCapiMachineSetgcp(oc *exutil.CLI) {
	e2e.Logf("Creating gcpMachineSet ...")
	if err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", capiMachineSetgcp.template, "-p", "NAME="+capiMachineSetgcp.name, "NAMESPACE="+clusterAPINamespace, "CLUSTERNAME="+capiMachineSetgcp.clusterName, "MACHINETEMPLATENAME="+capiMachineSetgcp.machineTemplateName, "KIND="+capiMachineSetgcp.kind, "FAILUREDOMAIN="+capiMachineSetgcp.failureDomain, "REPLICAS="+strconv.Itoa(capiMachineSetgcp.replicas)); err != nil {
		capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		waitForCapiMachinesRunning(oc, capiMachineSetgcp.replicas, capiMachineSetgcp.name)
	}
}

func (capiMachineSetgcp *capiMachineSetgcpDescription) deleteCapiMachineSetgcp(oc *exutil.CLI) error {
	e2e.Logf("Deleting gcpMachineSet ...")
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args(capiMachineset, capiMachineSetgcp.name, "-n", clusterAPINamespace).Execute()
}

// waitForCapiMachinesRunning check if all the machines are Running in a MachineSet
func waitForCapiMachinesRunning(oc *exutil.CLI, machineNumber int, machineSetName string) {
	e2e.Logf("Waiting for the machines Running ...")
	pollErr := wait.Poll(60*time.Second, 960*time.Second, func() (bool, error) {
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachineset, machineSetName, "-o=jsonpath={.status.readyReplicas}", "-n", clusterAPINamespace).Output()
		machinesRunning, _ := strconv.Atoi(msg)
		if machinesRunning != machineNumber {
			e2e.Logf("Expected %v  machine are not Running yet and waiting up to 1 minutes ...", machineNumber)
			return false, nil
		}
		e2e.Logf("Expected %v  machines are Running", machineNumber)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Expected %v  machines are not Running after waiting up to 16 minutes ...", machineNumber))
	e2e.Logf("All machines are Running ...")
}

// waitForCapiMachinesDisapper check if all the machines are Dissappered in a MachineSet
func waitForCapiMachinesDisapper(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Waiting for the machines Dissapper ...")
	err := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		machineNames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-l", "cluster.x-k8s.io/set-name="+machineSetName, "-n", clusterAPINamespace).Output()
		if machineNames != "" {
			e2e.Logf(" Still have machines are not Disappered yet and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("All machines are Disappered")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait machine disappear failed.")
}

// waitForCapiMachinesDisappergcp check if all the machines are Dissappered in a MachineSet
func waitForCapiMachinesDisappergcp(oc *exutil.CLI, machineSetName string) {
	e2e.Logf("Waiting for the machines Dissapper ...")
	err := wait.Poll(60*time.Second, 1200*time.Second, func() (bool, error) {
		machineNames, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args(capiMachine, "-o=jsonpath={.items[*].metadata.name}", "-n", clusterAPINamespace).Output()
		if strings.Contains(machineNames, machineSetName) {
			e2e.Logf(" Still have machines are not Disappered yet and waiting up to 1 minutes ...")
			return false, nil
		}
		e2e.Logf("All machines are Disappered")
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Wait machine disappear failed.")
}
