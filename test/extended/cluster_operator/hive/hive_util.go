package hive

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type hiveNameSpace struct {
	name     string
	template string
}

type operatorGroup struct {
	name      string
	namespace string
	template  string
}

type subscription struct {
	name            string
	namespace       string
	channel         string
	approval        string
	operatorName    string
	sourceName      string
	sourceNamespace string
	startingCSV     string
	currentCSV      string
	installedCSV    string
	template        string
}

type hiveconfig struct {
	logLevel        string
	targetNamespace string
	template        string
}

type clusterImageSet struct {
	name         string
	releaseImage string
	template     string
}

type clusterPool struct {
	name           string
	namespace      string
	fake           string
	baseDomain     string
	imageSetRef    string
	platformType   string
	credRef        string
	region         string
	pullSecretRef  string
	size           int
	maxSize        int
	runningCount   int
	maxConcurrent  int
	hibernateAfter string
	template       string
}

type clusterClaim struct {
	name            string
	namespace       string
	clusterPoolName string
	template        string
}

type installConfig struct {
	name1      string
	namespace  string
	baseDomain string
	name2      string
	region     string
	template   string
}

type clusterDeployment struct {
	fake                string
	name                string
	namespace           string
	baseDomain          string
	clusterName         string
	platformType        string
	credRef             string
	region              string
	imageSetRef         string
	installConfigSecret string
	pullSecretRef       string
	template            string
}

type machinepool struct {
	clusterName string
	namespace   string
	template    string
}

type syncSet struct {
	name        string
	namespace   string
	cdrefname   string
	cmname      string
	cmnamespace string
	template    string
}

type objectTableRef struct {
	kind      string
	namespace string
	name      string
}

const (
	HIVE_NAMESPACE            = "hive"
	AWS_BASE_DOMAIN           = "qe.devcluster.openshift.com"
	AWS_REGION                = "us-east-2"
	OCP49_RELEASE_IMAGE       = "quay.io/openshift-release-dev/ocp-release:4.9.0-rc.6-x86_64"
	AWS_CREDS                 = "aws-creds"
	PULL_SECRET               = "pull-secret"
	CLUSTER_INSTALL_TIMEOUT   = 3600
	DEFAULT_TIMEOUT           = 120
	CLUSTER_RESUME_TIMEOUT    = 600
	CLUSTER_UNINSTALL_TIMEOUT = 1800
	CLUSTER_POOL              = "ClusterPool"
	CLUSTER_DEPLOYMENT        = "ClusterDeployment"
	CLUSTER_IMAGE_SET         = "ClusterImageSet"
	CLUSTER_CLAIM             = "ClusterClaim"
	MACHINE_POOL              = "MachinePool"
	MACHINE_SET               = "MachineSet"
	MACHINE                   = "Machine"
	SYNC_SET                  = "SyncSet"
	CONFIG_MAP                = "ConfigMap"
)

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var cfgFileJson string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "hive-resource-cfg.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		cfgFileJson = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, "fail to create config file")

	e2e.Logf("the file of resource is %s", cfgFileJson)
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", cfgFileJson).Execute()
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

//Create hive namespace if not exist
func (ns *hiveNameSpace) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ns.template, "-p", "NAME="+ns.name)
	o.Expect(err).NotTo(o.HaveOccurred())
}

//Create operatorGroup for Hive if not exist
func (og *operatorGroup) createIfNotExist(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", og.template, "-p", "NAME="+og.name, "NAMESPACE="+og.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sub *subscription) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
		"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Compare(sub.approval, "Automatic") == 0 {
		sub.findInstalledCSV(oc)
	} else {
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DEFAULT_TIMEOUT, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	}
}

