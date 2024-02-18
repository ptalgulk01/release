package mco

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"github.com/openshift/openshift-tests-private/test/extended/util/architecture"
	"k8s.io/apimachinery/pkg/util/wait"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	logger "github.com/openshift/openshift-tests-private/test/extended/util/logext"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// CertExprity describes the information that MCPs are reporting about a given certificate.
type CertExpiry struct {
	// Bundle where the cert is storaged
	Bundle string `json:"bundle"`
	// Date fields have been temporarily removed by devs:  https://github.com/openshift/machine-config-operator/pull/3866
	// Expiry expiration date for the certificate
	Expiry string `json:"expiry"`
	// Subject certificate's subject
	Subject string `json:"subject"`
}

// MachineConfigPool struct is used to handle MachineConfigPool resources in OCP
type MachineConfigPool struct {
	template string
	Resource
	MinutesWaitingPerNode int
}

// MachineConfigPoolList struct handles list of MCPs
type MachineConfigPoolList struct {
	ResourceList
}

// NewMachineConfigPool create a NewMachineConfigPool struct
func NewMachineConfigPool(oc *exutil.CLI, name string) *MachineConfigPool {
	return &MachineConfigPool{Resource: *NewResource(oc, "mcp", name), MinutesWaitingPerNode: DefaultMinutesWaitingPerNode}
}

// MachineConfigPoolList construct a new node list struct to handle all existing nodes
func NewMachineConfigPoolList(oc *exutil.CLI) *MachineConfigPoolList {
	return &MachineConfigPoolList{*NewResourceList(oc, "mcp")}
}

// String implements the Stringer interface

func (mcp *MachineConfigPool) create() {
	exutil.CreateClusterResourceFromTemplate(mcp.oc, "--ignore-unknown-parameters=true", "-f", mcp.template, "-p", "NAME="+mcp.name)
	mcp.waitForComplete()
}

