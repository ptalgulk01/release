package hypershift

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver"
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

func doOcpReq(oc *exutil.CLI, verb OcpClientVerb, notEmpty bool, args ...string) string {
	g.GinkgoHelper()
	res, err := oc.AsAdmin().WithoutNamespace().Run(verb).Args(args...).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	if notEmpty {
		o.Expect(res).ShouldNot(o.BeEmpty())
	}
	return res
}

func checkSubstring(src string, expect []string) {
	if expect == nil || len(expect) <= 0 {
		o.Expect(expect).ShouldNot(o.BeEmpty())
	}

	for i := 0; i < len(expect); i++ {
		o.Expect(src).To(o.ContainSubstring(expect[i]))
	}
}

func checkSubstringWithNoExit(src string, expect []string) bool {
	if expect == nil || len(expect) <= 0 {
		e2e.Logf("Warning expected sub string empty ? %+v", expect)
		return true
	}

	for i := 0; i < len(expect); i++ {
		if !strings.Contains(src, expect[i]) {
			e2e.Logf("expected sub string %s not in src %s", expect[i], src)
			return false
		}
	}

	return true
}

type workload struct {
	name      string
	namespace string
	template  string
}

func (wl *workload) create(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	err := wl.applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, "--ignore-unknown-parameters=true", "-f", wl.template, "-p", "NAME="+wl.name, "NAMESPACE="+wl.namespace)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (wl *workload) delete(oc *exutil.CLI, kubeconfig, parsedTemplate string) {
	defer func() {
		path := filepath.Join(e2e.TestContext.OutputDir, oc.Namespace()+"-"+parsedTemplate)
		os.Remove(path)
	}()
	args := []string{"job", wl.name, "-n", wl.namespace}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args(args...).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (wl *workload) applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	return applyResourceFromTemplate(oc, kubeconfig, parsedTemplate, parameters...)
}

// parse a struct for a Template variables to generate params like "NAME=myname", "NAMESPACE=clusters" ...
// currently only support int, string, bool, *int, *string, *bool. A pointer is used to check whether it is set explicitly.
// use json tag as the true variable Name in the struct e.g. < Name string `json:"NAME"`>
func parseTemplateVarParams(obj interface{}) ([]string, error) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return []string{}, errors.New("params must be a pointer pointed to a struct")
	}

	var params []string
	t := v.Elem().Type()
	for i := 0; i < t.NumField(); i++ {
		if !v.Elem().Field(i).CanInterface() {
			continue
		}
		varName := t.Field(i).Name
		varType := t.Field(i).Type
		varValue := v.Elem().Field(i).Interface()
		tagName := t.Field(i).Tag.Get("json")

		if tagName == "" {
			continue
		}

		//handle non nil pointer that set the params explicitly
		if varType.Kind() == reflect.Ptr {
			if reflect.ValueOf(varValue).IsNil() {
				continue
			}

			switch reflect.ValueOf(varValue).Elem().Type().Kind() {
			case reflect.Int:
				p := fmt.Sprintf("%s=%d", tagName, reflect.ValueOf(varValue).Elem().Interface().(int))
				params = append(params, p)
			case reflect.String:
				params = append(params, tagName+"="+reflect.ValueOf(varValue).Elem().Interface().(string))
			case reflect.Bool:
				v, _ := reflect.ValueOf(varValue).Elem().Interface().(bool)
				params = append(params, tagName+"="+strconv.FormatBool(v))
			default:
				e2e.Logf("parseTemplateVarParams params %v invalid, ignore it", varName)
			}
			continue
		}

		//non-pointer
		switch varType.Kind() {
		case reflect.String:
			if varValue.(string) != "" {
				params = append(params, tagName+"="+varValue.(string))
			}
		case reflect.Int:
			params = append(params, tagName+"="+strconv.Itoa(varValue.(int)))
		case reflect.Bool:
			params = append(params, tagName+"="+strconv.FormatBool(varValue.(bool)))
		default:
			e2e.Logf("parseTemplateVarParams params %v not support, ignore it", varValue)
		}
	}

	return params, nil
}

