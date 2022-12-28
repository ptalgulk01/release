package util

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

//This will check if operator deployment/daemonset is created sucessfully
//will update sro test case to use this common utils later.
//example:
//WaitOprResourceReady(oc, deployment, deployment-name, namespace, true, true)
//WaitOprResourceReady(oc, statefulset, statefulset-name, namespace, false, false)
//WaitOprResourceReady(oc, daemonset, daemonset-name, namespace, false, false)
//If islongduration is true, it will sleep 720s, otherwise 180s
//If excludewinnode is true, skip checking windows nodes daemonset status
//For daemonset or deployment have random name, getting name before use this function

// WaitOprResourceReady used for checking if deployment/daemonset/statefulset is ready
func WaitOprResourceReady(oc *CLI, kind, name, namespace string, islongduration bool, excludewinnode bool) {
	//If islongduration is true, it will sleep 720s, otherwise 180s
	var timeDurationSec int
	if islongduration {
		timeDurationSec = 720
	} else {
		timeDurationSec = 360
	}

	waitErr := wait.Poll(20*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {
		var (
			kindNames  string
			err        error
			isCreated  bool
			desiredNum string
			readyNum   string
		)

		//Check if deployment/daemonset/statefulset is created.
		switch kind {
		case "deployment", "statefulset":
			kindNames, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, name, "-n", namespace, "-oname").Output()
			if strings.Contains(kindNames, "NotFound") || strings.Contains(kindNames, "No resources") || len(kindNames) == 0 || err != nil {
				isCreated = false
			} else {
				//deployment/statefulset has been created, but not running, need to compare .status.readyReplicas and  in .status.replicas
				isCreated = true
				desiredNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.readyReplicas}").Output()
				readyNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.replicas}").Output()
			}
		case "daemonset":
			kindNames, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, name, "-n", namespace, "-oname").Output()
			e2e.Logf("daemonset name is:" + kindNames)
			if len(kindNames) == 0 || err != nil {
				isCreated = false
			} else {
				//daemonset/statefulset has been created, but not running, need to compare .status.desiredNumberScheduled and .status.numberReady}
				//if the two value is equal, set output="has successfully progressed"
				isCreated = true
				desiredNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.desiredNumberScheduled}").Output()
				//If there are windows worker nodes, the desired daemonset should be linux node's num
				_, WindowsNodeNum := CountNodeNumByOS(oc)
				if WindowsNodeNum > 0 && excludewinnode {

					//Exclude windows nodes
					e2e.Logf("%v desiredNum is: %v", kindNames, desiredNum)
					desiredLinuxWorkerNum, _ := strconv.Atoi(desiredNum)
					e2e.Logf("desiredlinuxworkerNum is:%v", desiredLinuxWorkerNum)
					desiredNum = strconv.Itoa(desiredLinuxWorkerNum - WindowsNodeNum)
				}
				readyNum, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args(kindNames, "-n", namespace, "-o=jsonpath={.status.numberReady}").Output()
			}
		default:
			e2e.Logf("Invalid Resource Type")
		}

		e2e.Logf("desiredNum is: " + desiredNum + " readyNum is: " + readyNum)
		//daemonset/deloyment has been created, but not running, need to compare desiredNum and readynum
		//if isCreate is true and the two value is equal, the pod is ready
		if isCreated && len(kindNames) != 0 && desiredNum == readyNum {
			e2e.Logf("The %v is successfully progressed and running normally", kindNames)
			return true, nil
		}
		e2e.Logf("The %v is not ready or running normally", kindNames)
		return false, nil

	})
	AssertWaitPollNoErr(waitErr, fmt.Sprintf("the pod of %v is not running", name))
}

// IsNodeLabeledByNFD Check if NFD Installed base on the cluster labels
func IsNodeLabeledByNFD(oc *CLI) bool {
	workNode, _ := GetFirstWorkerNode(oc)
	Output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", workNode, "-o", "jsonpath='{.metadata.annotations}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(Output, "nfd.node.kubernetes.io/feature-labels") {
		e2e.Logf("NFD installed on openshift container platform and labeled nodes")
		return true
	}
	return false
}