func (mcp *MachineConfigPool) delete() {
	logger.Infof("deleting custom mcp: %s", mcp.name)
	err := mcp.oc.AsAdmin().WithoutNamespace().Run("delete").Args("mcp", mcp.name, "--ignore-not-found=true").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (mcp *MachineConfigPool) pause(enable bool) {
	logger.Infof("patch mcp %v, change spec.paused to %v", mcp.name, enable)
	err := mcp.Patch("merge", `{"spec":{"paused": `+strconv.FormatBool(enable)+`}}`)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// IsPaused return true is mcp is paused
func (mcp *MachineConfigPool) IsPaused() bool {
	return mcp.GetOrFail(`{.spec.paused}`) == "true"
}

// IsCustom returns true if the pool is not the master pool nor the worker pool
func (mcp *MachineConfigPool) IsCustom() bool {
	return !mcp.IsMaster() && !mcp.IsWorker()
}

// IsMaster returns true if the pool is the master pool
func (mcp *MachineConfigPool) IsMaster() bool {
	return mcp.GetName() == MachineConfigPoolMaster
}

// IsWorker returns true if the pool is the worker pool
func (mcp *MachineConfigPool) IsWorker() bool {
	return mcp.GetName() == MachineConfigPoolWorker
}

// IsEmpty returns true if the pool has no nodes
func (mcp *MachineConfigPool) IsEmpty() bool {
	var (
		numNodes int
	)

	o.Eventually(func() (err error) {
		numNodes, err = mcp.getMachineCount()
		return err
	}, "2m", "10s").Should(o.Succeed(),
		"It was not possible to get the status.machineCount value for MPC %s", mcp.GetName())
	return numNodes == 0
}

// SetMaxUnavailable sets the value for maxUnavailable
func (mcp *MachineConfigPool) SetMaxUnavailable(maxUnavailable int) {
	logger.Infof("patch mcp %v, change spec.maxUnavailable to %d", mcp.name, maxUnavailable)
	err := mcp.Patch("merge", fmt.Sprintf(`{"spec":{"maxUnavailable": %d}}`, maxUnavailable))
	o.Expect(err).NotTo(o.HaveOccurred())
}

// RemoveMaxUnavailable removes spec.maxUnavailable attribute from the pool config
func (mcp *MachineConfigPool) RemoveMaxUnavailable() {
	logger.Infof("patch mcp %v, removing spec.maxUnavailable", mcp.name)
	err := mcp.Patch("json", `[{ "op": "remove", "path": "/spec/maxUnavailable" }]`)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (mcp *MachineConfigPool) getConfigNameOfSpec() (string, error) {
	output, err := mcp.Get(`{.spec.configuration.name}`)
	logger.Infof("spec.configuration.name of mcp/%v is %v", mcp.name, output)
	return output, err
}

func (mcp *MachineConfigPool) getConfigNameOfSpecOrFail() string {
	config, err := mcp.getConfigNameOfSpec()
	o.Expect(err).NotTo(o.HaveOccurred(), "Get config name of spec failed")
	return config
}

func (mcp *MachineConfigPool) getConfigNameOfStatus() (string, error) {
	output, err := mcp.Get(`{.status.configuration.name}`)
	logger.Infof("status.configuration.name of mcp/%v is %v", mcp.name, output)
	return output, err
}

func (mcp *MachineConfigPool) getConfigNameOfStatusOrFail() string {
	config, err := mcp.getConfigNameOfStatus()
	o.Expect(err).NotTo(o.HaveOccurred(), "Get config name of status failed")
	return config
}

func (mcp *MachineConfigPool) getMachineCount() (int, error) {
	machineCountStr, ocErr := mcp.Get(`{.status.machineCount}`)
	if ocErr != nil {
		logger.Infof("Error getting machineCount: %s", ocErr)
		return -1, ocErr
	}

	if machineCountStr == "" {
		return -1, fmt.Errorf(".status.machineCount value is not already set in MCP %s", mcp.GetName())
	}

	machineCount, convErr := strconv.Atoi(machineCountStr)

	if convErr != nil {
		logger.Errorf("Error converting machineCount to integer: %s", ocErr)
		return -1, convErr
	}

	return machineCount, nil
}

func (mcp *MachineConfigPool) getDegradedMachineCount() (int, error) {
	dmachineCountStr, ocErr := mcp.Get(`{.status.degradedMachineCount}`)
	if ocErr != nil {
		logger.Errorf("Error getting degradedmachineCount: %s", ocErr)
		return -1, ocErr
	}
	dmachineCount, convErr := strconv.Atoi(dmachineCountStr)

	if convErr != nil {
		logger.Errorf("Error converting degradedmachineCount to integer: %s", ocErr)
		return -1, convErr
	}

	return dmachineCount, nil
}

func (mcp *MachineConfigPool) pollMachineCount() func() string {
	return mcp.Poll(`{.status.machineCount}`)
}

func (mcp *MachineConfigPool) pollReadyMachineCount() func() string {
	return mcp.Poll(`{.status.readyMachineCount}`)
}

func (mcp *MachineConfigPool) pollDegradedMachineCount() func() string {
	return mcp.Poll(`{.status.degradedMachineCount}`)
}

// GetDegradedStatus returns the value of the 'Degraded' condition in the MCP
func (mcp *MachineConfigPool) GetDegradedStatus() (string, error) {
	return mcp.Get(`{.status.conditions[?(@.type=="Degraded")].status}`)
}

func (mcp *MachineConfigPool) pollDegradedStatus() func() string {
	return mcp.Poll(`{.status.conditions[?(@.type=="Degraded")].status}`)
}

// GetUpdatedStatus returns the value of the 'Updated' condition in the MCP
func (mcp *MachineConfigPool) GetUpdatedStatus() (string, error) {
	return mcp.Get(`{.status.conditions[?(@.type=="Updated")].status}`)
}

// GetUpdatingStatus returns the value of 'Updating' condition in the MCP
func (mcp *MachineConfigPool) GetUpdatingStatus() (string, error) {
	return mcp.Get(`{.status.conditions[?(@.type=="Updating")].status}`)
}

func (mcp *MachineConfigPool) pollUpdatedStatus() func() string {
	return mcp.Poll(`{.status.conditions[?(@.type=="Updated")].status}`)
}

func (mcp *MachineConfigPool) estimateWaitTimeInMinutes() int {
	var (
		totalNodes   int
		masterAdjust = 1.0
	)

	o.Eventually(func() int {
		var err error
		totalNodes, err = mcp.getMachineCount()
		if err != nil {
			return -1
		}
		return totalNodes
	},
		"5m", "5s").Should(o.BeNumerically(">=", 0), fmt.Sprintf("machineCount field has no value in MCP %s", mcp.name))

	// If the pool has no node configured, we wait at least 1 minute.
	// There are tests that create pools with 0 nodes and wait for the pool to be updated. They cant wait 0 minutes.
	if totalNodes == 0 {
		return 1
	}

	if mcp.IsMaster() {
		masterAdjust = 1.3 // if the pool is the master pool, we wait an extra 30% time
	}

	return int(float64(totalNodes*mcp.MinutesWaitingPerNode) * masterAdjust)
}

// SetWaitingTimeForKernelChange increases the time that the MCP will wait for the update to be executed
func (mcp *MachineConfigPool) SetWaitingTimeForKernelChange() {
	mcp.MinutesWaitingPerNode = DefaultMinutesWaitingPerNode + KernelChangeIncWait
}

// SetDefaultWaitingTime restore the default waiting time that the MCP will wait for the update to be executed
func (mcp *MachineConfigPool) SetDefaultWaitingTime() {
	mcp.MinutesWaitingPerNode = DefaultMinutesWaitingPerNode
}

// EnableOnClusterBuild() enables on-cluster build funcionality in this pool
func (mcp *MachineConfigPool) EnableOnClusterBuild() error {
	return mcp.AddLabel(OCBMachineConfigPoolLabel, "")
}

// DisableOnClusterBuild() disables on-cluster build funcionality in this pool
func (mcp *MachineConfigPool) DisableOnClusterBuild() error {
	return mcp.RemoveLabel(OCBMachineConfigPoolLabel)
}

// GetInternalIgnitionConfigURL return the internal URL used by the nodes in this pool to get the ignition config
func (mcp *MachineConfigPool) GetInternalIgnitionConfigURL(secure bool) (string, error) {
	var (
		// SecurePort is the tls secured port to serve ignition configs
		// InsecurePort is the port to serve ignition configs w/o tls
		port     = IgnitionSecurePort
		protocol = "https"
	)
	internalAPIServerURI, err := GetAPIServerInternalURI(mcp.oc)
	if err != nil {
		return "", err
	}
	if !secure {
		port = IgnitionInsecurePort
		protocol = "http"
	}

	return fmt.Sprintf("%s://%s:%d/config/%s", protocol, internalAPIServerURI, port, mcp.GetName()), nil
}

// GetMCSIgnitionConfig returns the ignition config that the MCS is serving for this pool
func (mcp *MachineConfigPool) GetMCSIgnitionConfig(secure bool, ignitionVersion string) (string, error) {
	var (
		// SecurePort is the tls secured port to serve ignition configs
		// InsecurePort is the port to serve ignition configs w/o tls
		port = IgnitionSecurePort
	)
	if !secure {
		port = IgnitionInsecurePort
	}

	url, err := mcp.GetInternalIgnitionConfigURL(secure)
	if err != nil {
		return "", err
	}

	// We will request the config from a master node
	mMcp := NewMachineConfigPool(mcp.oc.AsAdmin(), MachineConfigPoolMaster)
	masters, err := mMcp.GetNodes()
	if err != nil {
		return "", err
	}
	master := masters[0]

	logger.Infof("Remove the IPV4 iptables rules that block the ignition config")
	removedRules, err := master.RemoveIPTablesRulesByRegexp(fmt.Sprintf("%d", port))
	defer master.ExecIPTables(removedRules)
	if err != nil {
		return "", err
	}

	logger.Infof("Remove the IPV6 ip6tables rules that block the ignition config")
	removed6Rules, err := master.RemoveIP6TablesRulesByRegexp(fmt.Sprintf("%d", port))
	defer master.ExecIP6Tables(removed6Rules)
	if err != nil {
		return "", err
	}

	cmd := []string{"curl", "-s"}
	if secure {
		cmd = append(cmd, "-k")
	}
	if ignitionVersion != "" {
		cmd = append(cmd, []string{"-H", fmt.Sprintf("Accept:application/vnd.coreos.ignition+json;version=%s", ignitionVersion)}...)
	}
	cmd = append(cmd, url)

	stdout, stderr, err := master.DebugNodeWithChrootStd(cmd...)
	if err != nil {
		return stdout + stderr, err
	}
	return stdout, nil
}

// getSelectedNodes returns a list with the nodes that match the .spec.nodeSelector.matchLabels criteria plus the provided extraLabels
func (mcp *MachineConfigPool) getSelectedNodes(extraLabels string) ([]Node, error) {
	mcp.oc.NotShowInfo()
	defer mcp.oc.SetShowInfo()

	labelsString, err := mcp.Get(`{.spec.nodeSelector.matchLabels}`)
	if err != nil {
		return nil, err
	}
	labels := JSON(labelsString)
	o.Expect(labels.Exists()).Should(o.BeTrue(), fmt.Sprintf("The pool %s has no machLabels value defined", mcp.GetName()))

	nodeList := NewNodeList(mcp.oc)
	// Never select windows nodes
	requiredLabel := "kubernetes.io/os!=windows"
	if extraLabels != "" {
		requiredLabel += ","
		requiredLabel += extraLabels
	}
	for k, v := range labels.ToMap() {
		requiredLabel += fmt.Sprintf(",%s=%s", k, v.(string))
	}
	nodeList.ByLabel(requiredLabel)

	return nodeList.GetAll()
}

// GetNodesByLabel returns a list with the nodes that belong to the machine config pool and contain the given labels
func (mcp *MachineConfigPool) GetNodesByLabel(labels string) ([]Node, error) {
	mcp.oc.NotShowInfo()
	defer mcp.oc.SetShowInfo()

	nodes, err := mcp.getSelectedNodes(labels)
	if err != nil {
		return nil, err
	}

	returnNodes := []Node{}

	for _, item := range nodes {
		node := item
		primaryPool, err := node.GetPrimaryPool()
		if err != nil {
			return nil, err
		}

		if primaryPool.GetName() == mcp.GetName() {
			returnNodes = append(returnNodes, node)
		}
	}

	return returnNodes, nil
}

// GetNodes returns a list with the nodes that belong to the machine config pool, by default, windows nodes will be excluded
func (mcp *MachineConfigPool) GetNodes() ([]Node, error) {
	return mcp.GetNodesByLabel("")
}

// GetNodesWithoutArchitecture returns a list of nodes that belong to this pool and do NOT use the given architectures
func (mcp *MachineConfigPool) GetNodesWithoutArchitecture(arch architecture.Architecture, archs ...architecture.Architecture) ([]Node, error) {
	archsList := arch.String()
	for _, itemArch := range archs {
		archsList = archsList + "," + itemArch.String()
	}
	return mcp.GetNodesByLabel(fmt.Sprintf(`%s notin (%s)`, architecture.NodeArchitectureLabel, archsList))
}

// GetNodesWithoutArchitectureOrFail returns a list of nodes that belong to this pool and do NOT use the given architectures. It fails the test if any error happens
func (mcp *MachineConfigPool) GetNodesWithoutArchitectureOrFail(arch architecture.Architecture, archs ...architecture.Architecture) []Node {
	nodes, err := mcp.GetNodesWithoutArchitecture(arch)
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "In MCP %s. Cannot get the nodes NOT using architectures %s", mcp.GetName(), append(archs, arch))
	return nodes
}

// GetNodesByArchitecture returns a list of nodes that belong to this pool and use the given architecture
func (mcp *MachineConfigPool) GetNodesByArchitecture(arch architecture.Architecture, archs ...architecture.Architecture) ([]Node, error) {
	archsList := arch.String()
	for _, itemArch := range archs {
		archsList = archsList + "," + itemArch.String()
	}
	return mcp.GetNodesByLabel(fmt.Sprintf(`%s in (%s)`, architecture.NodeArchitectureLabel, archsList))
}

// GetNodesByArchitecture returns a list of nodes that belong to this pool and use the given architecture. It fails the test if any error happens
func (mcp *MachineConfigPool) GetNodesByArchitectureOrFail(arch architecture.Architecture, archs ...architecture.Architecture) []Node {
	nodes, err := mcp.GetNodesByArchitecture(arch)
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "In MCP %s. Cannot get the nodes using architectures %s", mcp.GetName(), append(archs, arch))
	return nodes
}

// GetNodesOrFail returns a list with the nodes that belong to the machine config pool and fail the test if any error happened
func (mcp *MachineConfigPool) GetNodesOrFail() []Node {
	ns, err := mcp.GetNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Cannot get the nodes in %s MCP", mcp.GetName())
	return ns
}

// GetCoreOsNodes returns a list with the CoreOs nodes that belong to the machine config pool
func (mcp *MachineConfigPool) GetCoreOsNodes() ([]Node, error) {
	return mcp.GetNodesByLabel("node.openshift.io/os_id=rhcos")
}

// GetCoreOsNodesOrFail returns a list with the nodes that belong to the machine config pool and fail the test if any error happened
func (mcp *MachineConfigPool) GetCoreOsNodesOrFail() []Node {
	ns, err := mcp.GetCoreOsNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Cannot get the coreOS nodes in %s MCP", mcp.GetName())
	return ns
}

// GetSortedNodes returns a list with the nodes that belong to the machine config pool in the same order used to update them
// when a configuration is applied
func (mcp *MachineConfigPool) GetSortedNodes() ([]Node, error) {

	poolNodes, err := mcp.GetNodes()
	if err != nil {
		return nil, err
	}

	return sortNodeList(poolNodes), nil

}

// GetSortedNodesOrFail returns a list with the nodes that belong to the machine config pool in the same order used to update them
// when a configuration is applied. If any error happens while getting the list, then the test is failed.
func (mcp *MachineConfigPool) GetSortedNodesOrFail() []Node {
	nodes, err := mcp.GetSortedNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(),
		"Cannot get the list of nodes that belong to '%s' MCP", mcp.GetName())

	return nodes
}

