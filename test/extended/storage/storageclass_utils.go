package storage

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type storageClass struct {
	name              string
	template          string
	provisioner       string
	reclaimPolicy     string
	volumeBindingMode string
	negativeTest      bool
	parameters        map[string]interface{}
}

// function option mode to change the default value of storageclass parameters, e.g. name, provisioner, reclaimPolicy, volumeBindingMode
type storageClassOption func(*storageClass)

// Replace the default value of storageclass name parameter
func setStorageClassName(name string) storageClassOption {
	return func(this *storageClass) {
		this.name = name
	}
}

// Replace the default value of storageclass template parameter
func setStorageClassTemplate(template string) storageClassOption {
	return func(this *storageClass) {
		splitResult := strings.Split(template, "/")
		if cloudProvider == "ibmcloud" && splitResult[len(splitResult)-1] == "storageclass-template.yaml" {
			splitResult[len(splitResult)-1] = "ibm-storageclass-template.yaml"
			this.template = strings.Replace(strings.Trim(fmt.Sprint(splitResult), "[]"), " ", "/", -1)
		} else {
			this.template = template
		}
	}
}

// Replace the default value of storageclass provisioner parameter
func setStorageClassProvisioner(provisioner string) storageClassOption {
	return func(this *storageClass) {
		this.provisioner = provisioner
	}
}

// Replace the default value of storageclass reclaimPolicy parameter
func setStorageClassReclaimPolicy(reclaimPolicy string) storageClassOption {
	return func(this *storageClass) {
		this.reclaimPolicy = reclaimPolicy
	}
}

// Replace the default value of storageclass volumeBindingMode parameter
func setStorageClassVolumeBindingMode(volumeBindingMode string) storageClassOption {
	return func(this *storageClass) {
		this.volumeBindingMode = volumeBindingMode
	}
}

// Create a new customized storageclass object
func newStorageClass(opts ...storageClassOption) storageClass {
	defaultStorageClass := storageClass{
		name:              "mystorageclass-" + getRandomString(),
		template:          "storageclass-template.yaml",
		provisioner:       "ebs.csi.aws.com",
		reclaimPolicy:     "Delete",
		volumeBindingMode: "WaitForFirstConsumer",
		parameters:        make(map[string]interface{}, 10),
	}

	for _, o := range opts {
		o(&defaultStorageClass)
	}

	return defaultStorageClass
}