// CountNodeNumByOS used for count how many worker node by windows or linux
func CountNodeNumByOS(oc *CLI) (linuxNum int, windowsNum int) {
	//Count how many windows node and linux node
	linuxNodeNames, err := GetAllNodesbyOSType(oc, "linux")
	o.Expect(err).NotTo(o.HaveOccurred())
	windowsNodeNames, err := GetAllNodesbyOSType(oc, "windows")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("linuxNodeNames is:%v", linuxNodeNames[:])
	e2e.Logf("windowsNodeNames is:%v", windowsNodeNames[:])
	linuxNum = len(linuxNodeNames)
	windowsNum = len(windowsNodeNames)
	e2e.Logf("Linux node is:%v, windows node is %v", linuxNum, windowsNum)
	return linuxNum, windowsNum
}

// GetFirstLinuxMachineSets used for getting first linux worker nodes name
func GetFirstLinuxMachineSets(oc *CLI) string {
	machinesets, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, "-o=jsonpath={.items[*].metadata.name}", "-n", "openshift-machine-api").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	machinesetsArray := strings.Split(machinesets, " ")
	//Remove windows machineset
	for i, machineset := range machinesetsArray {
		if machineset == "windows" {
			machinesetsArray = append(machinesetsArray[:i], machinesetsArray[i+1:]...)
			e2e.Logf("%T,%v", machinesets, machinesets)
		}
	}
	return machinesetsArray[0]
}

// InstallNFD attempts to install the Node Feature Discovery operator and verify that it is running
func InstallNFD(oc *CLI, nfdNamespace string) {
	var (
		nfdNamespaceFile     = FixturePath("testdata", "psap", "nfd", "nfd-namespace.yaml")
		nfdOperatorgroupFile = FixturePath("testdata", "psap", "nfd", "nfd-operatorgroup.yaml")
		nfdSubFile           = FixturePath("testdata", "psap", "nfd", "nfd-sub.yaml")
	)
	// check if NFD namespace already exists
	nsName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespace", nfdNamespace).Output()
	// if namespace exists, check if NFD is installed - exit if it is, continue with installation otherwise
	// if an error is thrown, namespace does not exist, create and continue with installation
	if strings.Contains(nsName, "NotFound") || strings.Contains(nsName, "No resources") || err != nil {
		e2e.Logf("NFD namespace not found - creating namespace and installing NFD ...")
		CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", nfdNamespaceFile)
	} else {
		e2e.Logf("NFD namespace found - checking if NFD is installed ...")
	}

	ogName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("OperatorGroup", "openshift-nfd", "-n", nfdNamespace).Output()
	if strings.Contains(ogName, "NotFound") || strings.Contains(ogName, "No resources") || err != nil {
		// create NFD operator group from template
		ApplyNsResourceFromTemplate(oc, nfdNamespace, "--ignore-unknown-parameters=true", "-f", nfdOperatorgroupFile)
	} else {
		e2e.Logf("NFD operatorgroup found - continue to check subscription ...")
	}

	// get default channel and create subscription from template
	channel, err := GetOperatorPKGManifestDefaultChannel(oc, "nfd", "openshift-marketplace")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Channel: %v", channel)
	// get default channel and create subscription from template
	source, err := GetOperatorPKGManifestSource(oc, "nfd", "openshift-marketplace")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Source: %v", source)

	subName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Subscription", "-n", nfdNamespace).Output()
	if strings.Contains(subName, "NotFound") || strings.Contains(subName, "No resources") || !strings.Contains(subName, "nfd") || err != nil {
		// create NFD operator group from template
		ApplyNsResourceFromTemplate(oc, nfdNamespace, "--ignore-unknown-parameters=true", "-f", nfdSubFile, "-p", "CHANNEL="+channel, "SOURCE="+source)
	} else {
		e2e.Logf("NFD subscription found - continue to check pod status ...")
	}

	//Wait for NFD controller manager is ready
	WaitOprResourceReady(oc, "deployment", "nfd-controller-manager", nfdNamespace, false, false)

}