// GetSortedUpdatedNodes returns the list of the UpdatedNodes sorted by the time when they started to be updated.
// If maxUnavailable>0, then the function will fail if more that maxUpdatingNodes are being updated at the same time
func (mcp *MachineConfigPool) GetSortedUpdatedNodes(maxUnavailable int) []Node {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s in pool %s for all nodes to start updating.", timeToWait, mcp.name)

	poolNodes, errget := mcp.GetNodes()
	o.Expect(errget).NotTo(o.HaveOccurred(), fmt.Sprintf("Cannot get nodes in pool %s", mcp.GetName()))

	pendingNodes := poolNodes
	updatedNodes := []Node{}
	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 20*time.Second, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degradedstdout, degradederr := mcp.getDegradedMachineCount()
		if degradederr != nil {
			logger.Errorf("the err:%v, and try next round", degradederr)
			return false, nil
		}

		if degradedstdout != 0 {
			logger.Errorf("Degraded MC:\n%s", mcp.PrettyString())
			exutil.AssertWaitPollNoErr(fmt.Errorf("Degraded machines"), fmt.Sprintf("mcp %s has degraded %d machines", mcp.name, degradedstdout))
		}

		// Check that there aren't more thatn maxUpdatingNodes updating at the same time
		if maxUnavailable > 0 {
			totalUpdating := 0
			for _, node := range poolNodes {
				if node.IsUpdating() {
					totalUpdating++
				}
			}
			if totalUpdating > maxUnavailable {
				exutil.AssertWaitPollNoErr(fmt.Errorf("maxUnavailable Not Honored"), fmt.Sprintf("Pool %s, error: %d nodes were updating at the same time. Only %d nodes should be updating at the same time.", mcp.GetName(), totalUpdating, maxUnavailable))
			}
		}

		remainingNodes := []Node{}
		for _, node := range pendingNodes {
			if node.IsUpdating() {
				logger.Infof("Node %s is UPDATING", node.GetName())
				updatedNodes = append(updatedNodes, node)
			} else {
				remainingNodes = append(remainingNodes, node)
			}
		}

		if len(remainingNodes) == 0 {
			logger.Infof("All nodes have started to be updated on mcp %s", mcp.name)
			return true, nil

		}
		logger.Infof(" %d remaining nodes", len(remainingNodes))
		pendingNodes = remainingNodes
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Could not get the list of updated nodes on mcp %s", mcp.name))
	return updatedNodes
}