func applyResourceFromTemplate(oc *exutil.CLI, kubeconfig, parsedTemplate string, parameters ...string) error {
	var configFile string
	defer func() {
		if len(configFile) > 0 {
			_ = os.Remove(configFile)
		}
	}()
	err := wait.PollUntilContextTimeout(context.Background(), 3*time.Second, 15*time.Second, true, func(_ context.Context) (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(parsedTemplate)
		if err != nil {
			e2e.Logf("Error processing template: %v, keep polling", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	o.Expect(err).NotTo(o.HaveOccurred())

	var args = []string{"-f", configFile}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args(args...).Execute()
}

func getClusterRegion(oc *exutil.CLI) (string, error) {
	return oc.AsAdmin().WithoutNamespace().Run("get").Args("node", `-ojsonpath={.items[].metadata.labels.topology\.kubernetes\.io/region}`).Output()
}

func getBaseDomain(oc *exutil.CLI) (string, error) {
	str, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns/cluster", `-ojsonpath={.spec.baseDomain}`).Output()
	if err != nil {
		return "", err
	}
	index := strings.Index(str, ".")
	if index == -1 {
		return "", fmt.Errorf("can not parse baseDomain because not finding '.'")
	}
	return str[index+1:], nil
}

func getAWSKey(oc *exutil.CLI) (string, string, error) {
	accessKeyID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", `template={{index .data "aws_access_key_id"|base64decode}}`).Output()
	if err != nil {
		return "", "", err
	}
	secureKey, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/aws-creds", "-n", "kube-system", "-o", `template={{index .data "aws_secret_access_key"|base64decode}}`).Output()
	if err != nil {
		return "", "", err
	}
	return accessKeyID, secureKey, nil
}

func getAzureKey(oc *exutil.CLI) (string, string, string, string, error) {
	clientID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "-o", `template={{index .data "azure_client_id"|base64decode}}`).Output()
	if err != nil {
		return "", "", "", "", err
	}
	clientSecret, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "-o", `template={{index .data "azure_client_secret"|base64decode}}`).Output()
	if err != nil {
		return "", "", "", "", err
	}
	subscriptionID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "-o", `template={{index .data "azure_subscription_id"|base64decode}}`).Output()
	if err != nil {
		return "", "", "", "", err
	}
	tenantID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/azure-credentials", "-n", "kube-system", "-o", `template={{index .data "azure_tenant_id"|base64decode}}`).Output()
	if err != nil {
		return "", "", "", "", err
	}
	return clientID, clientSecret, subscriptionID, tenantID, nil
}

/*
	parse a structure's tag 'param' and output cli command parameters like --params=$var, support embedded struct

e.g.
Input:

	  type example struct {
		Name string `param:"name"`
	    PullSecret string `param:"pull_secret"`
	  } {
	  	Name:"hypershift",
	    PullSecret:"pullsecret.txt",
	  }

Output:

	--name="hypershift" --pull_secret="pullsecret.txt"
*/
func parse(obj interface{}) ([]string, error) {
	var params []string
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() == reflect.Struct {
		return parseStruct(v.Interface(), params)
	}
	return []string{}, nil
}

func parseStruct(obj interface{}, params []string) ([]string, error) {
	v := reflect.ValueOf(obj)
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		varType := t.Field(i).Type
		varValueV := v.Field(i)

		if !t.Field(i).IsExported() {
			continue
		}

		if varType.Kind() == reflect.Ptr && varValueV.IsNil() {
			continue
		}

		for varType.Kind() == reflect.Ptr {
			varType = varType.Elem()
			varValueV = varValueV.Elem()
		}

		var err error
		if varType.Kind() == reflect.Struct {
			params, err = parseStruct(varValueV.Interface(), params)
			if err != nil {
				return []string{}, err
			}
			continue
		}

		varValue := varValueV.Interface()
		tagName := t.Field(i).Tag.Get("param")
		if tagName == "" {
			continue
		}

		switch varType.Kind() {
		case reflect.String:
			if varValue.(string) != "" {
				params = append(params, "--"+tagName+"="+varValue.(string))
			}
		case reflect.Int:
			params = append(params, "--"+tagName+"="+strconv.Itoa(varValue.(int)))
		case reflect.Int64:
			params = append(params, "--"+tagName+"="+strconv.FormatInt(varValue.(int64), 10))
		case reflect.Bool:
			params = append(params, "--"+tagName+"="+strconv.FormatBool(varValue.(bool)))
		default:
			e2e.Logf("parseTemplateVarParams params %s %v not support, ignore it", varType.Kind(), varValue)
		}
	}
	return params, nil
}

func getSha256ByFile(file string) string {
	ha := sha256.New()
	f, err := os.Open(file)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	defer f.Close()
	_, err = io.Copy(ha, f)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return fmt.Sprintf("%X", ha.Sum(nil))
}

func getJSONByFile(filePath string, path string) gjson.Result {
	file, err := os.Open(filePath)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	defer file.Close()
	con, err := ioutil.ReadAll(file)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return gjson.Get(string(con), path)
}