// CreateNFDInstance used for create NFD Instance in different namespace
func CreateNFDInstance(oc *CLI, namespace string) {

	var (
		nfdInstanceFile = FixturePath("testdata", "psap", "nfd", "nfd-instance.yaml")
	)
	// get cluster version and create NFD instance from template
	clusterVersion, _, err := GetClusterVersion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Cluster Version: %v", clusterVersion)

	nfdinstanceName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("NodeFeatureDiscovery", "nfd-instance", "-n", namespace).Output()
	e2e.Logf("NFD Instance is: %v", nfdinstanceName)
	if strings.Contains(nfdinstanceName, "NotFound") || strings.Contains(nfdinstanceName, "No resources") || err != nil {
		// create NFD operator group from template
		nfdInstanceImage := GetNFDInstanceImage(oc, namespace)
		e2e.Logf("NFD instance image name: %v", nfdInstanceImage)
		o.Expect(nfdInstanceImage).NotTo(o.BeEmpty())
		ApplyNsResourceFromTemplate(oc, namespace, "--ignore-unknown-parameters=true", "-f", nfdInstanceFile, "-p", "IMAGE="+nfdInstanceImage, "NAMESPACE="+namespace)
	} else {
		e2e.Logf("NFD instance found - continue to check pod status ...")
	}

	//wait for NFD master and worker is ready
	WaitOprResourceReady(oc, "daemonset", "nfd-master", namespace, false, false)
	WaitOprResourceReady(oc, "daemonset", "nfd-worker", namespace, false, true)
}

// GetNFDVersionbyPackageManifest return NFD version
func GetNFDVersionbyPackageManifest(oc *CLI, namespace string) string {
	nfdVersionOrigin, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "nfd", "-n", namespace, "-ojsonpath={.status.channels[*].currentCSVDesc.version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(nfdVersionOrigin).NotTo(o.BeEmpty())
	nfdVersionArr := strings.Split(nfdVersionOrigin, ".")
	nfdVersion := nfdVersionArr[0] + "." + nfdVersionArr[1]
	return nfdVersion
}

// GetNFDInstanceImage return correct image name in manifest channel
func GetNFDInstanceImage(oc *CLI, namespace string) string {
	var nfdInstanceImage string
	nfdInstanceImageStr, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "nfd", "-n", namespace, "-ojsonpath={.status.channels[*].currentCSVDesc.relatedImages}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(nfdInstanceImageStr).NotTo(o.BeEmpty())

	strTmp1 := strings.ReplaceAll(nfdInstanceImageStr, "[", ",")
	strTmp2 := strings.ReplaceAll(strTmp1, "]", ",")
	strTmp3 := strings.ReplaceAll(strTmp2, `"`, "")

	nfdInstanceImageArr := strings.Split(strTmp3, ",")

	//using the last one image if mulitiple image was found
	for i := 0; i < len(nfdInstanceImageArr); i++ {
		if strings.Contains(nfdInstanceImageArr[i], "node-feature-discovery") {
			nfdInstanceImage = nfdInstanceImageArr[i]
		}
	}
	e2e.Logf("NFD instance image name: %v", nfdInstanceImage)
	return nfdInstanceImage
}

// GetOperatorPKGManifestSource used for getting operator Packagemanifest source name
func GetOperatorPKGManifestSource(oc *CLI, pkgManifestName, namespace string) (string, error) {
	catalogSourceNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "-n", namespace, "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(catalogSourceNames, "qe-app-registry") || err != nil {
		//If the catalogsource qe-app-registry exist, prefer to use qe-app-registry, not use redhat-operators or certificate-operator ...
		return "qe-app-registry", nil
	}
	soureName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", pkgManifestName, "-n", namespace, "-o=jsonpath={.status.catalogSource}").Output()
	return soureName, err
}

// GetOperatorPKGManifestDefaultChannel to getting operator Packagemanifest default channel
func GetOperatorPKGManifestDefaultChannel(oc *CLI, pkgManifestName, namespace string) (string, error) {
	channel, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", pkgManifestName, "-n", namespace, "-o", "jsonpath={.status.defaultChannel}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return channel, err
}