// GetCordonedNodes get cordoned nodes (if maxUnavailable > 1 ) otherwise return the 1st cordoned node
func (mcp *MachineConfigPool) GetCordonedNodes() []Node {

	// requirement is: when pool is in updating state, get the updating node list
	o.Expect(mcp.WaitForUpdatingStatus()).NotTo(o.HaveOccurred(), "Waiting for Updating status change failed")
	// polling all nodes in this pool and check whether all cordoned nodes (SchedulingDisabled)
	var allUpdatingNodes []Node
	err := wait.PollUntilContextTimeout(context.TODO(), 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		nodes, nerr := mcp.GetNodes()
		if nerr != nil {
			return false, fmt.Errorf("Get all linux node failed, will try again in next run %v", nerr)
		}
		for _, node := range nodes {
			schedulable, serr := node.IsSchedulable()
			if serr != nil {
				logger.Errorf("Checking node is schedulable failed %v", serr)
				continue
			}
			if !schedulable {
				allUpdatingNodes = append(allUpdatingNodes, node)
			}
		}

		return len(allUpdatingNodes) > 0, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Could not get the list of updating nodes on mcp %s", mcp.GetName()))

	return allUpdatingNodes
}

// GetUnreconcilableNodes get all nodes that value of annotation machineconfiguration.openshift.io/state is Unreconcilable
func (mcp *MachineConfigPool) GetUnreconcilableNodes() ([]Node, error) {

	allUnreconcilableNodes := []Node{}
	allNodes, err := mcp.GetNodes()
	if err != nil {
		return nil, err
	}

	for _, n := range allNodes {
		state := n.GetAnnotationOrFail(NodeAnnotationState)
		if state == "Unreconcilable" {
			allUnreconcilableNodes = append(allUnreconcilableNodes, n)
		}
	}

	return allUnreconcilableNodes, nil
}

// GetUnreconcilableNodesOrFail get all nodes that value of annotation machineconfiguration.openshift.io/state is Unreconcilable
// fail the test if any error occurred
func (mcp *MachineConfigPool) GetUnreconcilableNodesOrFail() []Node {

	allUnreconcilableNodes, err := mcp.GetUnreconcilableNodes()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Cannot get the unreconcilable nodes in %s MCP", mcp.GetName())

	return allUnreconcilableNodes
}

// WaitForNotDegradedStatus waits until MCP is not degraded, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForNotDegradedStatus() error {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s for MCP %s status to be not degraded.", timeToWait, mcp.name)

	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		stdout, err := mcp.GetDegradedStatus()
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, "False") {
			logger.Infof("MCP degraded status is False %s", mcp.name)
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		logger.Errorf("MCP: %s .Error waiting for not degraded status: %s", mcp.GetName(), err)
	}

	return err
}