func replaceInFile(file string, old string, new string) error {
	input, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	output := bytes.Replace(input, []byte(old), []byte(new), -1)
	err = ioutil.WriteFile(file, output, 0666)
	return err
}

func execCMDOnWorkNodeByBastion(showInfo bool, nodeIP, bastionIP, exec string) string {
	var bashClient = NewCmdClient().WithShowInfo(showInfo)
	privateKey, err := exutil.GetPrivateKey()
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := `chmod 600 ` + privateKey + `; ssh -i ` + privateKey + ` -o StrictHostKeyChecking=no -o ProxyCommand="ssh -i ` + privateKey + " -o StrictHostKeyChecking=no -W %h:%p ec2-user@" + bastionIP + `" core@` + nodeIP + ` '` + exec + `'`
	log, err := bashClient.Run(cmd).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return log
}

func getAllByFile(filePath string) string {
	con, err := ioutil.ReadFile(filePath)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return string(con)
}

func getAWSPrivateCredentials() string {
	privateCred := DefaultAWSHyperShiftPrivateSecretFile
	if cred := os.Getenv(AWS_HYPERSHIFT_PRIVATE_SECRET_FILE); cred != "" {
		privateCred = cred
	}
	return privateCred
}

func subtractMinor(version *semver.Version, count uint64) *semver.Version {
	result := *version
	result.Minor = maxInt64(0, result.Minor-count)
	return &result
}

func maxInt64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func getHyperShiftOperatorLatestSupportOCPVersion() string {
	var bashClient = NewCmdClient().WithShowInfo(true)
	res, err := bashClient.Run(fmt.Sprintf("oc logs -n hypershift -lapp=operator --tail=-1 | head -1")).Output()
	o.Expect(err).ShouldNot(o.HaveOccurred())

	re := regexp.MustCompile(`Latest supported OCP: (\d+\.\d+\.\d+)`)
	match := re.FindStringSubmatch(res)
	o.Expect(len(match) > 1).Should(o.BeTrue())
	return match[1]
}

func getHyperShiftSupportedOCPVersion() (semver.Version, semver.Version) {
	v := getHyperShiftOperatorLatestSupportOCPVersion()
	latestSupportedVersion := semver.MustParse(v)
	minSupportedVersion := semver.MustParse(subtractMinor(&latestSupportedVersion, uint64(SupportedPreviousMinorVersions)).String())
	return latestSupportedVersion, minSupportedVersion
}

func getMinSupportedOCPVersion() string {
	_, minVersion := getHyperShiftSupportedOCPVersion()
	return minVersion.String()
}

// getAWSMgmtClusterAvailableZones returns available zones based on mgmt cluster's oc client and region
func getAWSMgmtClusterRegionAvailableZones(oc *exutil.CLI) []string {
	region, err := exutil.GetAWSClusterRegion(oc)
	o.Expect(err).ShouldNot(o.HaveOccurred())
	exutil.GetAwsCredentialFromCluster(oc)
	awsClient := exutil.InitAwsSessionWithRegion(region)
	availableZones, err := awsClient.GetAvailabilityZoneNames()
	o.Expect(err).ShouldNot(o.HaveOccurred())
	return availableZones
}

// removeNodesTaint removes the node taint by taintKey if the node exists
func removeNodesTaint(oc *exutil.CLI, nodes []string, taintKey string) {
	for _, no := range nodes {
		nodeInfo := doOcpReq(oc, OcpGet, false, "no", no, "--ignore-not-found")
		if nodeInfo != "" {
			doOcpReq(oc, OcpAdm, false, "taint", "node", no, taintKey+"-")
		}
	}
}

// removeNodesLabel removes the node label by labelKey if the node exists
func removeNodesLabel(oc *exutil.CLI, nodes []string, labelKey string) {
	for _, no := range nodes {
		nodeInfo := doOcpReq(oc, OcpGet, false, "no", no, "--ignore-not-found")
		if nodeInfo != "" {
			doOcpReq(oc, OcpLabel, false, "node", no, labelKey+"-")
		}
	}
}

func getLatestUnsupportedOCPVersion() string {
	min := semver.MustParse(getMinSupportedOCPVersion())
	return semver.MustParse(subtractMinor(&min, uint64(1)).String()).String()
}

// remove z stream suffix 4.12.0 --> 4.12
func getVersionWithMajorAndMinor(version string) (string, error) {
	v := strings.Split(version, ".")
	if len(v) == 0 || len(v) > 3 {
		return "", fmt.Errorf("invalid version")
	}
	if len(v) < 3 {
		return version, nil
	} else {
		return strings.Join(v[:2], "."), nil
	}
}