// ApplyOperatorResourceByYaml - It's not a template yaml file, the yaml shouldn't include namespace, we specify namespace by parameter.
func ApplyOperatorResourceByYaml(oc *CLI, namespace string, yamlfile string) {
	if len(namespace) == 0 {
		//Create cluster-wide resource
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", yamlfile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		//Create namespace-wide resource
		err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", yamlfile, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CleanupOperatorResourceByYaml - It's not a template yaml file, the yaml shouldn't include namespace, we specify namespace by parameter.
func CleanupOperatorResourceByYaml(oc *CLI, namespace string, yamlfile string) {
	if len(namespace) == 0 {
		//Delete cluster-wide resource
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", yamlfile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		//Delete namespace-wide resource
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-f", yamlfile, "-n", namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// AssertOprPodLogsbyFilterWithDuration used for truncting pods logs by filter
func AssertOprPodLogsbyFilterWithDuration(oc *CLI, podName string, namespace string, filter string, timeDurationSec int, minimalMatch int) {
	podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podList).To(o.ContainSubstring(podName))

	e2e.Logf("Got pods list as below: \n" + podList)
	//Filter pod name base on deployment name
	regexpoprname, _ := regexp.Compile(".*" + podName + ".*")
	podListArry := regexpoprname.FindAllString(podList, -1)

	podListSize := len(podListArry)
	for i := 0; i < podListSize; i++ {
		//Check the log files until finding the keywords by filter
		waitErr := wait.Poll(15*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {
			e2e.Logf("Verify the logs on %v", podListArry[i])
			output, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podListArry[i], "-n", namespace).Output()
			regexpstr, _ := regexp.Compile(".*" + filter + ".*")
			loglines := regexpstr.FindAllString(output, -1)
			matchNumber := len(loglines)
			if strings.Contains(output, filter) && matchNumber >= minimalMatch {
				//Print the last entry log
				matchNumber = matchNumber - 1
				e2e.Logf("The result is: %v", loglines[matchNumber])
				return true, nil
			}
			e2e.Logf("Can not find the key words in pod logs by: %v", filter)
			return false, nil
		})
		AssertWaitPollNoErr(waitErr, fmt.Sprintf("the pod of %v is not running", podName))
	}
}

// AssertOprPodLogsbyFilter trunct pods logs by filter
func AssertOprPodLogsbyFilter(oc *CLI, podName string, namespace string, filter string, minimalMatch int) bool {
	podList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", namespace, "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(podList).To(o.ContainSubstring(podName))

	e2e.Logf("Got pods list as below: \n" + podList)
	//Filter pod name base on deployment name
	regexpoprname, _ := regexp.Compile(".*" + podName + ".*")
	podListArry := regexpoprname.FindAllString(podList, -1)

	podListSize := len(podListArry)
	var isMatch bool
	for i := 0; i < podListSize; i++ {
		e2e.Logf("Verify the logs on %v", podListArry[i])
		output, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podListArry[i], "-n", namespace).Output()
		regexpstr, _ := regexp.Compile(".*" + filter + ".*")
		loglines := regexpstr.FindAllString(output, -1)
		matchNumber := len(loglines)
		if strings.Contains(output, filter) && matchNumber >= minimalMatch {
			//Print the last entry log
			matchNumber = matchNumber - 1
			e2e.Logf("The result is: %v", loglines[matchNumber])
			isMatch = true
		} else {
			e2e.Logf("Can not find the key words in pod logs by: %v", filter)
			isMatch = false
		}
	}
	return isMatch
}

// WaitForNoPodsAvailableByKind used for checking no pods in a certain namespace
func WaitForNoPodsAvailableByKind(oc *CLI, kind string, name string, namespace string) {
	err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		kindNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, name, "-n", namespace, "-oname").Output()
		if strings.Contains(kindNames, "NotFound") || strings.Contains(kindNames, "No resources") || len(kindNames) == 0 || err != nil {
			//Check if the new profiles name applied on a node
			e2e.Logf("All the pod has been terminated:\n %v", kindNames)
			return true, nil
		}
		e2e.Logf("The pod is still terminating, waiting for a while: \n%v", kindNames)
		return false, nil
	})
	AssertWaitPollNoErr(err, "No pod was found ...")
}

