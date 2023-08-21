package apiserverauth

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/openshift-tests-private/test/extended/util"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// fixturePathCache to store fixture path mapping, key: dir name under testdata, value: fixture path
var fixturePathCache = make(map[string]string)

type admissionWebhook struct {
	name             string
	webhookname      string
	servicenamespace string
	servicename      string
	namespace        string
	apigroups        string
	apiversions      string
	operations       string
	resources        string
	version          string
	pluralname       string
	singularname     string
	kind             string
	shortname        string
	template         string
}

type service struct {
	name      string
	clusterip string
	namespace string
	template  string
}

const (
	asAdmin          = true
	withoutNamespace = true
	contain          = false
	ok               = true
)

type User struct {
	Username string
	Password string
}

// createAdmissionWebhookFromTemplate : Used for creating different admission hooks from pre-existing template.
func (admissionHook *admissionWebhook) createAdmissionWebhookFromTemplate(oc *exutil.CLI) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", admissionHook.template, "-p", "NAME="+admissionHook.name, "WEBHOOKNAME="+admissionHook.webhookname,
		"SERVICENAMESPACE="+admissionHook.servicenamespace, "SERVICENAME="+admissionHook.servicename, "NAMESPACE="+admissionHook.namespace, "APIGROUPS="+admissionHook.apigroups, "APIVERSIONS="+admissionHook.apiversions,
		"OPERATIONS="+admissionHook.operations, "RESOURCES="+admissionHook.resources, "KIND="+admissionHook.kind, "SHORTNAME="+admissionHook.shortname,
		"SINGULARNAME="+admissionHook.singularname, "PLURALNAME="+admissionHook.pluralname, "VERSION="+admissionHook.version)
}

func (service *service) createServiceFromTemplate(oc *exutil.CLI) {
	exutil.CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "NAME="+service.name, "CLUSTERIP="+service.clusterip, "NAMESPACE="+service.namespace)
}

func compareAPIServerWebhookConditions(oc *exutil.CLI, conditionReason interface{}, conditionStatus string, conditionTypes []string) {
	for _, webHookErrorConditionType := range conditionTypes {
		// increase wait time for prow ci failures
		err := wait.Poll(20*time.Second, 300*time.Second, func() (bool, error) {
			webhookError, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiserver/cluster", "-o", `jsonpath='{.status.conditions[?(@.type=="`+webHookErrorConditionType+`")]}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			//Inline conditional statement for evaluating 1) reason and status together,2) only status.
			webhookConditionStatus := gjson.Get(webhookError, `status`).String()
			if containsAnyWebHookReason(webhookError, conditionReason) && webhookConditionStatus == conditionStatus {
				e2e.Logf("kube-apiserver admission webhook errors as \n %s ::: %s ::: %s ::: %s", conditionStatus, webhookError, webHookErrorConditionType, conditionReason)
				o.Expect(webhookError).Should(o.MatchRegexp(`"type":"%s"`, webHookErrorConditionType), "Mismatch in 'type' of admission errors reported")
				o.Expect(webhookError).Should(o.MatchRegexp(`"status":"%s"`, conditionStatus), "Mismatch in 'status' of admission errors reported")
				return true, nil
			}
			// Adding logging for more debug
			e2e.Logf("Retrying for expected kube-apiserver admission webhook error ::: %s ::: %s ::: %s ::: %s", conditionStatus, webhookError, webHookErrorConditionType, conditionReason)
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "Test Fail: Expected Kube-apiserver admissionwebhook errors not present.")
	}
}

// GetEncryptionPrefix :
func GetEncryptionPrefix(oc *exutil.CLI, key string) (string, error) {
	var etcdPodName string

	encryptionType, err1 := oc.WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.encryption.type}").Output()
	o.Expect(err1).NotTo(o.HaveOccurred())
	if encryptionType != "aesabc" && encryptionType != "aesgcm" {
		e2e.Logf("The etcd is not encrypted on!")
	}
	err := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		podName, err := oc.WithoutNamespace().Run("get").Args("pods", "-n", "openshift-etcd", "-l=etcd", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Fail to get etcd pod, error: %s. Trying again", err)
			return false, nil
		}
		etcdPodName = podName
		return true, nil
	})
	if err != nil {
		return "", err
	}
	var encryptionPrefix string
	err = wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		prefix, err := oc.WithoutNamespace().Run("rsh").Args("-n", "openshift-etcd", "-c", "etcd", etcdPodName, "bash", "-c", `etcdctl get `+key+` --prefix -w fields | grep -e "Value" | grep -o k8s:enc:`+encryptionType+`:v1:[^:]*: | head -n 1`).Output()
		if err != nil {
			e2e.Logf("Fail to rsh into etcd pod, error: %s. Trying again", err)
			return false, nil
		}
		encryptionPrefix = prefix
		return true, nil
	})
	if err != nil {
		return "", err
	}
	return encryptionPrefix, nil
}

// GetEncryptionKeyNumber :
func GetEncryptionKeyNumber(oc *exutil.CLI, patten string) (int, error) {
	secretNames, err := oc.WithoutNamespace().Run("get").Args("secrets", "-n", "openshift-config-managed", `-o=jsonpath={.items[*].metadata.name}`, "--sort-by=metadata.creationTimestamp").Output()
	if err != nil {
		e2e.Logf("Fail to get secret, error: %s", err)
		return 0, nil
	}
	rePattern := regexp.MustCompile(patten)
	locs := rePattern.FindAllStringIndex(secretNames, -1)
	i, j := locs[len(locs)-1][0], locs[len(locs)-1][1]
	maxSecretName := secretNames[i:j]
	strSlice := strings.Split(maxSecretName, "-")
	var number int
	number, err = strconv.Atoi(strSlice[len(strSlice)-1])
	if err != nil {
		e2e.Logf("Fail to get secret, error: %s", err)
		return 0, nil
	}
	return number, nil
}