// WaitForUpdatedStatus waits until MCP is rerpoting updated status, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForUpdatedStatus() error {
	return mcp.waitForConditionStatus("Updated", "True", time.Duration(mcp.estimateWaitTimeInMinutes())*time.Minute, 1*time.Minute, false)
}

// WaitForUpdatingStatus waits until MCP is rerpoting updating status, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForUpdatingStatus() error {
	return mcp.waitForConditionStatus("Updating", "True", 5*time.Minute, 5*time.Second, true)
}

func (mcp MachineConfigPool) waitForConditionStatus(condition, status string, timeout, interval time.Duration, immediate bool) error {

	logger.Infof("Waiting %s for MCP %s condition %s to be %s", timeout, mcp.GetName(), condition, status)

	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, immediate, func(ctx context.Context) (bool, error) {
		stdout, err := mcp.Get(`{.status.conditions[?(@.type=="` + condition + `")].status}`)
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, status) {
			logger.Infof("MCP %s condition %s status is %s", mcp.GetName(), condition, stdout)
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		logger.Errorf("MCP: %s .Error waiting for %s status: %s", mcp.GetName(), condition, err)
	}

	return err

}

// WaitForMachineCount waits until MCP is rerpoting the desired number of machineCount in the status, if the condition times out the returned error is != nil
func (mcp MachineConfigPool) WaitForMachineCount(expectedMachineCount int, timeToWait time.Duration) error {
	logger.Infof("Waiting %s for MCP %s to report %d machine count.", timeToWait, mcp.GetName(), expectedMachineCount)

	immediate := true
	err := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		mCount, err := mcp.getMachineCount()
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if mCount == expectedMachineCount {
			logger.Infof("MCP is reporting %d machine count", mCount)
			return true, nil
		}
		logger.Infof("Expected machine count %d. Reported machine count %d", expectedMachineCount, mCount)
		return false, nil
	})

	if err != nil {
		logger.Errorf("MCP: %s .Error waiting for %d machine count: %s", mcp.GetName(), expectedMachineCount, err)
	}

	return err
}

func (mcp *MachineConfigPool) waitForComplete() {
	timeToWait := time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute
	logger.Infof("Waiting %s for MCP %s to be completed.", timeToWait, mcp.name)

	immediate := false
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degradedstdout, degradederr := mcp.getDegradedMachineCount()
		if degradederr != nil {
			logger.Errorf("the err:%v, and try next round", degradederr)
			return false, nil
		}

		if degradedstdout != 0 {
			logger.Errorf("Degraded MC:\n%s", mcp.PrettyString())
			exutil.AssertWaitPollNoErr(fmt.Errorf("Degraded machines"), fmt.Sprintf("mcp %s has degraded %d machines", mcp.name, degradedstdout))
		}

		stdout, err := mcp.Get(`{.status.conditions[?(@.type=="Updated")].status}`)
		if err != nil {
			logger.Errorf("the err:%v, and try next round", err)
			return false, nil
		}
		if strings.Contains(stdout, "True") {
			// i.e. mcp updated=true, mc is applied successfully
			logger.Infof("The new MC has been successfully applied to MCP '%s'", mcp.name)
			return true, nil
		}
		return false, nil
	})

	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("mc operation is not completed on mcp %s", mcp.name))
}

// GetReportedOsImageOverrideValue returns the value of the os_image_url_override prometheus metric for this pool
func (mcp *MachineConfigPool) GetReportedOsImageOverrideValue() (string, error) {
	query := fmt.Sprintf(`os_image_url_override{pool="%s"}`, strings.ToLower(mcp.GetName()))

	mon, err := exutil.NewMonitor(mcp.oc.AsAdmin())
	if err != nil {
		return "", err
	}

	osImageOverride, err := mon.SimpleQuery(query)
	if err != nil {
		return "", err
	}

	jsonOsImageOverride := JSON(osImageOverride)
	status := jsonOsImageOverride.Get("status").ToString()
	if status != "success" {
		return "", fmt.Errorf("Query %s execution failed: %s", query, osImageOverride)
	}

	logger.Infof("%s metric is:%s", query, osImageOverride)

	metricValue := JSON(osImageOverride).Get("data").Get("result").Item(0).Get("value").Item(1).ToString()
	return metricValue, nil
}

// RecoverFromDegraded updates the current and desired machine configs so that the pool can recover from degraded state once the offending MC is deleted
func (mcp *MachineConfigPool) RecoverFromDegraded() error {
	logger.Infof("Recovering %s pool from degraded status", mcp.GetName())
	mcpNodes, _ := mcp.GetNodes()
	for _, node := range mcpNodes {
		logger.Infof("Restoring desired config in node: %s", node)
		if node.IsUpdated() {
			logger.Infof("node is updated, don't need to recover")
		} else {
			err := node.RestoreDesiredConfig()
			if err != nil {
				return fmt.Errorf("Error restoring desired config in node %s. Error: %s",
					mcp.GetName(), err)
			}
		}
	}

	derr := mcp.WaitForNotDegradedStatus()
	if derr != nil {
		logger.Infof("Could not recover from the degraded status: %s", derr)
		return derr
	}

	uerr := mcp.WaitForUpdatedStatus()
	if uerr != nil {
		logger.Infof("Could not recover from the degraded status: %s", uerr)
		return uerr
	}

	return nil
}