// InstallPAO attempts to install the Performance Add-On operator and verify that it is running
func InstallPAO(oc *CLI, paoNamespace string) {
	var (
		paoNamespaceFile     = FixturePath("testdata", "psap", "pao", "pao-namespace.yaml")
		paoOperatorgroupFile = FixturePath("testdata", "psap", "pao", "pao-operatorgroup.yaml")
		paoSubFile           = FixturePath("testdata", "psap", "pao", "pao-subscription.yaml")
	)
	// check if PAO namespace already exists
	nsName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("namespace", paoNamespace).Output()
	// if namespace exists, check if PAO is installed - exit if it is, continue with installation otherwise
	// if an error is thrown, namespace does not exist, create and continue with installation
	if strings.Contains(nsName, "NotFound") || strings.Contains(nsName, "No resources") || err != nil {
		e2e.Logf("PAO namespace not found - creating namespace and installing PAO ...")
		CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", paoNamespaceFile)
	} else {
		e2e.Logf("PAO namespace found - checking if PAO is installed ...")
	}

	ogName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("OperatorGroup", "openshift-performance-addon-operator", "-n", paoNamespace).Output()
	if strings.Contains(ogName, "NotFound") || strings.Contains(ogName, "No resources") || err != nil {
		// create PAO operator group from template
		ApplyNsResourceFromTemplate(oc, paoNamespace, "--ignore-unknown-parameters=true", "-f", paoOperatorgroupFile)
	} else {
		e2e.Logf("PAO operatorgroup found - continue to check subscription ...")
	}

	// get default channel and create subscription from template
	channel, err := GetOperatorPKGManifestDefaultChannel(oc, "performance-addon-operator", "openshift-marketplace")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Channel: %v", channel)
	// get default channel and create subscription from template
	source, err := GetOperatorPKGManifestSource(oc, "performance-addon-operator", "openshift-marketplace")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Source: %v", source)

	subName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("Subscription", "-n", paoNamespace).Output()
	if strings.Contains(subName, "NotFound") || strings.Contains(subName, "No resources") || !strings.Contains(subName, "performance-operator") || err != nil {
		// create PAO operator group from template
		ApplyNsResourceFromTemplate(oc, paoNamespace, "--ignore-unknown-parameters=true", "-f", paoSubFile, "-p", "CHANNEL="+channel, "SOURCE="+source)
	} else {
		e2e.Logf("PAO subscription found - continue to check pod status ...")
	}

	//Wait for PAO controller manager is ready
	WaitOprResourceReady(oc, "deployment", "performance-operator", paoNamespace, false, false)
}

// IsPAOInstalled used for deploying Performance Add-on Operator
func IsPAOInstalled(oc *CLI) bool {
	var isInstalled bool
	deployments, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-A").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(deployments, "performance-operator") {
		isInstalled = true
	} else {
		e2e.Logf("PAO doesn't installed - will install pao ...")
		isInstalled = false
	}
	return isInstalled
}

// IsPAOInOperatorHub used for checking if PAO exist in OperatorHub
func IsPAOInOperatorHub(oc *CLI) bool {
	var havePAO bool
	packagemanifest, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-A").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(packagemanifest, "performance-addon-operator") {
		havePAO = true
	} else {
		e2e.Logf("No PAO packagemanifet detect in operatorhub - skip ...")
		havePAO = false
	}
	return havePAO
}

// StringToBASE64 Base64 Encode
func StringToBASE64(src string) string {
	// plaintext, err := base64.StdEncoding.DecodeString(src)
	stdEnc := base64.StdEncoding.EncodeToString([]byte(src))
	return string(stdEnc)
}

// BASE64DecodeStr Base64 Decode
func BASE64DecodeStr(src string) string {
	plaintext, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return ""
	}
	return string(plaintext)
}

