package storage

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// Define the global cloudProvider
var cloudProvider string

//  Kubeadmin user use oc client apply yaml template
func applyResourceFromTemplateAsAdmin(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

//  Common user use oc client apply yaml template
func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

//  Get a random string of 8 byte
func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

//  Get the cloud provider type of the test environment
func getCloudProvider(oc *exutil.CLI) string {
	var (
		err_msg error
		output  string
	)
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, err_msg = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if err_msg != nil {
			e2e.Logf("Get cloudProvider *failed with* :\"%v\",wait 5 seconds retry.", err_msg)
			return false, err_msg
		}
		e2e.Logf("The test cluster cloudProvider is :\"%s\".", strings.ToLower(output))
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "Waiting for get cloudProvider timeout")
	return strings.ToLower(output)
}

//  Strings contain sub string check
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// Strings slice contains duplicate string check
func containsDuplicate(strings []string) bool {
	elemMap := make(map[string]bool)
	for _, value := range strings {
		if _, ok := elemMap[value]; ok {
			return true
		}
		elemMap[value] = true
	}
	return false
}

// Convert interface type to string
func interfaceToString(value interface{}) string {
	var key string
	if value == nil {
		return key
	}

	switch value.(type) {
	case float64:
		ft := value.(float64)
		key = strconv.FormatFloat(ft, 'f', -1, 64)
	case float32:
		ft := value.(float32)
		key = strconv.FormatFloat(float64(ft), 'f', -1, 64)
	case int:
		it := value.(int)
		key = strconv.Itoa(it)
	case uint:
		it := value.(uint)
		key = strconv.Itoa(int(it))
	case int8:
		it := value.(int8)
		key = strconv.Itoa(int(it))
	case uint8:
		it := value.(uint8)
		key = strconv.Itoa(int(it))
	case int16:
		it := value.(int16)
		key = strconv.Itoa(int(it))
	case uint16:
		it := value.(uint16)
		key = strconv.Itoa(int(it))
	case int32:
		it := value.(int32)
		key = strconv.Itoa(int(it))
	case uint32:
		it := value.(uint32)
		key = strconv.Itoa(int(it))
	case int64:
		it := value.(int64)
		key = strconv.FormatInt(it, 10)
	case uint64:
		it := value.(uint64)
		key = strconv.FormatUint(it, 10)
	case string:
		key = value.(string)
	case []byte:
		key = string(value.([]byte))
	default:
		newValue, _ := json.Marshal(value)
		key = string(newValue)
	}

	return key
}