// WaitEncryptionKeyMigration :
func WaitEncryptionKeyMigration(oc *exutil.CLI, secret string) (bool, error) {
	var pattern string
	var waitTime time.Duration
	if strings.Contains(secret, "openshift-apiserver") {
		pattern = `migrated-resources: .*route.openshift.io.*routes`
		waitTime = 15 * time.Minute
	} else if strings.Contains(secret, "openshift-kube-apiserver") {
		pattern = `migrated-resources: .*configmaps.*secrets.*`
		waitTime = 30 * time.Minute // see below explanation
	} else {
		return false, errors.New("Unknown key " + secret)
	}

	rePattern := regexp.MustCompile(pattern)
	// In observation, the waiting time in max can take 25 mins if it is kube-apiserver,
	// and 12 mins if it is openshift-apiserver, so the Poll parameters are long.
	err := wait.Poll(1*time.Minute, waitTime, func() (bool, error) {
		output, err := oc.WithoutNamespace().Run("get").Args("secrets", secret, "-n", "openshift-config-managed", "-o=yaml").Output()
		if err != nil {
			e2e.Logf("Fail to get the encryption key secret %s, error: %s. Trying again", secret, err)
			return false, nil
		}
		matchedStr := rePattern.FindString(output)
		if matchedStr == "" {
			e2e.Logf("Not yet see migrated-resources. Trying again")
			return false, nil
		}
		e2e.Logf("Saw all migrated-resources:\n%s", matchedStr)
		return true, nil
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// CheckIfResourceAvailable :
func CheckIfResourceAvailable(oc *exutil.CLI, resource string, resourceNames []string, namespace ...string) {
	args := append([]string{resource}, resourceNames...)
	if len(namespace) == 1 {
		args = append(args, "-n", namespace[0]) // HACK: implement no namespace input
	}
	out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, resourceName := range resourceNames {
		o.Expect(out).Should(o.ContainSubstring(resourceName))
	}
}

func waitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	errCo := wait.Poll(20*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := getCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			eq := reflect.DeepEqual(expectedStatus, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
			if eq {
				// For True False False, we want to wait some bit more time and double check, to ensure it is stably healthy
				time.Sleep(100 * time.Second)
				gottenStatus := getCoStatus(oc, coName, expectedStatus)
				eq := reflect.DeepEqual(expectedStatus, gottenStatus)
				if eq {
					e2e.Logf("Given operator %s becomes available/non-progressing/non-degraded", coName)
					return true, nil
				}
			} else {
				e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
				return true, nil
			}
		}
		return false, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

func getCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", args, coName).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

// Check ciphers for authentication operator cliconfig, openshiftapiservers.operator.openshift.io and kubeapiservers.operator.openshift.io:
func verifyCiphers(oc *exutil.CLI, expectedCipher string, operator string) error {
	return wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		switch operator {
		case "openshift-authentication":
			e2e.Logf("Get the ciphers for openshift-authentication:")
			getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-authentication", "v4-0-config-system-cliconfig", "-o=jsonpath='{.data.v4-0-config-system-cliconfig}'").Output()
			if err == nil {
				// Use jqCMD to call jq because .servingInfo part JSON comming in string format
				jqCMD := fmt.Sprintf(`echo %s | jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"'|tr -d '\n'`, getadminoutput)
				output, err := exec.Command("bash", "-c", jqCMD).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				gottenCipher := string(output)
				e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, gottenCipher)
				if expectedCipher == gottenCipher {
					e2e.Logf("Ciphers are matched: %s", gottenCipher)
					return true, nil
				}
				e2e.Logf("Ciphers are not matched: %s", gottenCipher)
				return false, nil
			}
			return false, nil

		case "openshiftapiservers.operator", "kubeapiservers.operator":
			e2e.Logf("Get the ciphers for %s:", operator)
			getadminoutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(operator, "cluster", "-o=jsonpath={.spec.observedConfig.servingInfo['cipherSuites', 'minTLSVersion']}").Output()
			if err == nil {
				e2e.Logf("Comparing the ciphers: %s with %s", expectedCipher, getadminoutput)
				if expectedCipher == getadminoutput {
					e2e.Logf("Ciphers are matched: %s", getadminoutput)
					return true, nil
				}
				e2e.Logf("Ciphers are not matched: %s", getadminoutput)
				return false, nil
			}
			return false, nil

		default:
			e2e.Logf("Operators parameters not correct..")
		}
		return false, nil
	})
}

func restoreClusterOcp41899(oc *exutil.CLI) {
	e2e.Logf("Checking openshift-controller-manager operator should be Available")
	expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	err := waitCoBecomes(oc, "openshift-controller-manager", 500, expectedStatus)
	exutil.AssertWaitPollNoErr(err, "openshift-controller-manager operator is not becomes available")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", "openshift-config").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(output, "client-ca-custom") {
		configmapErr := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "client-ca-custom", "-n", "openshift-config").Execute()
		o.Expect(configmapErr).NotTo(o.HaveOccurred())
		e2e.Logf("Cluster configmap reset to default values")
	} else {
		e2e.Logf("Cluster configmap not changed from default values")
	}
}