// CreateMachinesetbyInstanceType used to create a machineset with specified machineset name and instance type
func CreateMachinesetbyInstanceType(oc *CLI, machinesetName string, instanceType string) {
	// Get existing machinesets in cluster
	ocGetMachineset, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, "-n", "openshift-machine-api", "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(ocGetMachineset).NotTo(o.BeEmpty())
	e2e.Logf("Existing machinesets:\n%v", ocGetMachineset)

	// Get name of first machineset in existing machineset list
	firstMachinesetName := GetFirstLinuxMachineSets(oc)
	o.Expect(firstMachinesetName).NotTo(o.BeEmpty())
	e2e.Logf("Got %v from machineset list", firstMachinesetName)

	machinesetYamlOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(MapiMachineset, firstMachinesetName, "-n", "openshift-machine-api", "-oyaml").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(machinesetYamlOutput).NotTo(o.BeEmpty())

	//Create machinset by specifying a machineset name
	regMachineSet := regexp.MustCompile(firstMachinesetName)
	newMachinesetYaml := regMachineSet.ReplaceAllString(machinesetYamlOutput, machinesetName)

	//Change instanceType to g4dn.xlarge
	regInstanceType := regexp.MustCompile(`instanceType:.*`)
	newInstanceType := "instanceType: " + instanceType
	newMachinesetYaml = regInstanceType.ReplaceAllString(newMachinesetYaml, newInstanceType)

	//Make sure the replicas is 1
	regReplicas := regexp.MustCompile(`replicas:.*`)
	replicasNum := "replicas: 1"
	newMachinesetYaml = regReplicas.ReplaceAllString(newMachinesetYaml, replicasNum)

	machinesetNewB := []byte(newMachinesetYaml)

	newMachinesetFileName := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+machinesetName+"-new.yaml")
	defer os.RemoveAll(newMachinesetFileName)
	err = ioutil.WriteFile(newMachinesetFileName, machinesetNewB, 0o644)
	o.Expect(err).NotTo(o.HaveOccurred())
	ApplyOperatorResourceByYaml(oc, "openshift-machine-api", newMachinesetFileName)
}

// IsMachineSetExist check if machineset exist in OCP
func IsMachineSetExist(oc *CLI) bool {

	haveMachineSet := true
	Output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", "-n", "openshift-machine-api").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(Output).NotTo(o.BeEmpty())
	if strings.Contains(Output, "No resources found") {
		haveMachineSet = false
	}
	return haveMachineSet
}

// GetMachineSetInstanceType used to get first machineset instance type
func GetMachineSetInstanceType(oc *CLI) string {
	var (
		instanceType string
		err          error
	)
	firstMachinesetName := GetFirstLinuxMachineSets(oc)
	e2e.Logf("Got %v from machineset list", firstMachinesetName)
	iaasPlatform := CheckPlatform(oc)
	if iaasPlatform == "aws" {
		instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", firstMachinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.template.spec.providerSpec.value.instanceType}").Output()
	} else if iaasPlatform == "azure" {
		instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", firstMachinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.template.spec.providerSpec.value.vmSize}").Output()
	} else if iaasPlatform == "gcp" {
		instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", firstMachinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.template.spec.providerSpec.value.machineType}").Output()
	} else if iaasPlatform == "ibmcloud" {
		instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", firstMachinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.template.spec.providerSpec.value.profile}").Output()
	} else if iaasPlatform == "alibabacloud" {
		instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", firstMachinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.template.spec.providerSpec.value.instanceType}").Output()
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(instanceType).NotTo(o.BeEmpty())
	return instanceType
}