// IsRealTimeKernel returns true if the pool is using a realtime kernel
func (mcp *MachineConfigPool) IsRealTimeKernel() (bool, error) {
	nodes, err := mcp.GetNodes()
	if err != nil {
		logger.Errorf("Error getting the nodes in pool %s", mcp.GetName())
		return false, err
	}

	return nodes[0].IsRealTimeKernel()
}

// GetConfiguredMachineConfig return the MachineConfig currently configured in the pool
func (mcp *MachineConfigPool) GetConfiguredMachineConfig() (*MachineConfig, error) {
	currentMcName, err := mcp.Get("{.status.configuration.name}")
	if err != nil {
		logger.Errorf("Error getting the currently configured MC in pool %s: %s", mcp.GetName(), err)
		return nil, err
	}

	logger.Debugf("The currently configured MC in pool %s is: %s", mcp.GetName(), currentMcName)
	return NewMachineConfig(mcp.oc, currentMcName, mcp.GetName()), nil
}

// SanityCheck returns an error if the MCP is Degraded or Updating.
// We can't use WaitForUpdatedStatus or WaitForNotDegradedStatus because they always wait the interval. In a sanity check we want a fast response.
func (mcp *MachineConfigPool) SanityCheck() error {
	timeToWait := (time.Duration(mcp.estimateWaitTimeInMinutes()) * time.Minute) / 13
	logger.Infof("Waiting %s for MCP %s to be completed.", timeToWait.Round(time.Second), mcp.name)

	const trueStatus = "True"
	var message string

	immediate := true
	err := wait.PollUntilContextTimeout(context.TODO(), 1*time.Minute, timeToWait, immediate, func(ctx context.Context) (bool, error) {
		// If there are degraded machines, stop polling, directly fail
		degraded, degradederr := mcp.GetDegradedStatus()
		if degradederr != nil {
			message = fmt.Sprintf("Error gettting Degraded status: %s", degradederr)
			return false, nil
		}

		if degraded == trueStatus {
			message = fmt.Sprintf("MCP '%s' is degraded", mcp.GetName())
			return false, nil
		}

		updated, err := mcp.GetUpdatedStatus()
		if err != nil {
			message = fmt.Sprintf("Error gettting Updated status: %s", err)
			return false, nil
		}
		if updated == trueStatus {
			logger.Infof("MCP '%s' is ready for testing", mcp.name)
			return true, nil
		}
		message = fmt.Sprintf("MCP '%s' is not updated", mcp.GetName())
		return false, nil
	})

	if err != nil {
		return fmt.Errorf(message)
	}

	return nil
}

// GetCertsExpiry returns the information about the certificates trackec by the MCP
func (mcp *MachineConfigPool) GetCertsExpiry() ([]CertExpiry, error) {
	expiryString, err := mcp.Get(`{.status.certExpirys}`)
	if err != nil {
		return nil, err
	}

	var certsExp []CertExpiry

	jsonerr := json.Unmarshal([]byte(expiryString), &certsExp)

	if jsonerr != nil {
		return nil, jsonerr
	}

	return certsExp, nil
}

// GetArchitectures returns the list of architectures that the nodes in this pool are using
func (mcp *MachineConfigPool) GetArchitectures() ([]architecture.Architecture, error) {
	archs := []architecture.Architecture{}
	nodes, err := mcp.GetNodes()
	if err != nil {
		return archs, err
	}

	for _, node := range nodes {
		archs = append(archs, node.GetArchitectureOrFail())
	}

	return archs, nil
}

// GetArchitecturesOrFail returns the list of architectures that the nodes in this pool are using, if there is any error it fails the test
func (mcp *MachineConfigPool) GetArchitecturesOrFail() []architecture.Architecture {
	archs, err := mcp.GetArchitectures()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error getting the architectures used by nodes in MCP %s", mcp.GetName())
	return archs
}

// AllNodesUseArch return true if all the nodes in the pool has the given architecture
func (mcp *MachineConfigPool) AllNodesUseArch(arch architecture.Architecture) bool {
	for _, currentArch := range mcp.GetArchitecturesOrFail() {
		if arch != currentArch {
			return false
		}
	}
	return true
}

// GetAll returns a []MachineConfigPool list with all existing machine config pools sorted by creation time
func (mcpl *MachineConfigPoolList) GetAll() ([]MachineConfigPool, error) {
	mcpl.ResourceList.SortByTimestamp()
	allMCPResources, err := mcpl.ResourceList.GetAll()
	if err != nil {
		return nil, err
	}
	allMCPs := make([]MachineConfigPool, 0, len(allMCPResources))

	for _, mcpRes := range allMCPResources {
		allMCPs = append(allMCPs, *NewMachineConfigPool(mcpl.oc, mcpRes.name))
	}

	return allMCPs, nil
}

// GetAllOrFail returns a []MachineConfigPool list with all existing machine config pools sorted by creation time, if any error happens it fails the test
func (mcpl *MachineConfigPoolList) GetAllOrFail() []MachineConfigPool {
	mcps, err := mcpl.GetAll()
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "Error getting the list of existing MCP in the cluster")
	return mcps
}