func checkClusterLoad(oc *exutil.CLI, nodeType, dirname string) (int, int) {
	var tmpPath string
	var errAdm error
	errAdmNode := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		tmpPath, errAdm = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "nodes", "-l", "node-role.kubernetes.io/"+nodeType, "--no-headers").OutputToFile(dirname)
		if errAdm != nil {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errAdmNode, fmt.Sprintf("Not able to run adm top command :: %v", errAdm))
	cmd := fmt.Sprintf(`cat %v | grep -v 'protocol-buffers' | awk '{print $3}'|awk -F '%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmpPath)
	cpuAvg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd = fmt.Sprintf(`cat %v | grep -v 'protocol-buffers' | awk '{print $5}'|awk -F'%%' '{ sum += $1 } END { print(sum / NR) }'|cut -d "." -f1`, tmpPath)
	memAvg, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	re, _ := regexp.Compile(`[^\w]`)
	cpuAvgs := string(cpuAvg)
	memAvgs := string(memAvg)
	cpuAvgs = re.ReplaceAllString(cpuAvgs, "")
	memAvgs = re.ReplaceAllString(memAvgs, "")
	cpuAvgVal, _ := strconv.Atoi(cpuAvgs)
	memAvgVal, _ := strconv.Atoi(memAvgs)
	return cpuAvgVal, memAvgVal
}

func checkResources(oc *exutil.CLI, dirname string) map[string]string {
	resUsedDet := make(map[string]string)
	resUsed := []string{"secrets", "deployments", "namespaces", "pods"}
	for _, key := range resUsed {
		tmpPath, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(key, "-A", "--no-headers").OutputToFile(dirname)
		o.Expect(err).NotTo(o.HaveOccurred())
		cmd := fmt.Sprintf(`cat %v | wc -l | awk '{print $1}'`, tmpPath)
		output, err := exec.Command("bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		resUsedDet[key] = string(output)
	}
	return resUsedDet
}

func getTestDataFilePath(filename string) string {
	// returns the file path of the testdata files with respect to apiserverauth subteam.
	apiDirName := "apiserverauth"
	apiBaseDir := ""
	if apiBaseDir = fixturePathCache[apiDirName]; len(apiBaseDir) == 0 {
		e2e.Logf("apiserver fixture dir is not initialized, start to create")
		apiBaseDir = exutil.FixturePath("testdata", apiDirName)
		fixturePathCache[apiDirName] = apiBaseDir
		e2e.Logf("apiserver fixture dir is initialized: %s", apiBaseDir)
	} else {
		apiBaseDir = fixturePathCache[apiDirName]
		e2e.Logf("apiserver fixture dir found in cache: %s", apiBaseDir)
	}
	return filepath.Join(apiBaseDir, filename)
}

func checkCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) {
	// Check ,compare and assert the current cluster operator status against the expected status given.
	currentCoStatus := getCoStatus(oc, coName, statusToCompare)
	o.Expect(reflect.DeepEqual(currentCoStatus, statusToCompare)).To(o.Equal(true), "Wrong %s CO status reported, actual status : %s", coName, currentCoStatus)
}

func getNodePortRange(oc *exutil.CLI) (int, int) {
	// Follow the steps in https://docs.openshift.com/container-platform/4.11/networking/configuring-node-port-service-range.html
	output, err := oc.AsAdmin().Run("get").Args("configmaps", "-n", "openshift-kube-apiserver", "config", `-o=jsonpath="{.data['config\.yaml']}"`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	rgx := regexp.MustCompile(`"service-node-port-range":\["([0-9]*)-([0-9]*)"\]`)
	rs := rgx.FindSubmatch([]byte(output))
	o.Expect(rs).To(o.HaveLen(3))

	leftBound, err := strconv.Atoi(string(rs[1]))
	o.Expect(err).NotTo(o.HaveOccurred())
	rightBound, err := strconv.Atoi(string(rs[2]))
	o.Expect(err).NotTo(o.HaveOccurred())
	return leftBound, rightBound
}

// Get a random number of int32 type [m,n], n > m
func getRandomNum(m int32, n int32) int32 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int31n(n-m+1) + m
}

func countResource(oc *exutil.CLI, resource string, namespace string) (int, error) {
	output, err := oc.Run("get").Args(resource, "-n", namespace, "-o", "jsonpath='{.items[*].metadata.name}'").Output()
	output = strings.Trim(strings.Trim(output, " "), "'")
	if output == "" {
		return 0, err
	}
	resources := strings.Split(output, " ")
	return len(resources), err
}

// GetAlertsByName get all the alerts
func GetAlertsByName(oc *exutil.CLI, alertName string) (string, error) {
	mon, monErr := exutil.NewPrometheusMonitor(oc.AsAdmin())
	if monErr != nil {
		return "", monErr
	}
	allAlerts, allAlertErr := mon.GetAlerts()
	if allAlertErr != nil {
		return "", allAlertErr
	}
	return allAlerts, nil
}

func isSNOCluster(oc *exutil.CLI) bool {
	//Only 1 master, 1 worker node and with the same hostname.
	masterNodes, _ := exutil.GetClusterNodesBy(oc, "master")
	workerNodes, _ := exutil.GetClusterNodesBy(oc, "worker")
	if len(masterNodes) == 1 && len(workerNodes) == 1 && masterNodes[0] == workerNodes[0] {
		return true
	}
	return false
}

// LoadCPUMemWorkload load cpu and memory workload
func LoadCPUMemWorkload(oc *exutil.CLI) {
	var (
		workerCPUtopstr    string
		workerCPUtopint    int
		workerMEMtopstr    string
		workerMEMtopint    int
		n                  int
		m                  int
		c                  int
		r                  int
		dn                 int
		s                  int
		cpuMetric          = 800
		memMetric          = 700
		reserveCPUP        = 50
		reserveMemP        = 50
		snoPodCapacity     = 250
		reservePodCapacity = 120
	)

	workerCPUtopall := []int{}
	workerMEMtopall := []int{}

	randomStr := exutil.GetRandomString()
	dirname := fmt.Sprintf("/tmp/-load-cpu-mem_%s/", randomStr)
	defer os.RemoveAll(dirname)
	os.MkdirAll(dirname, 0755)

	workerNode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/worker", "--no-headers").OutputToFile("load-cpu-mem_" + randomStr + "-log")
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf(`cat %v |head -1 | awk '{print $1}'`, workerNode)
	cmdOut, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	worker1 := strings.Replace(string(cmdOut), "\n", "", 1)
	// Check if there is an node.metrics on node
	err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", worker1).Execute()
	var workerTop string
	if err == nil {
		workerTop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", worker1, "--no-headers=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	cpuUsageCmd := fmt.Sprintf(`echo "%v" | awk '{print $2}'`, workerTop)
	cpuUsage, err := exec.Command("bash", "-c", cpuUsageCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cpu1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cpuUsage), "")
	cpu, _ := strconv.Atoi(cpu1)
	cpuUsageCmdP := fmt.Sprintf(`echo "%v" | awk '{print $3}'`, workerTop)
	cpuUsageP, err := exec.Command("bash", "-c", cpuUsageCmdP).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	cpuP1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cpuUsageP), "")
	cpuP, _ := strconv.Atoi(cpuP1)
	totalCPU := int(float64(cpu) / (float64(cpuP) / 100))
	cmd = fmt.Sprintf(`cat %v | awk '{print $1}'`, workerNode)
	workerCPU1, err := exec.Command("bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	workerCPU := strings.Fields(string(workerCPU1))
	workerNodeCount := len(workerCPU)
	o.Expect(err).NotTo(o.HaveOccurred())

	for i := 0; i < len(workerCPU); i++ {
		// Check if there is node.metrics on node
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", workerCPU[i]).Execute()
		var workerCPUtop string
		if err == nil {
			workerCPUtop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		workerCPUtopcmd := fmt.Sprintf(`cat %v | awk '{print $3}'`, workerCPUtop)
		workerCPUUsage, err := exec.Command("bash", "-c", workerCPUtopcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		workerCPUtopstr = regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(workerCPUUsage), "")
		workerCPUtopint, _ = strconv.Atoi(workerCPUtopstr)
		workerCPUtopall = append(workerCPUtopall, workerCPUtopint)
	}
	for j := 1; j < len(workerCPU); j++ {
		if workerCPUtopall[0] < workerCPUtopall[j] {
			workerCPUtopall[0] = workerCPUtopall[j]
		}
	}
	cpuMax := workerCPUtopall[0]
	availableCPU := int(float64(totalCPU) * (100 - float64(reserveCPUP) - float64(cpuMax)) / 100)
	e2e.Logf("----> Cluster has total CPU, Reserved CPU percentage, Max CPU of node :%v,%v,%v", totalCPU, reserveCPUP, cpuMax)
	n = int(availableCPU / int(cpuMetric))
	if n <= 0 {
		e2e.Logf("No more CPU resource is available, no load will be added!")
	} else {
		p := workerNodeCount
		if workerNodeCount == 1 {
			dn = 1
			r = 2
			c = 3
		} else {
			dn = 2
			c = 3
			if n > workerNodeCount {
				r = 3
			} else {
				r = workerNodeCount
			}
		}
		s = int(500 / n / dn)
		// Get the available pods of worker nodes, based on this, the upper limit for a namespace is calculated
		cmd1 := fmt.Sprintf(`oc describe node/%s | grep 'Non-terminated Pods' | grep -oP "[0-9]+"`, worker1)
		cmdOut1, err := exec.Command("bash", "-c", cmd1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		usedPods, err := strconv.Atoi(regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cmdOut1), ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		availablePods := snoPodCapacity - usedPods - reservePodCapacity
		if workerNodeCount > 1 {
			availablePods = availablePods * workerNodeCount
		}
		nsMax := int(availablePods / dn / r)
		if nsMax > 0 {
			if n > nsMax {
				n = nsMax
			}
		} else {
			n = 1
			r = 1
			dn = 1
			c = 3
			s = 10
		}
		e2e.Logf("Start CPU load ...")
		cpuloadCmd := fmt.Sprintf(`clusterbuster -N %v -B cpuload -P server -b 5 -r %v -p %v -d %v -c %v -s %v -W -m 1000 -D .2 -M 1 -t 36000 -x -v > %v`, n, r, p, dn, c, s, dirname+"clusterbuster-cpu-log")
		e2e.Logf("%v", cpuloadCmd)
		_, err = exec.Command("bash", "-c", cpuloadCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Wait for 3 mins(this time is based on many tests), when the load starts, it will reach a peak within a few minutes, then falls back.
		time.Sleep(180 * time.Second)
		e2e.Logf("----> Created cpuload related pods: %v", n*r*dn)
	}

	memUsageCmd := fmt.Sprintf(`echo "%v" | awk '{print $4}'`, workerTop)
	memUsage, err := exec.Command("bash", "-c", memUsageCmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	mem1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(memUsage), "")
	mem, _ := strconv.Atoi(mem1)
	memUsageCmdP := fmt.Sprintf(`echo "%v" | awk '{print $5}'`, workerTop)
	memUsageP, err := exec.Command("bash", "-c", memUsageCmdP).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	memP1 := regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(memUsageP), "")
	memP, _ := strconv.Atoi(memP1)
	totalMem := int(float64(mem) / (float64(memP) / 100))

	for i := 0; i < len(workerCPU); i++ {
		// Check if there is node.metrics on node
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodemetrics", workerCPU[i]).Execute()
		var workerMEMtop string
		if err == nil {
			workerMEMtop, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "node", workerCPU[i], "--no-headers=true").OutputToFile("load-cpu-mem_" + randomStr + "-log")
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		workerMEMtopcmd := fmt.Sprintf(`cat %v | awk '{print $5}'`, workerMEMtop)
		workerMEMUsage, err := exec.Command("bash", "-c", workerMEMtopcmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		workerMEMtopstr = regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(workerMEMUsage), "")
		workerMEMtopint, _ = strconv.Atoi(workerMEMtopstr)
		workerMEMtopall = append(workerMEMtopall, workerMEMtopint)
	}
	for j := 1; j < len(workerCPU); j++ {
		if workerMEMtopall[0] < workerMEMtopall[j] {
			workerMEMtopall[0] = workerMEMtopall[j]
		}
	}
	memMax := workerMEMtopall[0]
	availableMem := int(float64(totalMem) * (100 - float64(reserveMemP) - float64(memMax)) / 100)
	m = int(availableMem / int(memMetric))
	e2e.Logf("----> Cluster has total Mem, Reserved Mem percentage, Max memory of node :%v,%v,%v", totalMem, reserveMemP, memMax)
	if m <= 0 {
		e2e.Logf("No more memory resource is available, no load will be added!")
	} else {
		p := workerNodeCount
		if workerNodeCount == 1 {
			dn = 1
			r = 2
			c = 6
		} else {
			r = workerNodeCount
			if m > workerNodeCount {
				dn = m
			} else {
				dn = workerNodeCount
			}
			c = 3
		}
		s = int(500 / m / dn)
		// Get the available pods of worker nodes, based on this, the upper limit for a namespace is calculated
		cmd1 := fmt.Sprintf(`oc describe node/%v | grep 'Non-terminated Pods' | grep -oP "[0-9]+"`, worker1)
		cmdOut1, err := exec.Command("bash", "-c", cmd1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		usedPods, err := strconv.Atoi(regexp.MustCompile(`[^0-9 ]+`).ReplaceAllString(string(cmdOut1), ""))
		o.Expect(err).NotTo(o.HaveOccurred())
		availablePods := snoPodCapacity - usedPods - reservePodCapacity
		if workerNodeCount > 1 {
			availablePods = availablePods * workerNodeCount
		}
		nsMax := int(availablePods / dn / r)
		if nsMax > 0 {
			if m > nsMax {
				m = nsMax
			}
		} else {
			m = 1
			r = 1
			dn = 1
			c = 3
			s = 10
		}
		e2e.Logf("Start Memory load ...")
		memloadCmd := fmt.Sprintf(`clusterbuster -N %v -B memload -P server -r %v -p %v -d %v -c %v -s %v -W -x -v > %v`, m, r, p, dn, c, s, dirname+"clusterbuster-mem-log")
		e2e.Logf("%v", memloadCmd)
		_, err = exec.Command("bash", "-c", memloadCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// Wait for 5 mins, ensure that all load pods are strated up.
		time.Sleep(300 * time.Second)
		e2e.Logf("----> Created memload related pods: %v", m*r*dn)
	}
	// If load are landed, will do some checking with logs
	if n > 0 || m > 0 {
		keywords := "body: net/http: request canceled (Client.Timeout|panic"
		bustercmd := fmt.Sprintf(`cat %v | grep -iE '%s' || true`, dirname+"clusterbuster*", keywords)
		busterLogs, err := exec.Command("bash", "-c", bustercmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(busterLogs) > 0 {
			e2e.Logf("%s", busterLogs)
			e2e.Logf("Found some panic or timeout errors, if errors are  potential bug then file a bug.")
		} else {
			e2e.Logf("No errors found in clusterbuster logs")
		}
	} else {
		e2e.Logf("No more CPU and memory resource, no any load is added.")
	}
}

// CopyToFile copy a given file into a temp folder with given file name
func CopyToFile(fromPath string, toFilename string) string {
	// check if source file is regular file
	srcFileStat, err := os.Stat(fromPath)
	if err != nil {
		e2e.Failf("get source file %s stat failed: %v", fromPath, err)
	}
	if !srcFileStat.Mode().IsRegular() {
		e2e.Failf("source file %s is not a regular file", fromPath)
	}

	// open source file
	source, err := os.Open(fromPath)
	if err != nil {
		e2e.Failf("open source file %s failed: %v", fromPath, err)
	}
	defer source.Close()

	// open dest file
	saveTo := filepath.Join(e2e.TestContext.OutputDir, toFilename)
	dest, err := os.Create(saveTo)
	if err != nil {
		e2e.Failf("open destination file %s failed: %v", saveTo, err)
	}
	defer dest.Close()

	// copy from source to dest
	_, err = io.Copy(dest, source)
	if err != nil {
		e2e.Failf("copy file from %s to %s failed: %v", fromPath, saveTo, err)
	}
	return saveTo
}

func ExecCommandOnPod(oc *exutil.CLI, podname string, namespace string, command string) string {
	var podOutput string
	var execpodErr error
	errExec := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
		podOutput, execpodErr = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", namespace, podname, "--", "/bin/sh", "-c", command).Output()
		podOutput = strings.TrimSpace(podOutput)
		if execpodErr != nil {
			return false, nil
		} else if podOutput != "" {
			return true, nil
		} else {
			return false, nil
		}
	})
	exutil.AssertWaitPollNoErr(errExec, fmt.Sprintf("Not able to run command on pod %v :: %v :: %v :: %v", podname, command, podOutput, execpodErr))
	return podOutput
}

// clusterHealthcheck do cluster health check like pod, node and operators
func clusterHealthcheck(oc *exutil.CLI, dirname string) error {
	err := clusterNodesHealthcheck(oc, 600, dirname)
	if err != nil {
		return fmt.Errorf("Cluster nodes health check failed. Abnormality found in nodes.")
	}
	err = clusterOperatorHealthcheck(oc, 1500, dirname)
	if err != nil {
		return fmt.Errorf("Cluster operators health check failed. Abnormality found in cluster operators.")
	}
	err = clusterPodsHealthcheck(oc, 600, dirname)
	if err != nil {
		return fmt.Errorf("Cluster pods health check failed. Abnormality found in pods.")
	}
	return nil
}

// clusterOperatorHealthcheck check abnormal operators
func clusterOperatorHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal operators")
	errCo := wait.Poll(10*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		coLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "--no-headers").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -v '.True.*False.*False' || true`, coLogFile)
			coLogs, err := exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(coLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("No abnormality found in cluster operators...")
		return true, nil
	})
	if errCo != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errCo
}