// Create a new customized storageclass
func (sc *storageClass) create(oc *exutil.CLI) {
	// Currently AWS Outpost only support gp2 type volumes
	// https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/docs/parameters.md
	if isAwsOutpostCluster(oc) {
		gp2VolumeTypeParameter := map[string]string{"type": "gp2"}
		sc.createWithExtraParameters(oc, map[string]interface{}{"parameters": gp2VolumeTypeParameter})
	} else {
		err := applyResourceFromTemplateAsAdmin(oc, "--ignore-unknown-parameters=true", "-f", sc.template, "-p", "SCNAME="+sc.name, "RECLAIMPOLICY="+sc.reclaimPolicy,
			"PROVISIONER="+sc.provisioner, "VOLUMEBINDINGMODE="+sc.volumeBindingMode)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// Delete Specified storageclass
func (sc *storageClass) deleteAsAdmin(oc *exutil.CLI) {
	oc.AsAdmin().WithoutNamespace().Run("delete").Args("sc", sc.name, "--ignore-not-found").Execute()
}

// Create a new customized storageclass with extra parameters
func (sc *storageClass) createWithExtraParameters(oc *exutil.CLI, extraParameters map[string]interface{}) error {
	sc.getParametersFromTemplate()
	// Currently AWS Outpost only support gp2 type volumes
	// https://github.com/kubernetes-sigs/aws-ebs-csi-driver/blob/master/docs/parameters.md
	if isAwsOutpostCluster(oc) {
		sc.parameters["type"] = "gp2"
	}
	if _, ok := extraParameters["parameters"]; ok || len(sc.parameters) > 0 {
		parametersByte, err := json.Marshal(extraParameters["parameters"])
		o.Expect(err).NotTo(o.HaveOccurred())
		finalParameters := make(map[string]interface{}, 10)
		err = json.Unmarshal(parametersByte, &finalParameters)
		o.Expect(err).NotTo(o.HaveOccurred())
		finalParameters = mergeMaps(sc.parameters, finalParameters)
		debugLogf("StorageClass/%s final parameter is %v", sc.name, finalParameters)
		extraParameters["parameters"] = finalParameters
	}
	err := applyResourceFromTemplateWithExtraParametersAsAdmin(oc, extraParameters, "--ignore-unknown-parameters=true", "-f", sc.template, "-p",
		"SCNAME="+sc.name, "RECLAIMPOLICY="+sc.reclaimPolicy, "PROVISIONER="+sc.provisioner, "VOLUMEBINDINGMODE="+sc.volumeBindingMode)
	if sc.negativeTest {
		o.Expect(err).Should(o.HaveOccurred())
		return err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return err
}

// GetFieldByJSONPath gets its field value by JSONPath
func (sc *storageClass) getFieldByJSONPath(oc *exutil.CLI, JSONPath string) string {
	fieldValue, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("storageclass/"+sc.name, "-o", "jsonpath="+JSONPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return fieldValue
}

// getParametersFromTemplate gets the storageClass parameters from yaml template
func (sc *storageClass) getParametersFromTemplate() *storageClass {
	output, err := ioutil.ReadFile(sc.template)
	o.Expect(err).NotTo(o.HaveOccurred())
	output, err = yaml.YAMLToJSON([]byte(output))
	o.Expect(err).NotTo(o.HaveOccurred())
	if gjson.Get(string(output), `objects.0.parameters`).Exists() {
		err = json.Unmarshal([]byte(gjson.Get(string(output), `objects.0.parameters`).String()), &sc.parameters)
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	debugLogf(`StorageClass/%s using template/%s's parameters is: "%+v"`, sc.name, sc.template, sc.parameters)
	return sc
}

// Storageclass negative test enable
func (sc *storageClass) negative() *storageClass {
	sc.negativeTest = true
	return sc
}

// Check if pre-defined storageclass exists
func preDefinedStorageclassCheck(cloudProvider string) {
	preDefinedStorageclassMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage", "config"), "pre-defined-storageclass.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	supportPlatformsBool := gjson.GetBytes(preDefinedStorageclassMatrix, "platforms.#(name="+cloudProvider+").storageclass|@flatten").Exists()
	if !supportPlatformsBool {
		g.Skip("Skip for no pre-defined storageclass on " + cloudProvider + "!!! Or please check the test configuration")
	}
}

// Get default storage class name from pre-defined-storageclass matrix
func getClusterDefaultStorageclassByPlatform(cloudProvider string) string {
	preDefinedStorageclassMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage", "config"), "pre-defined-storageclass.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	sc := gjson.GetBytes(preDefinedStorageclassMatrix, "platforms.#(name="+cloudProvider+").storageclass.default_sc").String()
	e2e.Logf("The default storageclass is: %s.", sc)
	return sc
}

// Get pre-defined storage class name list from pre-defined-storageclass matrix
func getClusterPreDefinedStorageclassByPlatform(cloudProvider string) []string {
	preDefinedStorageclassMatrix, err := ioutil.ReadFile(filepath.Join(exutil.FixturePath("testdata", "storage", "config"), "pre-defined-storageclass.json"))
	o.Expect(err).NotTo(o.HaveOccurred())
	preDefinedStorageclass := []string{}
	sc := gjson.GetBytes(preDefinedStorageclassMatrix, "platforms.#(name="+cloudProvider+").storageclass.pre_defined_sc").Array()
	for _, v := range sc {
		preDefinedStorageclass = append(preDefinedStorageclass, v.Str)
	}
	return preDefinedStorageclass
}

// check storageclass exist in given waitting time
func checkStorageclassExists(oc *exutil.CLI, sc string) {
	err := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", sc, "-o", "jsonpath={.metadata.name}").Output()
		if err1 != nil {
			e2e.Logf("Get error to get the storageclass %v", sc)
			return false, nil
		}
		if output != sc {
			return false, nil
		}
		e2e.Logf("storageClass %s is installed successfully\n", sc)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Could not find the storageclass %v", sc))
}

// Check if given storageclass is default storageclass
func checkDefaultStorageclass(oc *exutil.CLI, sc string) bool {
	stat, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", sc, "-o", "jsonpath={.metadata.annotations.storageclass\\.kubernetes\\.io/is-default-class}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	debugLogf("oc get sc:\n%s", output)
	return strings.EqualFold(stat, "true")
}

// Get reclaimPolicy by storageclass name
func getReclaimPolicyByStorageClassName(oc *exutil.CLI, storageClassName string) string {
	reclaimPolicy, err := oc.WithoutNamespace().Run("get").Args("sc", storageClassName, "-o", "jsonpath={.reclaimPolicy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.ToLower(reclaimPolicy)
}

// Get volumeBindingMode by storageclass name
func getVolumeBindingModeByStorageClassName(oc *exutil.CLI, storageClassName string) string {
	volumeBindingMode, err := oc.WithoutNamespace().Run("get").Args("sc", storageClassName, "-o", "jsonpath={.volumeBindingMode}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return strings.ToLower(volumeBindingMode)
}

// Get the fileSystemId from sc
func getFsIDFromStorageClass(oc *exutil.CLI, scName string) string {
	fsID, err := oc.WithoutNamespace().Run("get").Args("sc", scName, "-o", "jsonpath={.parameters.fileSystemId}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The filesystem Id is %s", fsID)
	return fsID
}

// Get the gidValue from sc
func getGidRangeStartValueFromStorageClass(oc *exutil.CLI, scName string) (int, error) {
	gidStartValue, err := oc.WithoutNamespace().Run("get").Args("sc", scName, "-o", "jsonpath={.parameters.gidRangeStart}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The gidRangeStart value is %s", gidStartValue)
	gidStartIntValue, err := strconv.Atoi(gidStartValue)
	if err != nil {
		e2e.Logf("Failed to convert with error %v\n", err)
		return gidStartIntValue, err
	}
	return gidStartIntValue, nil
}

// Define CSI Driver Provisioners const
const (
	ebsCsiDriverProvisioner string = "ebs.csi.aws.com"
	efsCsiDriverProvisioner string = "efs.csi.aws.com"
)

// Generate storageClass parameters by volume type
func gererateCsiScExtraParametersByVolType(oc *exutil.CLI, csiProvisioner string, volumeType string) map[string]interface{} {
	var (
		storageClassParameters map[string]string
		extraParameters        map[string]interface{}
	)
	switch csiProvisioner {
	case ebsCsiDriverProvisioner:
		// aws-ebs-csi
		// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-volume-types.html
		// io1, io2, gp2, gp3, sc1, st1,standard
		// Default is gp3 if not set the volumeType in storageClass parameters
		storageClassParameters = map[string]string{
			"type": volumeType}
		// I/O operations per second per GiB. Required when io1 or io2 volume type is specified.
		if volumeType == "io1" || volumeType == "io2" {
			storageClassParameters["iopsPerGB"] = "50"
		}
	// aws-efs-csi
	// https://github.com/kubernetes-sigs/aws-efs-csi-driver
	case efsCsiDriverProvisioner:
		fsID := getFsIDFromStorageClass(oc, getPresetStorageClassNameByProvisioner(oc, cloudProvider, "efs.csi.aws.com"))
		storageClassParameters = map[string]string{
			"provisioningMode": volumeType,
			"fileSystemId":     fsID,
			"directoryPerms":   "700",
		}
	default:
		storageClassParameters = map[string]string{
			"type": volumeType}
	}
	extraParameters = map[string]interface{}{
		"parameters":           storageClassParameters,
		"allowVolumeExpansion": true,
	}
	return extraParameters
}
func getNetworkFromStorageClass(oc *exutil.CLI, scName string) string {
	networkID, err := oc.WithoutNamespace().Run("get").Args("sc", scName, "-o", "jsonpath={.parameters.network}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The network Id is %s", networkID)
	return networkID
}

// Set specified storage class as a default one
func setSpecifiedStorageClassAsDefault(oc *exutil.CLI, scName string) {
	patchResourceAsAdmin(oc, "", "sc/"+scName, `{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}`, "merge")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", scName, "-o=jsonpath={.metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.Equal("true"))
	e2e.Logf("Changed the storage class %v to be default one successfully", scName)

}

// Set specified storage class as a non-default one
func setSpecifiedStorageClassAsNonDefault(oc *exutil.CLI, scName string) {
	patchResourceAsAdmin(oc, "", "sc/"+scName, `{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}`, "merge")
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", scName, "-o=jsonpath={.metadata.annotations.storageclass\\.kubernetes\\.io\\/is-default-class}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.Equal("false"))
	e2e.Logf("Changed the storage class %v to be non-default one successfully", scName)
}