//Create subscription for Hive if not exist and wait for resource is ready
func (sub *subscription) createIfNotExist(oc *exutil.CLI) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", sub.namespace).Output()
	if strings.Contains(output, "NotFound") || strings.Contains(output, "No resources") || err != nil {
		e2e.Logf("No hive subscription, Create it.")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sub.template, "-p", "NAME="+sub.name, "NAMESPACE="+sub.namespace, "CHANNEL="+sub.channel,
			"APPROVAL="+sub.approval, "OPERATORNAME="+sub.operatorName, "SOURCENAME="+sub.sourceName, "SOURCENAMESPACE="+sub.sourceNamespace, "STARTINGCSV="+sub.startingCSV)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(sub.approval, "Automatic") == 0 {
			sub.findInstalledCSV(oc)
		} else {
			newCheck("expect", "get", asAdmin, withoutNamespace, compare, "UpgradePending", ok, DEFAULT_TIMEOUT, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
		}
		//wait for pod running
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=control-plane=hive-operator", "-n",
			sub.namespace, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
	} else {
		e2e.Logf("hive subscription already exists.")
	}

}

func (sub *subscription) findInstalledCSV(oc *exutil.CLI) {
	newCheck("expect", "get", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, DEFAULT_TIMEOUT, []string{"sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)
	installedCSV := getResource(oc, asAdmin, withoutNamespace, "sub", sub.name, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}")
	o.Expect(installedCSV).NotTo(o.BeEmpty())
	if strings.Compare(sub.installedCSV, installedCSV) != 0 {
		sub.installedCSV = installedCSV
	}
	e2e.Logf("the installed CSV name is %s", sub.installedCSV)
}

func (hc *hiveconfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

//Create hivconfig if not exist and wait for resource is ready
func (hc *hiveconfig) createIfNotExist(oc *exutil.CLI) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("HiveConfig", "hive").Output()
	if strings.Contains(output, "have a resource type") || err != nil {
		e2e.Logf("No hivconfig, Create it.")
		err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", hc.template, "-p", "LOGLEVEL="+hc.logLevel, "TARGETNAMESPACE="+hc.targetNamespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		//wait for pods running
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-clustersync", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=control-plane=clustersync",
			"-n", HIVE_NAMESPACE, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=control-plane=clustersync", "-n",
			HIVE_NAMESPACE, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hive-controllers", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=control-plane=controller-manager",
			"-n", HIVE_NAMESPACE, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=control-plane=controller-manager", "-n",
			HIVE_NAMESPACE, "-o=jsonpath={.items[0].status.phase}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, contain, "hiveadmission", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=app=hiveadmission",
			"-n", HIVE_NAMESPACE, "-o=jsonpath={.items[*].metadata.name}"}).check(oc)
		newCheck("expect", "get", asAdmin, withoutNamespace, compare, "Running Running", ok, DEFAULT_TIMEOUT, []string{"pod", "--selector=app=hiveadmission", "-n",
			HIVE_NAMESPACE, "-o=jsonpath={.items[*].status.phase}"}).check(oc)
	} else {
		e2e.Logf("hivconfig already exists.")
	}

}