// GetNodeNameByMachineset used for get
func GetNodeNameByMachineset(oc *CLI, machinesetName string) string {

	machinesetLabels, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machineset", machinesetName, "-n", "openshift-machine-api", "-ojsonpath={.spec.selector.matchLabels.machine\\.openshift\\.io/cluster-api-machineset}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(machinesetLabels).NotTo(o.BeEmpty())
	machineName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("machine", "-l", "machine.openshift.io/cluster-api-machineset="+machinesetLabels, "-n", "openshift-machine-api", "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(machineName).NotTo(o.BeEmpty())
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(machineName, "-n", "openshift-machine-api", "-ojsonpath={.status.nodeRef.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(nodeName).NotTo(o.BeEmpty())
	return nodeName
}

// AssertIfMCPChangesAppliedByName checks the MCP of a given oc client and determines if the machine counts are as expected
func AssertIfMCPChangesAppliedByName(oc *CLI, mcpName string, timeDurationMin int) {
	err := wait.Poll(1*time.Minute, time.Duration(timeDurationMin)*time.Minute, func() (bool, error) {
		var (
			mcpMachineCount         string
			mcpReadyMachineCount    string
			mcpUpdatedMachineCount  string
			mcpDegradedMachineCount string
			mcpUpdatingStatus       string
			mcpUpdatedStatus        string
			err                     error
		)

		mcpUpdatingStatus, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, `-ojsonpath='{.status.conditions[?(@.type=="Updating")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcpUpdatingStatus).NotTo(o.BeEmpty())
		mcpUpdatedStatus, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, `-ojsonpath='{.status.conditions[?(@.type=="Updated")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcpUpdatedStatus).NotTo(o.BeEmpty())

		//For master node, only make sure one of master is ready.
		if strings.Contains(mcpName, "master") {
			mcpMachineCount = "1"
			//Do not check master err due to sometimes SNO can not accesss api server when server rebooted
			mcpReadyMachineCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.readyMachineCount}").Output()
			mcpUpdatedMachineCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.updatedMachineCount}").Output()
			mcpDegradedMachineCount, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.degradedMachineCount}").Output()
			if mcpMachineCount == mcpReadyMachineCount && mcpMachineCount == mcpUpdatedMachineCount && mcpDegradedMachineCount == "0" {
				e2e.Logf("MachineConfigPool [%v] checks succeeded!", mcpName)
				return true, nil
			}
		} else {
			mcpMachineCount, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.machineCount}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			mcpReadyMachineCount, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.readyMachineCount}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			mcpUpdatedMachineCount, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.updatedMachineCount}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			mcpDegradedMachineCount, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.degradedMachineCount}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(mcpUpdatingStatus, "False") && strings.Contains(mcpUpdatedStatus, "True") && mcpMachineCount == mcpReadyMachineCount && mcpMachineCount == mcpUpdatedMachineCount && mcpDegradedMachineCount == "0" {
				e2e.Logf("MachineConfigPool [%v] checks succeeded!", mcpName)
				return true, nil
			}
		}
		e2e.Logf("MachineConfigPool [%v] checks failed, the following values were found (all should be '%v'):\nmachineCount: %v\nmcpUpdatingStatus: %v\nmcpUpdatedStatus: %v\nreadyMachineCount: %v\nupdatedMachineCount: %v\nmcpDegradedMachine:%v\nRetrying...", mcpName, mcpMachineCount, mcpMachineCount, mcpUpdatingStatus, mcpUpdatedStatus, mcpReadyMachineCount, mcpUpdatedMachineCount, mcpDegradedMachineCount)
		return false, nil
	})
	AssertWaitPollNoErr(err, "MachineConfigPool checks were not successful within timeout limit")
}

// DeleteMCAndMCPByName used for checking if node return to worker machine config pool and the specified mcp is zero, then delete mc and mcp
func DeleteMCAndMCPByName(oc *CLI, mcName string, mcpName string, timeDurationMin int) {

	//Check if labeled node return back to worker mcp, then delete mc and mcp after worker mcp is ready
	e2e.Logf("Check if labeled node return back to worker mcp")
	AssertIfMCPChangesAppliedByName(oc, "worker", timeDurationMin)

	mcpNameList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(mcpNameList).NotTo(o.BeEmpty())

	if strings.Contains(mcpNameList, mcpName) {
		//Confirm if the custom machine count is 0
		mcpMachineCount, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mcp", mcpName, "-o=jsonpath={..status.machineCount}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(mcpMachineCount).NotTo(o.BeEmpty())
		if mcpMachineCount == "0" {
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("mcp", mcpName, "--ignore-not-found").Execute()
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("mc", mcName, "--ignore-not-found").Execute()
		}
	} else {
		e2e.Logf("The mcp [%v] has been deleted ...", mcpName)
	}
}

