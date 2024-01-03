package baremetal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	machineAPINamespace              = "openshift-machine-api"
	maxCpuUsageAllowed       float64 = 90.0
	minRequiredMemoryInBytes         = 1000000000
)

type Response struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Instance string `json:"instance"`
			} `json:"metric"`
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func checkOperatorsRunning(oc *exutil.CLI) (bool, error) {
	jpath := `{range .items[*]}{.metadata.name}:{.status.conditions[?(@.type=='Available')].status}{':'}{.status.conditions[?(@.type=='Degraded')].status}{'\n'}{end}`
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperators.config.openshift.io", "-o", "jsonpath="+jpath).Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute 'oc get clusteroperators.config.openshift.io' command: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		e2e.Logf("%s", line)
		parts := strings.Split(line, ":")
		available := parts[1] == "True"
		degraded := parts[2] == "False"

		if !available || !degraded {
			return false, nil
		}
	}

	return true, nil
}

func checkNodesRunning(oc *exutil.CLI) (bool, error) {
	nodeNames, nodeErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-o=jsonpath={.items[*].metadata.name}").Output()
	if nodeErr != nil {
		return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", nodeErr)
	}
	nodes := strings.Fields(nodeNames)
	e2e.Logf("\nNode Names are %v", nodeNames)
	for _, node := range nodes {
		nodeStatus, statusErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", node, "-o=jsonpath={.status.conditions[?(@.type=='Ready')].status}").Output()
		if statusErr != nil {
			return false, fmt.Errorf("failed to execute 'oc get nodes' command: %v", statusErr)
		}
		e2e.Logf("\nNode %s Status is %s\n", node, nodeStatus)

		if nodeStatus != "True" {
			return false, nil
		}
	}
	return true, nil
}

func waitForDeployStatus(oc *exutil.CLI, depName string, nameSpace string, depStatus string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		statusOp, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "deployment", depName, "-o=jsonpath={.status.conditions[?(@.type=='Available')].status}'").Output()
		if err != nil {
			return false, err
		}

		if strings.Contains(statusOp, depStatus) {
			e2e.Logf("Deployment %v state is %v", depName, depStatus)
			return true, nil
		}
		e2e.Logf("deployment %v is state %v, Trying again", depName, statusOp)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test deployment job is not running")
}

func getPodName(oc *exutil.CLI, ns string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-o=jsonpath={.items[0].metadata.name}", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("\nPod Name is %v", podName)
	return podName
}

func getPodStatus(oc *exutil.CLI, namespace string, podName string) string {
	podStatus, err := oc.AsAdmin().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod %s status is %q", podName, podStatus)
	return podStatus
}

func getNodeCpuUsage(oc *exutil.CLI, node string, sampling_time int) float64 {
	samplingTime := strconv.Itoa(sampling_time)

	cpu_sampling := "node_cpu_seconds_total%20%7Binstance%3D%27" + node
	cpu_sampling += "%27%2C%20mode%3D%27idle%27%7D%5B5" + samplingTime + "m%5D"
	query := "query=100%20-%20(avg%20by%20(instance)(irate(" + cpu_sampling + "))%20*%20100)"
	url := "http://localhost:9090/api/v1/query?" + query

	jsonString, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-s", url).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var response Response
	unmarshalErr := json.Unmarshal([]byte(jsonString), &response)
	o.Expect(unmarshalErr).NotTo(o.HaveOccurred())
	cpuUsage := response.Data.Result[0].Value[1].(string)
	cpu_usage, err := strconv.ParseFloat(cpuUsage, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	return cpu_usage
}

func getClusterUptime(oc *exutil.CLI) (int, error) {
	layout := "2006-01-02T15:04:05Z"
	completionTime, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[*].status.history[*].completionTime}").Output()
	returnTime, perr := time.Parse(layout, completionTime)
	if perr != nil {
		e2e.Logf("Error trying to parse uptime %s", perr)
		return 0, perr
	}
	now := time.Now()
	uptime := now.Sub(returnTime)
	uptimeByMin := int(uptime.Minutes())
	return uptimeByMin, nil
}

func getNodeavailMem(oc *exutil.CLI, node string) int {
	query := "query=node_memory_MemAvailable_bytes%7Binstance%3D%27" + node + "%27%7D"
	url := "http://localhost:9090/api/v1/query?" + query

	jsonString, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-s", url).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var response Response
	unmarshalErr := json.Unmarshal([]byte(jsonString), &response)
	o.Expect(unmarshalErr).NotTo(o.HaveOccurred())
	memUsage := response.Data.Result[0].Value[1].(string)
	availableMem, err := strconv.Atoi(memUsage)
	o.Expect(err).NotTo(o.HaveOccurred())
	return availableMem
}

// make sure operator is not processing and degraded
func checkOperator(oc *exutil.CLI, operatorName string) (bool, error) {
	output, err := oc.AsAdmin().Run("get").Args("clusteroperator", operatorName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
		e2e.Logf("clusteroperator %s is abnormal\n", operatorName)
		return false, nil
	}
	return true, nil
}

func waitForPodNotFound(oc *exutil.CLI, podName string, nameSpace string) {
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 300*time.Second, true, func(context.Context) (bool, error) {
		out, err := oc.AsAdmin().Run("get").Args("-n", nameSpace, "pods", "-o=jsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			return false, err
		}
		if !strings.Contains(out, podName) {
			e2e.Logf("Pod %v still exists is", podName)
			return true, nil
		}
		e2e.Logf("Pod %v exists, Trying again", podName)
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "The test deployment job is running")
}

func getUserFromSecret(oc *exutil.CLI, namespace string, secretName string) string {
	userbase64, pwderr := oc.AsAdmin().Run("get").Args("secrets", "-n", machineAPINamespace, secretName, "-o=jsonpath={.data.username}").Output()
	o.Expect(pwderr).ShouldNot(o.HaveOccurred())
	user, err := base64.StdEncoding.DecodeString(userbase64)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return string(user)
}

func getPassFromSecret(oc *exutil.CLI, namespace string, secretName string) string {
	pwdbase64, pwderr := oc.AsAdmin().Run("get").Args("secrets", "-n", machineAPINamespace, secretName, "-o=jsonpath={.data.password}").Output()
	o.Expect(pwderr).ShouldNot(o.HaveOccurred())
	pwd, err := base64.StdEncoding.DecodeString(pwdbase64)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return string(pwd)
}