// clusterPodsHealthcheck check abnormal pods.
func clusterPodsHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	e2e.Logf("Check the abnormal pods")
	var podLogs []byte
	errPod := wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		podLogFile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-A").OutputToFile(dirname)
		if err == nil {
			cmd := fmt.Sprintf(`cat %v | grep -ivE 'Running|Completed|namespace|installer' || true`, podLogFile)
			podLogs, err = exec.Command("bash", "-c", cmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podLogs) > 0 {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("No abnormality found in pods...")
		return true, nil
	})
	if errPod != nil {
		e2e.Logf("%s", podLogs)
	}
	return errPod
}

// clusterNodesHealthcheck check abnormal nodes
func clusterNodesHealthcheck(oc *exutil.CLI, waitTime int, dirname string) error {
	errNode := wait.Poll(5*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
		if err == nil {
			if strings.Contains(output, "NotReady") || strings.Contains(output, "SchedulingDisabled") {
				return false, nil
			}
		} else {
			return false, nil
		}
		e2e.Logf("Nodes are normal...")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		return true, nil
	})
	if errNode != nil {
		err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	return errNode
}

// apiserverReadinessProbe use for microshift to check apiserver readiness
func apiserverReadinessProbe(tokenValue string, apiserverName string) string {
	timeoutDuration := 3 * time.Second
	var bodyString string
	url := fmt.Sprintf(`%s/apis`, apiserverName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		e2e.Failf("error creating request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenValue)
	req.Header.Set("X-OpenShift-Internal-If-Not-Ready", "reject")

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeoutDuration,
	}
	errCurl := wait.PollImmediate(1*time.Second, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			e2e.Logf("Error while making curl request :: %v", err)
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == 429 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString = string(bodyBytes)
			return strings.Contains(bodyString, "The apiserver hasn't been fully initialized yet, please try again later"), nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCurl, fmt.Sprintf("error waiting for API server readiness: %v", errCurl))
	return bodyString
}

// Get one available service IP, retry 3 times
func getServiceIP(oc *exutil.CLI, clusterIP string) net.IP {
	var serviceIP net.IP
	err := wait.Poll(1*time.Second, 3*time.Second, func() (bool, error) {
		randomServiceIP := net.ParseIP(clusterIP).To4()
		if randomServiceIP != nil {
			randomServiceIP[3] += byte(rand.Intn(254 - 1))
		} else {
			randomServiceIP = net.ParseIP(clusterIP).To16()
			randomServiceIP[len(randomServiceIP)-1] = byte(rand.Intn(254 - 1))
		}
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-A", `-o=jsonpath={.items[*].spec.clusterIP}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched, _ := regexp.MatchString(randomServiceIP.String(), output); matched {
			e2e.Logf("IP %v has been used!", randomServiceIP)
			return false, nil
		}
		serviceIP = randomServiceIP
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Failed to get one available service IP!")
	return serviceIP
}

// the method is to do something with oc.
func doAction(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, parameters ...string) (string, error) {
	if asAdmin && withoutNamespace {
		return oc.AsAdmin().WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if asAdmin && !withoutNamespace {
		return oc.AsAdmin().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && withoutNamespace {
		return oc.WithoutNamespace().Run(action).Args(parameters...).Output()
	}
	if !asAdmin && !withoutNamespace {
		return oc.Run(action).Args(parameters...).Output()
	}
	return "", nil
}

// the method is to get something from resource. it is "oc get xxx" actaully
func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
	var result string
	var err error
	err = wait.Poll(6*time.Second, 300*time.Second, func() (bool, error) {
		result, err = doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("The output is %v, error is %v, and try next", result, err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to get %v", parameters))
	e2e.Logf("The resource returned:\n%v", result)
	return result
}

func getGlobalProxy(oc *exutil.CLI) (string, string, string) {
	httpProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
	httpsProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
	noProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.noProxy}")
	return httpProxy, httpsProxy, noProxy
}

// Get the pods List by label
func getPodsListByLabel(oc *exutil.CLI, namespace string, selectorLabel string) []string {
	podsOp := getResource(oc, asAdmin, withoutNamespace, "pod", "-n", namespace, "-l", selectorLabel, "-o=jsonpath={.items[*].metadata.name}")
	o.Expect(podsOp).NotTo(o.BeEmpty())
	return strings.Split(podsOp, " ")
}

func checkApiserversAuditPolicies(oc *exutil.CLI, auditPolicyName string) {
	e2e.Logf("Checking the current " + auditPolicyName + " audit policy of cluster")
	defaultProfile := getResource(oc, asAdmin, withoutNamespace, "apiserver/cluster", `-o=jsonpath={.spec.audit.profile}`)
	o.Expect(defaultProfile).Should(o.ContainSubstring(auditPolicyName), "current audit policy of cluster is not default :: "+defaultProfile)

	e2e.Logf("Checking the audit config file of kube-apiserver currently in use.")
	podsList := getPodsListByLabel(oc.AsAdmin(), "openshift-kube-apiserver", "app=openshift-kube-apiserver")
	execKasOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-kube-apiserver", "ls /etc/kubernetes/static-pod-resources/configmaps/kube-apiserver-audit-policies/")
	re := regexp.MustCompile(`policy.yaml`)
	matches := re.FindAllString(execKasOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of kube-apiserver is wrong :: %s", execKasOuptut)
	}
	e2e.Logf("Audit config file of kube-apiserver :: %s", execKasOuptut)

	e2e.Logf("Checking the audit config file of openshif-apiserver currently in use.")
	podsList = getPodsListByLabel(oc.AsAdmin(), "openshift-apiserver", "app=openshift-apiserver-a")
	execOasOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-apiserver", "cat /var/run/configmaps/config/config.yaml")
	re = regexp.MustCompile(`/var/run/configmaps/audit/policy.yaml`)
	matches = re.FindAllString(execOasOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of openshift-apiserver is wrong :: %s", execOasOuptut)
	}
	e2e.Logf("Audit config file of openshift-apiserver :: %v", matches)

	e2e.Logf("Checking the audit config file of openshif-oauth-apiserver currently in use.")
	podsList = getPodsListByLabel(oc.AsAdmin(), "openshift-oauth-apiserver", "app=openshift-oauth-apiserver")
	execAuthOuptut := ExecCommandOnPod(oc, podsList[0], "openshift-oauth-apiserver", "ls /var/run/configmaps/audit/")
	re = regexp.MustCompile(`policy.yaml`)
	matches = re.FindAllString(execAuthOuptut, -1)
	if len(matches) == 0 {
		e2e.Failf("Audit config file of openshift-oauth-apiserver is wrong :: %s", execAuthOuptut)
	}
	e2e.Logf("Audit config file of openshift-oauth-apiserver :: %v", execAuthOuptut)
}

func checkAuditLogs(oc *exutil.CLI, script string, masterNode string, namespace string) (string, int) {
	g.By(fmt.Sprintf("Get audit log file from %s", masterNode))
	masterNodeLogs, checkLogFileErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, masterNode, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", script)
	o.Expect(checkLogFileErr).NotTo(o.HaveOccurred())
	errCount := len(strings.TrimSpace(masterNodeLogs))
	return masterNodeLogs, errCount
}

func setAuditProfile(oc *exutil.CLI, patchNamespace string, patch string) string {
	expectedProgCoStatus := map[string]string{"Progressing": "True"}
	expectedCoStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}
	coOps := []string{"authentication", "openshift-apiserver"}
	patchOutput, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(patchNamespace, "--type=json", "-p", patch).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(patchOutput, "patched") {
		e2e.Logf("Checking KAS, OAS, Auththentication operators should be in Progressing and Available after audit profile change")
		g.By("Checking kube-apiserver operator should be in Progressing in 100 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 100, expectedProgCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not start progressing in 100 seconds")
		e2e.Logf("Checking kube-apiserver operator should be Available in 1500 seconds")
		err = waitCoBecomes(oc, "kube-apiserver", 1500, expectedCoStatus)
		exutil.AssertWaitPollNoErr(err, "kube-apiserver operator is not becomes available in 1500 seconds")
		// Using 60s because KAS takes long time, when KAS finished rotation, OAS and Auth should have already finished.
		for _, ops := range coOps {
			e2e.Logf("Checking %s should be Available in 60 seconds", ops)
			err = waitCoBecomes(oc, ops, 60, expectedCoStatus)
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("%v operator is not becomes available in 60 seconds", ops))
		}
		e2e.Logf("Post audit profile set. KAS, OAS and Auth operator are available after rollout")
		return patchOutput
	}
	return patchOutput
}

func getNewUser(oc *exutil.CLI, count int) ([]User, string, string) {
	usersDirPath := "/tmp/" + exutil.GetRandomString()
	usersHTpassFile := usersDirPath + "/htpasswd"
	err := os.MkdirAll(usersDirPath, 0o755)
	o.Expect(err).NotTo(o.HaveOccurred())

	htPassSecret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("oauth/cluster", "-o", "jsonpath={.spec.identityProviders[0].htpasswd.fileData.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if htPassSecret == "" {
		htPassSecret = "htpass-secret"
		os.Create(usersHTpassFile)
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-config", "secret", "generic", htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("--type=json", "-p", `[{"op": "add", "path": "/spec/identityProviders", "value": [{"htpasswd": {"fileData": {"name": "htpass-secret"}}, "mappingMethod": "claim", "name": "htpasswd", "type": "HTPasswd"}]}]`, "oauth/cluster").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Checking authentication operator should be in Progressing in 180 seconds")
		err = waitCoBecomes(oc, "authentication", 180, map[string]string{"Progressing": "True"})
		exutil.AssertWaitPollNoErr(err, "authentication operator is not start progressing in 180 seconds")
		e2e.Logf("Checking authentication operator should be Available in 600 seconds")
		err = waitCoBecomes(oc, "authentication", 600, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
		exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 600 seconds")
	} else {
		err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("-n", "openshift-config", "secret/"+htPassSecret, "--to", usersDirPath, "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	users := make([]User, count)

	for i := 0; i < count; i++ {
		// Generate new username and password
		users[i].Username = fmt.Sprintf("testuser-%v-%v", i, exutil.GetRandomString())
		users[i].Password = exutil.GetRandomString()

		// Add new user to htpasswd file in the temp directory
		cmd := fmt.Sprintf("htpasswd -b %v %v %v", usersHTpassFile, users[i].Username, users[i].Password)
		err := exec.Command("bash", "-c", cmd).Run()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	// Update htpass-secret with the modified htpasswd file
	err = oc.AsAdmin().WithoutNamespace().Run("set").Args("-n", "openshift-config", "data", "secret/"+htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Checking authentication operator should be in Progressing in 180 seconds")
	err = waitCoBecomes(oc, "authentication", 180, map[string]string{"Progressing": "True"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not start progressing in 180 seconds")
	e2e.Logf("Checking authentication operator should be Available in 600 seconds")
	err = waitCoBecomes(oc, "authentication", 600, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 600 seconds")

	return users, usersHTpassFile, htPassSecret
}

func userCleanup(oc *exutil.CLI, users []User, usersHTpassFile string, htPassSecret string) {
	defer os.RemoveAll(usersHTpassFile)
	for _, user := range users {
		// Add new user to htpasswd file in the temp directory
		cmd := fmt.Sprintf("htpasswd -D %v %v", usersHTpassFile, user.Username)
		err := exec.Command("bash", "-c", cmd).Run()
		o.Expect(err).NotTo(o.HaveOccurred())
	}

	// Update htpass-secret with the modified htpasswd file
	err := oc.AsAdmin().WithoutNamespace().Run("set").Args("-n", "openshift-config", "data", "secret/"+htPassSecret, "--from-file", "htpasswd="+usersHTpassFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Checking authentication operator should be in Progressing in 180 seconds")
	err = waitCoBecomes(oc, "authentication", 180, map[string]string{"Progressing": "True"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not start progressing in 180 seconds")
	e2e.Logf("Checking authentication operator should be Available in 600 seconds")
	err = waitCoBecomes(oc, "authentication", 600, map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"})
	exutil.AssertWaitPollNoErr(err, "authentication operator is not becomes available in 600 seconds")
}

func isConnectedInternet(oc *exutil.CLI) bool {
	masterNode, masterErr := exutil.GetFirstMasterNode(oc)
	o.Expect(masterErr).NotTo(o.HaveOccurred())

	cmd := `timeout 9 curl -k https://github.com/openshift/ruby-hello-world/ > /dev/null;[ $? -eq 0 ] && echo "connected"`
	output, _ := exutil.DebugNodeWithChroot(oc, masterNode, "bash", "-c", cmd)
	if matched, _ := regexp.MatchString("connected", output); !matched {
		// Failed to access to the internet in the cluster.
		return false
	}
	return true
}

func restartMicroshift(oc *exutil.CLI, nodename string) {
	_, restartErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodename, []string{"-q"}, "bash", "-c", "systemctl restart microshift")
	o.Expect(restartErr).NotTo(o.HaveOccurred())
	mstatusErr := wait.Poll(6*time.Second, 300*time.Second, func() (bool, error) {
		output, err := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodename, []string{"-q"}, "bash", "-c", "systemctl is-active microshift")
		if err == nil && strings.TrimSpace(output) == "active" {
			e2e.Logf("microshift status is: %v ", output)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(mstatusErr, fmt.Sprintf("Failed to restart Microshift: %v", mstatusErr))
}

func replacePatternInfile(oc *exutil.CLI, microshiftFilePathYaml string, oldPattern string, newPattern string) {
	content, err := ioutil.ReadFile(microshiftFilePathYaml)
	o.Expect(err).NotTo(o.HaveOccurred())

	re := regexp.MustCompile(oldPattern)
	newContent := re.ReplaceAll(content, []byte(newPattern))

	err = ioutil.WriteFile(microshiftFilePathYaml, newContent, 0644)
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Get the pods List by label
func getPodsList(oc *exutil.CLI, namespace string) []string {
	podsOp := getResource(oc, asAdmin, withoutNamespace, "pod", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}")
	podNames := strings.Split(strings.TrimSpace(podsOp), " ")
	e2e.Logf("Namespace %s pods are: %s", namespace, string(podsOp))
	return podNames
}

func changeMicroshiftConfig(oc *exutil.CLI, configStr string, nodeName string, namespace string, configPath string) {
	etcConfigCMD := fmt.Sprintf(`
configfile=%v
cat > $configfile << EOF
%v
EOF`, configPath, configStr)
	_, mchgConfigErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeName, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", etcConfigCMD)
	o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
}

func addKustomizationToMicroshift(oc *exutil.CLI, nodeName string, namespace string, kustomizationFiles map[string][]string) {
	for key, file := range kustomizationFiles {
		tmpFileName := getTestDataFilePath(file[0])
		replacePatternInfile(oc, tmpFileName, file[2], file[3])
		fileOutput, err := exec.Command("bash", "-c", fmt.Sprintf(`cat %s`, tmpFileName)).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		destFile := file[1] + strings.Split(key, ".")[0] + ".yaml"
		fileCmd := fmt.Sprintf(`echo '%s' | tee %s `, string(fileOutput), destFile)
		_, mchgConfigErr := exutil.DebugNodeRetryWithOptionsAndChroot(oc, nodeName, []string{"--quiet=true", "--to-namespace=" + namespace}, "bash", "-c", fileCmd)
		o.Expect(mchgConfigErr).NotTo(o.HaveOccurred())
	}
}

// Check ciphers of configmap of kube-apiservers, openshift-apiservers and oauth-openshift-apiservers are using.
func verifyHypershiftCiphers(oc *exutil.CLI, expectedCipher string, ns string) error {
	var (
		cipherStr string
		randomStr = exutil.GetRandomString()
		tmpDir    = fmt.Sprintf("/tmp/-api-%s/", randomStr)
	)

	defer os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)

	for _, item := range []string{"kube-apiserver", "openshift-apiserver", "oauth-openshift"} {
		e2e.Logf("#### Checking the ciphers of  %s:", item)
		if item == "kube-apiserver" {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns, "kas-config", `-o=jsonpath='{.data.config\.json}'`).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// Use jq command line to extrack .servingInfo part JSON comming in string format
			jqCmd := fmt.Sprintf(`echo %s | jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"'|tr -d '\n'`, out)
			outJQ, err := exec.Command("bash", "-c", jqCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cipherStr = string(outJQ)
		} else {
			jsonOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns, item, `-ojson`).OutputToFile("api-" + randomStr + "." + item)
			o.Expect(err).NotTo(o.HaveOccurred())
			jqCmd := fmt.Sprintf(`cat %v | jq -r '.data."config.yaml"'`, jsonOut)
			yamlConfig, err := exec.Command("bash", "-c", jqCmd).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			jsonConfig, errJson := util.Yaml2Json(string(yamlConfig))
			o.Expect(errJson).NotTo(o.HaveOccurred())

			jsonFile := tmpDir + item + "config.json"
			f, err := os.Create(jsonFile)
			o.Expect(err).NotTo(o.HaveOccurred())
			defer f.Close()
			w := bufio.NewWriter(f)
			_, err = fmt.Fprintf(w, "%s", jsonConfig)
			w.Flush()
			o.Expect(err).NotTo(o.HaveOccurred())

			jqCmd1 := fmt.Sprintf(`jq -cr '.servingInfo | "\(.cipherSuites) \(.minTLSVersion)"' %s |tr -d '\n'`, jsonFile)
			jsonOut1, err := exec.Command("bash", "-c", jqCmd1).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cipherStr = string(jsonOut1)
		}
		e2e.Logf("#### Checking if the ciphers has been changed as the expected: %s", expectedCipher)
		if expectedCipher != cipherStr {
			e2e.Logf("#### Ciphers of %s are: %s", item, cipherStr)
			return fmt.Errorf("Ciphers not matched")
		}
		e2e.Logf("#### Ciphers are matched.")
	}
	return nil
}

// Waiting for apiservers restart
func waitApiserverRestartOfHypershift(oc *exutil.CLI, appLabel string, ns string, waitTime int) error {
	re, err := regexp.Compile(`(0/[0-9]|Pending|Terminating|Init)`)
	o.Expect(err).NotTo(o.HaveOccurred())
	errKas := wait.Poll(20*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app="+appLabel, "--no-headers", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if matched := re.MatchString(out); matched {
			e2e.Logf("#### %s was restarting ...", appLabel)
			return false, nil
		}
		time.Sleep(20 * time.Second)
		// Recheck status of pods and confirm twice, avoid false restarts
		for i := 1; i <= 4; i++ {
			out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "app="+appLabel, "--no-headers", "-n", ns).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if matchedAgain := re.MatchString(out); matchedAgain {
				e2e.Logf("#### %s was restarting ...", appLabel)
				return false, nil
			}
			time.Sleep(10 * time.Second)
		}
		e2e.Logf("#### %s have been restarted!", appLabel)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errKas, "Failed to complete the restart within the expected time, please check the cluster status!")
	return errKas
}

func containsAnyWebHookReason(webhookError string, conditionReasons interface{}) bool {
	switch reasons := conditionReasons.(type) {
	case string:
		return strings.Contains(webhookError, reasons)
	case []string:
		for _, reason := range reasons {
			if strings.Contains(webhookError, reason) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func clientCurl(tokenValue string, url string) string {
	timeoutDuration := 3 * time.Second
	var bodyString string

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		e2e.Failf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenValue)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeoutDuration,
	}

	errCurl := wait.PollImmediate(10*time.Second, 300*time.Second, func() (bool, error) {
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			bodyBytes, _ := ioutil.ReadAll(resp.Body)
			bodyString = string(bodyBytes)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(errCurl, fmt.Sprintf("error waiting for curl request output: %v", errCurl))
	return bodyString
}

// parse base domain from dns config. format is like $clustername.$basedomain
func getBaseDomain(oc *exutil.CLI) string {
	str, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns/cluster", `-ojsonpath={.spec.baseDomain}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return str
}

// Return  the API server FQDN. format is like api.$clustername.$basedomain
func getApiServerFQDN(oc *exutil.CLI) string {
	return fmt.Sprintf("api.%s", getBaseDomain(oc))
}

// isTechPreviewNoUpgrade checks if a cluster is a TechPreviewNoUpgrade cluster
func isTechPreviewNoUpgrade(oc *exutil.CLI) bool {
	featureGate, err := oc.AdminConfigClient().ConfigV1().FeatureGates().Get(context.Background(), "cluster", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		e2e.Failf("could not retrieve feature-gate: %v", err)
	}

	return featureGate.Spec.FeatureSet == configv1.TechPreviewNoUpgrade
}

// IsIPv4 check if the string is an IPv4 address.
func isIPv4(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ".")
}

// IsIPv6 check if the string is an IPv6 address.
func isIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ":")
}