func (imageset *clusterImageSet) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", imageset.template, "-p", "NAME="+imageset.name, "RELEASEIMAGE="+imageset.releaseImage)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (pool *clusterPool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pool.template, "-p", "NAME="+pool.name, "NAMESPACE="+pool.namespace, "FAKE="+pool.fake, "BASEDOMAIN="+pool.baseDomain, "IMAGESETREF="+pool.imageSetRef, "PLATFORMTYPE="+pool.platformType, "CREDREF="+pool.credRef, "REGION="+pool.region, "PULLSECRETREF="+pool.pullSecretRef, "SIZE="+strconv.Itoa(pool.size), "MAXSIZE="+strconv.Itoa(pool.maxSize), "RUNNINGCOUNT="+strconv.Itoa(pool.runningCount), "MAXCONCURRENT="+strconv.Itoa(pool.maxConcurrent), "HIBERNATEAFTER="+pool.hibernateAfter)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (claim *clusterClaim) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", claim.template, "-p", "NAME="+claim.name, "NAMESPACE="+claim.namespace, "CLUSTERPOOLNAME="+claim.clusterPoolName)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (config *installConfig) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", config.template, "-p", "NAME1="+config.name1, "NAMESPACE="+config.namespace, "BASEDOMAIN="+config.baseDomain, "NAME2="+config.name2, "REGION="+config.region)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (cluster *clusterDeployment) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cluster.template, "-p", "FAKE="+cluster.fake, "NAME="+cluster.name, "NAMESPACE="+cluster.namespace, "BASEDOMAIN="+cluster.baseDomain, "CLUSTERNAME="+cluster.clusterName, "PLATFORMTYPE="+cluster.platformType, "CREDREF="+cluster.credRef, "REGION="+cluster.region, "IMAGESETREF="+cluster.imageSetRef, "INSTALLCONFIGSECRET="+cluster.installConfigSecret, "PULLSECRETREF="+cluster.pullSecretRef)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (machine *machinepool) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", machine.template, "-p", "CLUSTERNAME="+machine.clusterName, "NAMESPACE="+machine.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (sync *syncSet) create(oc *exutil.CLI) {
	err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", sync.template, "-p", "NAME="+sync.name, "NAMESPACE="+sync.namespace, "CDREFNAME="+sync.cdrefname, "CMNAME="+sync.cmname, "CMNAMESPACE="+sync.cmnamespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) string {
	var result string
	err := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		result = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cat not get %v without empty", parameters))
	e2e.Logf("the result of queried resource:%v", result)
	return result
}

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

func newCheck(method string, action string, executor bool, inlineNamespace bool, expectAction bool,
	expectContent string, expect bool, timeout int, resource []string) checkDescription {
	return checkDescription{
		method:          method,
		action:          action,
		executor:        executor,
		inlineNamespace: inlineNamespace,
		expectAction:    expectAction,
		expectContent:   expectContent,
		expect:          expect,
		timeout:         timeout,
		resource:        resource,
	}
}

type checkDescription struct {
	method          string
	action          string
	executor        bool
	inlineNamespace bool
	expectAction    bool
	expectContent   string
	expect          bool
	timeout         int
	resource        []string
}

const (
	asAdmin          = true
	withoutNamespace = true
	requireNS        = true
	compare          = true
	contain          = false
	present          = true
	notPresent       = false
	ok               = true
	nok              = false
)

func (ck checkDescription) check(oc *exutil.CLI) {
	switch ck.method {
	case "present":
		ok := isPresentResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.resource...)
		o.Expect(ok).To(o.BeTrue())
	case "expect":
		err := expectedResource(oc, ck.action, ck.executor, ck.inlineNamespace, ck.expectAction, ck.expectContent, ck.expect, ck.timeout, ck.resource...)
		exutil.AssertWaitPollNoErr(err, "can not get expected result")
	default:
		err := fmt.Errorf("unknown method")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func isPresentResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, present bool, parameters ...string) bool {
	parameters = append(parameters, "--ignore-not-found")
	err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		if !present && strings.Compare(output, "") == 0 {
			return true, nil
		}
		if present && strings.Compare(output, "") != 0 {
			return true, nil
		}
		return false, nil
	})
	return err == nil
}