// GetCompactCompatiblePool returns worker pool if the cluster is not compact/SNO. Else it will return master pool or custom pool if worker pool is empty.
// Current logic:
// If worker pool has nodes, we return worker pool
// Else if worker pool is empty
//
//		If custom pools exist
//			If any custom pool has nodes, we return the custom pool
//	     	Else (all custom pools are empty) we are in a Compact/SNO cluster with extra empty custom pools, we return master
//		Else (worker pool is empty and there is no custom pool) we are in a Compact/SNO cluster, we return master
func GetCompactCompatiblePool(oc *exutil.CLI) *MachineConfigPool {
	var (
		wMcp    = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp    = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
		mcpList = NewMachineConfigPoolList(oc)
	)

	mcpList.PrintDebugCommand()

	if IsCompactOrSNOCluster(oc) {
		return mMcp
	}

	if !wMcp.IsEmpty() {
		return wMcp
	}

	// The cluster is not Compact/SNO but the the worker pool is empty. All nodes have been moved to one or several custom pool
	for _, mcp := range mcpList.GetAllOrFail() {
		if mcp.IsCustom() && !mcp.IsEmpty() { // All worker pools were moved to cutom pools
			logger.Infof("Worker pool is empty, but there is a custom pool with nodes. Proposing %s MCP for testing", mcp.GetName())
			return &mcp
		}
	}

	e2e.Failf("Something went wrong. There is no suitable pool to execute the test case")
	return nil
}

// GetCoreOsCompatiblePool returns worker pool if it has CoreOs nodes. If there is no CoreOs node in the worker pool, then it returns master pool.
func GetCoreOsCompatiblePool(oc *exutil.CLI) *MachineConfigPool {
	var (
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
		mMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolMaster)
	)

	if len(wMcp.GetCoreOsNodesOrFail()) == 0 {
		logger.Infof("No CoreOs nodes in the worker pool. Using master pool for testing")
		return mMcp
	}

	return wMcp
}

// CreateCustomMCPByLabel Creates a new custom MCP using the nodes in the worker pool with the given label. If numNodes < 0, we will add all existing nodes to the custom pool
// If numNodes == 0, no node will be added to the new custom pool.
func CreateCustomMCPByLabel(oc *exutil.CLI, name, label string, numNodes int) (*MachineConfigPool, error) {
	wMcp := NewMachineConfigPool(oc, MachineConfigPoolWorker)
	nodes, err := wMcp.GetNodesByLabel(label)
	if err != nil {
		logger.Errorf("Could not get the nodes with %s label", label)
		return nil, err
	}

	if len(nodes) < numNodes {
		return nil, fmt.Errorf("The worker MCP only has %d nodes, it is not possible to take %d nodes from worker pool to create a custom pool",
			len(nodes), numNodes)
	}

	customMcpNodes := []Node{}
	for i, item := range nodes {
		n := item
		if numNodes > 0 && i >= numNodes {
			break
		}
		customMcpNodes = append(customMcpNodes, n)
	}

	return CreateCustomMCPByNodes(oc, name, customMcpNodes)
}

// CreateCustomMCP create a new custom MCP with the given name and the given number of nodes
// Nodes will be taken from the worker pool
func CreateCustomMCP(oc *exutil.CLI, name string, numNodes int) (*MachineConfigPool, error) {
	var (
		wMcp = NewMachineConfigPool(oc.AsAdmin(), MachineConfigPoolWorker)
	)

	workerNodes, err := wMcp.GetNodes()
	if err != nil {
		return nil, err
	}

	if numNodes > len(workerNodes) {
		return nil, fmt.Errorf("A %d nodes custom pool cannot be created because there are only %d nodes in the %s pool",
			numNodes, len(workerNodes), wMcp.GetName())
	}

	return CreateCustomMCPByNodes(oc, name, workerNodes[0:numNodes])
}

// CreateCustomMCPByNodes creates a new MCP containing the nodes provided in the "nodes" parameter
func CreateCustomMCPByNodes(oc *exutil.CLI, name string, nodes []Node) (*MachineConfigPool, error) {
	exutil.By(fmt.Sprintf("Creating custom MachineConfigPool %s with %d nodes", name, len(nodes)))

	err := NewMCOTemplate(oc, "custom-machine-config-pool.yaml").Create("-p", fmt.Sprintf("NAME=%s", name))
	if err != nil {
		logger.Errorf("Could not create a custom MCP for worker nodes with nodes %s", nodes)
		return nil, err
	}

	customMcp := NewMachineConfigPool(oc, name)

	for _, n := range nodes {
		_, err := n.AddLabel(fmt.Sprintf("node-role.kubernetes.io/%s", name), "")
		if err != nil {
			logger.Infof("Error labeling node %s to add it to pool %s", n.GetName(), customMcp.GetName())
		}
		logger.Infof("Node %s added to custom pool %s", n.GetName(), customMcp.GetName())
	}

	expectedNodes := len(nodes)
	err = customMcp.WaitForMachineCount(expectedNodes, 5*time.Minute)
	if err != nil {
		logger.Errorf("The %s MCP is not reporting the expected machine count", customMcp.GetName())
		return nil, err
	}

	err = customMcp.WaitForUpdatedStatus()
	if err != nil {
		logger.Errorf("The %s MCP is not updated", customMcp.GetName())
		return nil, err
	}

	logger.Infof("OK!\n")

	return customMcp, nil
}