// Json add extra parameters to jsonfile
func jsonAddExtraParametersToFile(jsonInput string, extraParameters map[string]interface{}) (string, error) {
	var (
		jsonPath string
		err      error
	)
	if interfaceToString(extraParameters["jsonPath"]) == "" {
		jsonPath = `items.0.`
	} else {
		jsonPath = interfaceToString(extraParameters["jsonPath"])
		delete(extraParameters, "jsonPath")
	}
	for extraParametersKey, extraParametersValue := range extraParameters {
		jsonInput, err = sjson.Set(jsonInput, jsonPath+extraParametersKey, extraParametersValue)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	if cloudProvider == "ibmcloud" && !gjson.Get(jsonInput, `items.0.parameters.profile`).Bool() {
		jsonInput, err = sjson.Set(jsonInput, jsonPath+"parameters.profile", "10iops-tier")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	path := filepath.Join(e2e.TestContext.OutputDir, "storageConfig"+"-"+getRandomString()+".json")
	return path, ioutil.WriteFile(path, pretty.Pretty([]byte(jsonInput)), 0644)
}

//  Kubeadmin user use oc client apply yaml template with extra parameters
func applyResourceFromTemplateWithExtraParametersAsAdmin(oc *exutil.CLI, extraParameters map[string]interface{}, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).Output()
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile, _ = jsonAddExtraParametersToFile(output, extraParameters)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	jsonOutput, _ := ioutil.ReadFile(configFile)
	debugLogf("The file content is: \n%s", jsonOutput)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// None dupulicate element slice intersect
func sliceIntersect(slice1, slice2 []string) []string {
	m := make(map[string]int)
	sliceResult := make([]string, 0)
	for _, value1 := range slice1 {
		m[value1]++
	}

	for _, value2 := range slice2 {
		appearTimes := m[value2]
		if appearTimes == 1 {
			sliceResult = append(sliceResult, value2)
		}
	}
	return sliceResult
}

// Common csi cloud provider support check
func generalCsiSupportCheck(cloudProvider string) {
	generalCsiSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportPlatformsBool := gjson.GetBytes(generalCsiSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+")|@flatten").Exists()
	e2e.Logf("%s * %v * %v", cloudProvider, gjson.GetBytes(generalCsiSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten"), supportPlatformsBool)
	if !supportPlatformsBool {
		g.Skip("Skip for non-supported cloud provider: " + cloudProvider + "!!!")
	}
}

// Get common csi provisioners by cloudplatform
func getSupportProvisionersByCloudProvider(cloudProvider string) []string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportProvisioners := []string{}
	supportProvisionersResult := gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten").Array()
	e2e.Logf("%s support provisioners are : %v", cloudProvider, supportProvisionersResult)
	for i := 0; i < len(supportProvisionersResult); i++ {
		supportProvisioners = append(supportProvisioners, gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#.name|@flatten."+strconv.Itoa(i)).String())
	}
	return supportProvisioners
}

// Get pre-defined storageclass by cloudplatform and provisioner
func getPresetStorageClassNameByProvisioner(cloudProvider string, provisioner string) string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	return gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#(name="+provisioner+").preset_scname").String()
}

// Get pre-defined volumesnapshotclass by cloudplatform and provisioner
func getPresetVolumesnapshotClassNameByProvisioner(cloudProvider string, provisioner string) string {
	csiCommonSupportMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage"), "general-csi-support-provisioners.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	return gjson.GetBytes(csiCommonSupportMatrix, "support_Matrix.platforms.#(name="+cloudProvider+").provisioners.#(name="+provisioner+").preset_vscname").String()
}

// Get the now timestamp mil second
func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}

// Log output the storage debug info
func debugLogf(format string, args ...interface{}) {
	if logLevel := os.Getenv("STORAGE_LOG_LEVEL"); logLevel == "DEBUG" {
		e2e.Logf(fmt.Sprintf(nowStamp()+": *STORAGE_DEBUG*:\n"+format, args...))
	}
}

func getZonesFromWorker(oc *exutil.CLI) []string {
	var workerZones []string
	workerNodes, err := exutil.GetClusterNodesBy(oc, "worker")
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, workerNode := range workerNodes {
		zone, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes/"+workerNode, "-o=jsonpath={.metadata.labels.failure-domain\\.beta\\.kubernetes\\.io\\/zone}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !contains(workerZones, zone) {
			workerZones = append(workerZones, zone)
		}
	}

	return workerZones
}

// Common oc CLI
//  Get the oc describe info, set namespace as "" for cluster-wide resource
func getOcDescribeInfo(oc *exutil.CLI, namespace string, resourceKind string, resourceName string) string {
	var ocDescribeInfo string
	var err error
	if namespace != "" {
		ocDescribeInfo, err = oc.WithoutNamespace().Run("describe").Args("-n", namespace, resourceKind, resourceName).Output()
	} else {
		ocDescribeInfo, err = oc.WithoutNamespace().Run("describe").Args(resourceKind, resourceName).Output()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return ocDescribeInfo
}

// Get a random number of int64 type [m,n], n > m
func getRandomNum(m int64, n int64) int64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Int63n(n-m+1) + m
}

// Restore the credential of vSphere CSI driver
func restoreVsphereCSIcredential(oc *exutil.CLI, pwdKey string, originPwd string) error {
	e2e.Logf("****** Restore the credential of vSphere CSI driver and make sure the CSO recover healthy ******")
	output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("secret/vmware-vsphere-cloud-credentials", "-n", "openshift-cluster-csi-drivers", `-p={"data":{"`+pwdKey+`":"`+originPwd+`"}}`).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("patched"))
	driverController.waitReady(oc.AsAdmin())
	// Make sure the Cluster Storage Operator recover healthy
	waitCSOhealthy(oc.AsAdmin())
	return nil
}

// Get Cluster Storage Operator specified status value
func getCSOspecifiedStatusValue(oc *exutil.CLI, specifiedStatus string) (string, error) {
	status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/storage", "-o=jsonpath={.status.conditions[?(.type=='"+specifiedStatus+"')].status}").Output()
	debugLogf("CSO \"%s\" status value is \"%s\"", specifiedStatus, status)
	return status, err
}

// Wait for Cluster Storage Operator specified status value as expected
func waitCSOspecifiedStatusValueAsExpected(oc *exutil.CLI, specifiedStatus string, expectedValue string) {
	pollErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		realValue, err := getCSOspecifiedStatusValue(oc, specifiedStatus)
		if err != nil {
			e2e.Logf("Get CSO \"%s\" status value failed of: \"%v\"", err)
			return false, err
		}
		if realValue == expectedValue {
			e2e.Logf("CSO \"%s\" status value become expected \"%s\"", specifiedStatus, expectedValue)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, fmt.Sprintf("Waiting for CSO \"%s\" status value become expected \"%s\" timeout", specifiedStatus, expectedValue))
}

// Check Cluster Storage Operator healthy
func checkCSOhealthy(oc *exutil.CLI) (bool, error) {
	// CSO healthyStatus:[degradedStatus:False, progressingStatus:False, avaiableStatus:True, upgradeableStatus:True]
	var healthyStatus = []string{"False", "False", "True", "True"}
	csoStatusJson, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/storage", "-o", "json").Output()
	degradedStatus := gjson.Get(csoStatusJson, `status.conditions.#(type=Degraded).status`).String()
	progressingStatus := gjson.Get(csoStatusJson, `status.conditions.#(type=Progressing).status`).String()
	avaiableStatus := gjson.Get(csoStatusJson, `status.conditions.#(type=Available).status`).String()
	upgradeableStatus := gjson.Get(csoStatusJson, `status.conditions.#(type=Upgradeable).status`).String()
	e2e.Logf("CSO degradedStatus:%s, progressingStatus:%v, avaiableStatus:%v, upgradeableStatus:%v", degradedStatus, progressingStatus, avaiableStatus, upgradeableStatus)
	return reflect.DeepEqual([]string{degradedStatus, progressingStatus, avaiableStatus, upgradeableStatus}, healthyStatus), err
}

// Wait for Cluster Storage Operator become healthy
func waitCSOhealthy(oc *exutil.CLI) {
	pollErr := wait.Poll(10*time.Second, 120*time.Second, func() (bool, error) {
		healthyBool, err := checkCSOhealthy(oc)
		if err != nil {
			e2e.Logf("Get CSO status failed of: \"%v\"", err)
			return false, err
		}
		if healthyBool {
			e2e.Logf("CSO status become healthy")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(pollErr, "Waiting for CSO become healthy timeout")
}

// Check CSI driver successfully installed or no
func checkCSIDriverInstalled(oc *exutil.CLI, supportProvisioners []string) bool {
	var provisioner string
	for _, provisioner = range supportProvisioners {
		csiDriver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clustercsidrivers", provisioner).Output()
		if err != nil || strings.Contains(csiDriver, "not found") {
			e2e.Logf("Error to get CSI driver:%v", err)
			return false
		}
	}
	e2e.Logf("CSI driver got successfully installed for provisioner '%s'", provisioner)
	return true
}