// CreateCustomNodePoolInHypershift retrun custom nodepool yaml
func CreateCustomNodePoolInHypershift(oc *CLI, cloudProvider, guestClusterName, nodePoolName, nodeCount, instanceType, clustersNS string) {

	cmdString := fmt.Sprintf("hypershift create nodepool %s --cluster-name %s --name %s --node-count %s --instance-type %s --namespace %s --render", cloudProvider, guestClusterName, nodePoolName, nodeCount, instanceType, clustersNS)
	rawOutput, err := exec.Command("bash", "-c", cmdString).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	//NTO required InPlace upgradeType
	nodePoolYaml := strings.ReplaceAll(string(rawOutput), "upgradeType: Replace", "upgradeType: InPlace")

	nodePoolNewB := []byte(nodePoolYaml)

	newNodePoolFileName := filepath.Join(e2e.TestContext.OutputDir, "openshift-psap-qe-"+nodePoolName+"-new.yaml")
	defer os.RemoveAll(newNodePoolFileName)
	err = ioutil.WriteFile(newNodePoolFileName, nodePoolNewB, 0o644)
	o.Expect(err).NotTo(o.HaveOccurred())

	nodePoolNameList, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodepool", "-n", clustersNS, "-oname").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	isMatch := strings.Contains(nodePoolNameList, nodePoolName)
	if !isMatch {
		ApplyOperatorResourceByYaml(oc, clustersNS, newNodePoolFileName)
	}
}

// AssertIfNodePoolIsReadyByName checks if the Nodepool is ready
func AssertIfNodePoolIsReadyByName(oc *CLI, nodePoolName string, timeDurationSec int, clustersNS string) {
	err := wait.Poll(20*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {
		var (
			isNodePoolReady string
			err             error
		)
		isNodePoolReady, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodepool", nodePoolName, "-n", clustersNS, `-ojsonpath='{.status.conditions[?(@.type=="Ready")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(isNodePoolReady).NotTo(o.BeEmpty())
		//For master node, only make sure one of master is ready.
		if strings.Contains(isNodePoolReady, "True") {
			return true, nil
		}
		e2e.Logf("Node Pool [%v] checks failed, the following values were found (read type should be true '%v')", nodePoolName, isNodePoolReady)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Nodepool checks were not successful within timeout limit")
}

// AssertIfNodePoolUpdatingConfigByName checks if the Nodepool is ready
func AssertIfNodePoolUpdatingConfigByName(oc *CLI, nodePoolName string, timeDurationSec int, clustersNS string) {
	err := wait.Poll(20*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {
		var (
			isNodePoolUpdatingConfig string
			err                      error
		)
		isNodePoolUpdatingConfig, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodepool", nodePoolName, "-n", clustersNS, `-ojsonpath='{.status.conditions[?(@.type=="UpdatingConfig")].status}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(isNodePoolUpdatingConfig).NotTo(o.BeEmpty())
		//For master node, only make sure one of master is ready.
		if !strings.Contains(isNodePoolUpdatingConfig, "True") {
			return true, nil
		}
		e2e.Logf("Node Pool [%v] checks failed, the following values were found (read type should be empty '%v')", nodePoolName, isNodePoolUpdatingConfig)
		return false, nil
	})
	AssertWaitPollNoErr(err, "Nodepool checks were not successful within timeout limit")
}

// IsSNOCluster will check if OCP is a single node cluster
func IsSNOCluster(oc *CLI) bool {

	topologyTypeStdOut, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "-ojsonpath={.items[*].status.infrastructureTopology}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(topologyTypeStdOut).NotTo(o.BeEmpty())
	topologyType := strings.ToLower(topologyTypeStdOut)
	return topologyType == "singlereplica"
}

// CheckAllNodepoolReadyByHostedClusterName used for checking if all nodepool is ready
// eg. CheckAllNodepoolReadyByHostedClusterName(oc, psap-qe-hcluster01,clusters,3600)
func CheckAllNodepoolReadyByHostedClusterName(oc *CLI, nodePoolName, hostedClusterNS string, timeDurationSec int) bool {

	var (
		isMatch bool
	)

	err := wait.Poll(90*time.Second, time.Duration(timeDurationSec)*time.Second, func() (bool, error) {
		nodesStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("--ignore-not-found", "np", nodePoolName, `-ojsonpath='{.status.conditions[?(@.type=="Ready")].status}'`, "--namespace", hostedClusterNS).Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		e2e.Logf("The nodepool ready status is %v ...", nodesStatus)
		if len(nodesStatus) <= 0 {
			isMatch = true
			return true, nil
		}
		return false, nil
	})
	AssertWaitPollNoErr(err, "The status of nodepool isn't ready")
	return isMatch
}