// DeleteCustomMCP deletes a custom MCP properly unlabeling the nodes first
func DeleteCustomMCP(oc *exutil.CLI, name string) error {
	mcp := NewMachineConfigPool(oc, name)
	if !mcp.Exists() {
		logger.Infof("MCP %s does not exist. No need to remove it", mcp.GetName())
		return nil
	}

	exutil.By(fmt.Sprintf("Removing custom MCP %s", name))

	nodes, err := mcp.GetNodes()
	if err != nil {
		logger.Errorf("Could not get the nodes that belong to MCP %s", mcp.GetName())
		return err
	}

	label := fmt.Sprintf("node-role.kubernetes.io/%s", mcp.GetName())
	for _, node := range nodes {
		logger.Infof("Removing pool label from node %s", node.GetName())
		err := node.RemoveLabel(label)
		if err != nil {
			logger.Errorf("Could not remove the role label from node %s", node.GetName())
			return err
		}
	}

	for _, node := range nodes {
		err := node.WaitForLabelRemoved(label)
		if err != nil {
			logger.Errorf("The label %s was not removed from node %s", label, node.GetName())
		}
	}

	err = mcp.WaitForMachineCount(0, 5*time.Minute)
	if err != nil {
		logger.Errorf("The %s MCP already contains nodes, it cannot be deleted", mcp.GetName())
		return err
	}

	// Wait for worker MCP to be updated before removing the custom pool
	// in order to make sure that no node has any annotation pointing to resources that depend on the custom pool that we want to delete
	wMcp := NewMachineConfigPool(oc, MachineConfigPoolWorker)
	err = wMcp.WaitForUpdatedStatus()
	if err != nil {
		logger.Errorf("The worker MCP was not ready after removing the custom pool")
		wMcp.PrintDebugCommand()
		return err
	}

	err = mcp.Delete()
	if err != nil {
		logger.Errorf("The %s MCP could not be deleted", mcp.GetName())
		return err
	}

	logger.Infof("OK!\n")
	return nil
}

// GetPoolAndNodesForArchitectureOrFail returns a MCP in this order of priority:
// 1) The master pool if it is a arm64 compact/SNO cluster.
// 2) A custom pool with 1 arm node in it if there are arm nodes in the worker pool.
// 3) Any existing custom MCP with all nodes using arm64
// 4) The master pools if the master pool is arm64
func GetPoolAndNodesForArchitectureOrFail(oc *exutil.CLI, createMCPName string, arch architecture.Architecture, numNodes int) (*MachineConfigPool, []Node) {
	var (
		wMcp                  = NewMachineConfigPool(oc, MachineConfigPoolWorker)
		mMcp                  = NewMachineConfigPool(oc, MachineConfigPoolMaster)
		masterHasTheRightArch = mMcp.AllNodesUseArch(arch)
		mcpList               = NewMachineConfigPoolList(oc)
	)

	mcpList.PrintDebugCommand()

	if masterHasTheRightArch && IsCompactOrSNOCluster(oc) {
		return mMcp, mMcp.GetNodesOrFail()
	}

	// we check if there is an already existing pool with all its nodes using the requested architecture
	for _, pool := range mcpList.GetAllOrFail() {
		if !pool.IsCustom() {
			continue
		}

		// If there isn't a node with the requested architecture in the worker pool,
		// but there is a custom pool where all nodes have this architecture
		if !pool.IsEmpty() && pool.AllNodesUseArch(arch) {
			logger.Infof("Using the predefined MCP %s", pool.GetName())
			return &pool, pool.GetNodesOrFail()
		}
		logger.Infof("The predefined %s MCP exists, but it is not suitable for testing", pool.GetName())
	}

	// If there are nodes with the rewquested architecture in the worker pool we build our own custom MCP
	if len(wMcp.GetNodesByArchitectureOrFail(arch)) > 0 {
		var err error

		mcp, err := CreateCustomMCPByLabel(oc.AsAdmin(), createMCPName, fmt.Sprintf(`%s=%s`, architecture.NodeArchitectureLabel, arch), numNodes)
		o.Expect(err).NotTo(o.HaveOccurred(), "Error creating the custom pool for infrastructure %s", architecture.ARM64)
		return mcp, mcp.GetNodesOrFail()

	}

	// If we are in a HA cluster but worker nor custom pools meet the achitecture conditions for the test
	// we return the master pool if it is using the right architecture
	if masterHasTheRightArch {
		logger.Infof("The cluster is not a Compact/SNO cluster and there are no %s worker nodes available for testing. We will use the master pool.", arch)
		return mMcp, mMcp.GetNodesOrFail()
	}

	e2e.Failf("Something went wrong. There is no suitable pool to execute the test case using architecture %s", arch)
	return nil, nil
}

// GetPoolAndNodesForNoArchitectureOrFail returns a MCP in this order of priority:
// 1) The master pool if it is a arm64 compact/SNO cluster.
// 2) First pool that is not master and contains any node NOT using the given architecture
func GetPoolWithArchDifferentFromOrFail(oc *exutil.CLI, arch architecture.Architecture) *MachineConfigPool {
	var (
		mcpList = NewMachineConfigPoolList(oc)
		mMcp    = NewMachineConfigPool(oc, MachineConfigPoolMaster)
	)

	mcpList.PrintDebugCommand()

	// we check if there is an already existing pool with all its nodes using the requested architecture
	for _, pool := range mcpList.GetAllOrFail() {
		if pool.IsMaster() {
			continue
		}

		// If there isn't a node with the requested architecture in the worker pool,
		// but there is a custom pool where all nodes have this architecture
		if !pool.IsEmpty() && len(pool.GetNodesWithoutArchitectureOrFail(arch)) > 0 {
			logger.Infof("Using pool %s", pool.GetName())
			return &pool
		}
	}

	// It includes compact and SNO
	if len(mMcp.GetNodesWithoutArchitectureOrFail(arch)) > 0 {
		return mMcp
	}

	e2e.Failf("Something went wrong. There is no suitable pool to execute the test case. There is no pool with nodes using  an architecture different from %s", arch)
	return nil
}