func expectedResource(oc *exutil.CLI, action string, asAdmin bool, withoutNamespace bool, isCompare bool, content string, expect bool, timeout int, parameters ...string) error {
	cc := func(a, b string, ic bool) bool {
		bs := strings.Split(b, "+2+")
		ret := false
		for _, s := range bs {
			if (ic && strings.Compare(a, s) == 0) || (!ic && strings.Contains(a, s)) {
				ret = true
			}
		}
		return ret
	}
	var interval, time_out time.Duration
	if timeout >= CLUSTER_INSTALL_TIMEOUT {
		time_out = time.Duration(timeout/60) * time.Minute
		interval = 6 * time.Minute
	} else {
		time_out = time.Duration(timeout) * time.Second
		interval = time.Duration(timeout/60) * time.Second
	}
	return wait.Poll(interval, time_out, func() (bool, error) {
		output, err := doAction(oc, action, asAdmin, withoutNamespace, parameters...)
		if err != nil {
			e2e.Logf("the get error is %v, and try next", err)
			return false, nil
		}
		e2e.Logf("the queried resource:%s", output)
		if isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s matches one of the content %s, expected", output, content)
			return true, nil
		}
		if isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not matche the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && expect && cc(output, content, isCompare) {
			e2e.Logf("the output %s contains one of the content %s, expected", output, content)
			return true, nil
		}
		if !isCompare && !expect && !cc(output, content, isCompare) {
			e2e.Logf("the output %s does not contain the content %s, expected", output, content)
			return true, nil
		}
		return false, nil
	})
}

func cleanupObjects(oc *exutil.CLI, objs ...objectTableRef) {
	for _, v := range objs {
		e2e.Logf("Start to remove: %v", v)
		if v.namespace != "" {
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, "-n", v.namespace, v.name).Output()
			o.Expect(err).NotTo(o.HaveOccurred())

		} else {
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(v.kind, v.name).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		//For ClusterPool or ClusterDeployment, need to wait ClusterDeployment delete done
		if v.kind == CLUSTER_POOL || v.kind == CLUSTER_DEPLOYMENT {
			e2e.Logf("Wait ClusterDeployment delete done for %s", v.name)
			newCheck("expect", "get", asAdmin, withoutNamespace, contain, v.name, nok, CLUSTER_UNINSTALL_TIMEOUT, []string{CLUSTER_DEPLOYMENT, "-A"}).check(oc)
		}
	}
}

func removeResource(oc *exutil.CLI, parameters ...string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(parameters...).Output()
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("No resource found!")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (hc *hiveconfig) delete(oc *exutil.CLI) {
	removeResource(oc, "hiveconfig", "hive")
}

//Create pull-secret in current project namespace
func createPullSecret(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-pull"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "pull-secret", "--from-file="+dirname+"/.dockerconfigjson", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

//Create AWS credentials in current project namespace
func createAWSCreds(oc *exutil.CLI, namespace string) {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)

	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--to="+dirname, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	err = oc.Run("create").Args("secret", "generic", "aws-creds", "--from-file="+dirname+"/aws_access_key_id", "--from-file="+dirname+"/aws_secret_access_key", "-n", namespace).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func extractRelfromImg(image string) string {
	index := strings.Index(image, ":")
	if index != -1 {
		temp_str := image[index+1 : len(image)]
		index = strings.Index(temp_str, "-")
		if index != -1 {
			e2e.Logf("Extracted OCP release: %s", temp_str[:index])
			return temp_str[:index]
		}
	}
	e2e.Logf("Failed to extract OCP release from Image.")
	return ""
}

//Get CD list from Pool
//Return string CD list such as "pool-44945-2bbln5m47s\n pool-44945-f8xlv6m6s"
func getCDlistfromPool(oc *exutil.CLI, pool string) string {
	fileName := "cd_output_" + getRandomString() + ".txt"
	cd_output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", "-A").OutputToFile(fileName)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.Remove(cd_output)
	pool_cd_list, err := exec.Command("bash", "-c", "cat "+cd_output+" | grep "+pool+" | awk '{print $1}'").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("CD list is %s for pool %s", pool_cd_list, pool)
	return string(pool_cd_list)
}

//Get cluster kubeconfig file
func getClusterKubeconfig(oc *exutil.CLI, clustername, namespace, dir string) {
	kubeconfigsecretname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cd", clustername, "-n", namespace, "-o=jsonpath={.spec.clusterMetadata.adminKubeconfigSecretRef.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Extract cluster %s kubeconfig to %s", clustername, dir)
	err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+kubeconfigsecretname, "-n", namespace, "--to="+dir, "--confirm").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}
