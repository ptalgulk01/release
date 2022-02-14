package router

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ingctrlNodePortDescription struct {
	name        string
	namespace   string
	defaultCert string
	domain      string
	replicas    int
	template    string
}

type ipfailoverDescription struct {
	name      string
	namespace string
	image     string
	vip       string
	template  string
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

func getBaseDomain(oc *exutil.CLI) string {
	var basedomain string

	basedomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.config/cluster", "-o=jsonpath={.spec.baseDomain}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the base domain of the cluster: %v", basedomain)
	return basedomain
}

func (ingctrl *ingctrlNodePortDescription) create(oc *exutil.CLI) {
	err := createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ingctrl.template, "-p", "NAME="+ingctrl.name, "NAMESPACE="+ingctrl.namespace, "DOMAIN="+ingctrl.domain)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (ingctrl *ingctrlNodePortDescription) delete(oc *exutil.CLI) error {
	return oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ingctrl.namespace, "ingresscontroller", ingctrl.name).Execute()
}

func createResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var jsonCfg string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "-temp-resource.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		jsonCfg = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", jsonCfg)
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", jsonCfg).Execute()
}

func waitForCustomIngressControllerAvailable(oc *exutil.CLI, icname string) error {
	e2e.Logf("check ingresscontroller if available")
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresscontroller", icname, "--namespace=openshift-ingress-operator", "-ojsonpath={.status.conditions[?(@.type==\"Available\")].status}").Output()
		e2e.Logf("the status of ingresscontroller is %v", status)
		if err != nil {
			e2e.Logf("failed to get ingresscontroller %s: %v, retrying...", icname, err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("ingresscontroller %s conditions not available, retrying...", icname)
			return false, nil
		}
		return true, nil
	})
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
		e2e.Logf("the Ready status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

// wait for the named resource is disappeared, e.g. used while router deployment rolled out
func waitForResourceToDisappear(oc *exutil.CLI, ns, rsname string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsname, "-n", ns).Output()
		e2e.Logf("check resource %v and got: %v", rsname, status)
		if err != nil {
			if strings.Contains(status, "NotFound") {
				e2e.Logf("the resource is disappeared!")
				return true, nil
			} else {
				e2e.Logf("failed to get the resource: %v, retrying...", err)
				return false, nil
			}
		} else {
			e2e.Logf("the resource is still there, retrying...")
			return false, nil
		}
	})
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For normal user to patch a resource in the specified namespace
func patchResourceAsUser(oc *exutil.CLI, ns, resource, patch string) {
	err := oc.WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For Admin to patch a resource in the specified namespace
func patchResourceAsAdmin(oc *exutil.CLI, ns, resource, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge", "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func exposeRoute(oc *exutil.CLI, ns, resource string) {
	err := oc.Run("expose").Args(resource, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func setAnnotation(oc *exutil.CLI, ns, resource, annotation string) {
	err := oc.Run("annotate").Args("-n", ns, resource, annotation, "--overwrite").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func setEnvVariable(oc *exutil.CLI, ns, resource, envstring string) {
	err := oc.WithoutNamespace().Run("set").Args("env", "-n", ns, resource, envstring).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// for collecting a single pod name for general use.
//usage example: podname := getRouterPod(oc, "default/labelname")
func getRouterPod(oc *exutil.CLI, icname string) string {
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "ingresscontroller.operator.openshift.io/deployment-ingresscontroller="+icname, "-o=jsonpath={.items[0].metadata.name}", "-n", "openshift-ingress").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of podname:%v", podName)
	return podName
}

// For collecting env details with grep from router pod [usage example: readDeploymentData(oc, podname, "search string")] .
// NOTE: This requires getRouterPod function to collect the podname variable first!
func readRouterPodEnv(oc *exutil.CLI, routername, envname string) string {
	ns := "openshift-ingress"
	output := readPodEnv(oc, routername, ns, envname)
	return output
}

// For collecting env details with grep [usage example: readDeploymentData(oc, namespace, podname, "search string")]
func readPodEnv(oc *exutil.CLI, routername, ns string, envname string) string {
	cmd := fmt.Sprintf("/usr/bin/env | grep %s", envname)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, routername, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the matched Env are: %v", output)
	return output
}

// to check the route data in haproxy.config
// grepOptions can specify the lines of the context, e.g. "-A20" or "-C10"
// searchString2 is the config to be checked, since it might exists in multiple routes so use
// searchString1 to locate the specified route config
// after configuring the route the searchString2 need some time to be updated in haproxy.config so wait.Poll is required
func readHaproxyConfig(oc *exutil.CLI, routerPodName, searchString1, grepOption, searchString2 string) string {
	e2e.Logf("Polling and search haproxy config file")
	cmd1 := fmt.Sprintf("grep \"%s\" haproxy.config %s | grep \"%s\"", searchString1, grepOption, searchString2)
	cmd2 := fmt.Sprintf("grep \"%s\" haproxy.config %s", searchString1, grepOption)
	waitErr := wait.Poll(5*time.Second, 60*time.Second, func() (bool, error) {
		_, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerPodName, "--", "bash", "-c", cmd1).Output()
		if err != nil {
			e2e.Logf("string not found, wait and try again...")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but config not found"))
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routerPodName, "--", "bash", "-c", cmd2).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of haproxy.config that matching \"%s\" is: %v", searchString1, output)
	return output
}

func getImagePullSpecFromPayload(oc *exutil.CLI, image string) string {
	var pullspec string
	baseDir := exutil.FixturePath("testdata", "router")
	indexTmpPath := filepath.Join(baseDir, getRandomString())
	dockerconfigjsonpath := filepath.Join(indexTmpPath, ".dockerconfigjson")
	defer exec.Command("rm", "-rf", indexTmpPath).Output()
	err := os.MkdirAll(indexTmpPath, 0755)
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+indexTmpPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	pullspec, err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("release", "info", "--image-for="+image, "-a", dockerconfigjsonpath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the pull spec of image %v is: %v", image, pullspec)
	return pullspec
}

func (ipf *ipfailoverDescription) create(oc *exutil.CLI, ns string) {
	// create ServiceAccount and add it to related SCC
	_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", "ipfailover", "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	_, err = oc.AsAdmin().Run("adm").Args("policy", "add-scc-to-user", "privileged", "-z", "ipfailover").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	// create the ipfailover deployment
	err = createResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", ipf.template, "-p", "NAME="+ipf.name, "NAMESPACE="+ipf.namespace, "IMAGE="+ipf.image)
	o.Expect(err).NotTo(o.HaveOccurred())
}

func waitForIpfailoverEnterMaster(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		log, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, "-l", label).Output()
		e2e.Logf("the logs of labeled pods are: %v", log)
		if err != nil || log == "" {
			e2e.Logf("failed to get logs: %v, retrying...", err)
			return false, nil
		}
		if !strings.Contains(log, "Entering MASTER STATE") {
			e2e.Logf("no ipfailover pod is in MASTER state, retrying...")
			return false, nil
		}
		return true, nil
	})
}

// For collecting information from router pod [usage example: readRouterPodData(oc, podname, executeCmd, "search string")] .
// NOTE: This requires getRouterPod function to collect the podname variable first!
func readRouterPodData(oc *exutil.CLI, routername, executeCmd string, searchString string) string {
	cmd := fmt.Sprintf("%s | grep \"%s\"", executeCmd, searchString)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ingress", routername, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The output from the search: %v", output)
	return output
}

func createConfigMapFromFile(oc *exutil.CLI, ns, name, cmFile string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", name, "--from-file="+cmFile, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func deleteConfigMap(oc *exutil.CLI, ns, name string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", name, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// check if a configmap is created in specific namespace [usage: checkConfigMap(oc, namesapce, configmapName)]
func checkConfigMap(oc *exutil.CLI, ns, configmapName string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		searchOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", ns).Output()
		if err != nil {
			e2e.Logf("failed to get configmap: %v", err)
			return false, nil
		}
		if o.Expect(searchOutput).To(o.ContainSubstring(configmapName)) {
			e2e.Logf("configmap %v found", configmapName)
			return true, nil
		}
		return false, nil
	})
}

// To Collect ingresscontroller domain name
func getIngressctlDomain(oc *exutil.CLI, icname string) string {
	var ingressctldomain string
	ingressctldomain, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ingresscontroller", icname, "--namespace=openshift-ingress-operator", "-o=jsonpath={.spec.domain}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the domain for the ingresscontroller is : %v", ingressctldomain)
	return ingressctldomain
}

// Function to deploy Edge route with default ceritifcates
func exposeRouteEdge(oc *exutil.CLI, ns, route, service, hostname string) {
	_, err := oc.WithoutNamespace().Run("create").Args("-n", ns, "route", "edge", route, "--service="+service, "--hostname="+hostname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// To patch global resources as Admin. Can used for patching resources such as ingresses or CVO
func patchGlobalResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "--patch="+patch, "--type=json").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// this function helps to get the ipv4 address of the given pod
func getPodv4Address(oc *exutil.CLI, podName, namespace string) string {
	podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", podName, namespace, "-o=jsonpath={.status.podIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("IP of the %s pod in namespace %s is %q ", podName, namespace, podIPv4)
	return podIPv4
}

//this function will describe the given pod details
func describePod(oc *exutil.CLI, podName, namespace string) string {
	podDescribe, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", "-n", podName, namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
}

// this function will replace the octate of the ipaddress with the given value
func replaceIpOctet(ipaddress string, octet int, octetValue string) string {
	ipList := strings.Split(ipaddress, ".")
	ipList[octet] = octetValue
	vip := strings.Join(ipList, ".")
	e2e.Logf("The modified ipaddress is %s ", vip)
	return vip
}

// this function is to obtain the pod name based on the particular label
func getPodName(oc *exutil.CLI, namespace string, label string) []string {
	var podName []string
	podNameAll, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podName = strings.Split(podNameAll, " ")
	e2e.Logf("The pod(s) are  %v ", podName)
	return podName
}

func getDNSPodName(oc *exutil.CLI) string {
	ns := "openshift-dns"
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The DNS pod name is: %v", podName)
	return podName
}

// to read the Corefile content in DNS pod
// searchString is to locate the specified section since Corefile might has multiple zones
// that containing same config strings
// grepOptions can specify the lines of the context, e.g. "-A20" or "-C10"
func readDNSCorefile(oc *exutil.CLI, DNSPodName, searchString, grepOption string) string {
	ns := "openshift-dns"
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString, grepOption)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, DNSPodName, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString, output)
	return output
}

// wait for "Progressing" is True
func ensureClusterOperatorProgress(oc *exutil.CLI, coName string) {
	e2e.Logf("waiting for CO %v to start rolling update......", coName)
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Progressing\")].status}"
	waitErr := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/"+coName, jsonPath).Output()
		if strings.Compare(status, "True") == 0 {
			e2e.Logf("Progressing status is True.")
			return true, nil
		} else {
			e2e.Logf("Progressing status is not True, wait and try again...")
			return false, nil
		}
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v didn't goto Progressing status."))
}

// wait for the cluster operator back to normal status ("True False False")
// wait until get 5 successive normal status to ensure it is stable
func ensureClusterOperatorNormal(oc *exutil.CLI, coName string) {
	jsonPath := "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"

	e2e.Logf("waiting for CO %v back to normal status......", coName)
	var count = 0
	waitErr := wait.Poll(3*time.Second, 300*time.Second, func() (bool, error) {
		status, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/"+coName, jsonPath).Output()
		if strings.Compare(status, "TrueFalseFalse") == 0 {
			count++
			if count == 5 {
				e2e.Logf("got %v successive good status (%v), the CO is stable!", count, status)
				return true, nil
			} else {
				e2e.Logf("got %v successive good status (%v), try again...", count, status)
				return false, nil
			}
		} else {
			count = 0
			e2e.Logf("CO status is still abnormal (%v), wait and try again...", status)
			return false, nil
		}
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v is still abnoraml.", coName))
}

// to ensure DNS rolling upgrade is done after updating the global resource "dns.operator/default"
// 1st, co/dns go to Progressing status
// 2nd, co/dns is back to normal and stable
func ensureDNSRollingUpdateDone(oc *exutil.CLI) {
	ensureClusterOperatorProgress(oc, "dns")
	ensureClusterOperatorNormal(oc, "dns")
}

// patch the dns.operator/default with the original value
func restoreDNSOperatorDefault(oc *exutil.CLI) {
	// the json value might be different in different version
	jsonPatch := "[{\"op\":\"replace\", \"path\":\"/spec\", \"value\":{\"logLevel\":\"Normal\",\"nodePlacement\":{},\"operatorLogLevel\":\"Normal\",\"upstreamResolvers\":{\"policy\":\"Sequential\",\"upstreams\":[{\"port\":53,\"type\":\"SystemResolvConf\"}]}}}]"
	e2e.Logf("restore(patch) dns.operator/default with original settings.")
	output, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("dns.operator/default", "-p", jsonPatch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	// patched but got "no change" that means no DNS rolling update, shouldn't goto Progressing
	if strings.Contains(output, "no change") {
		e2e.Logf("skip the Progressing check step.")
	} else {
		ensureClusterOperatorProgress(oc, "dns")
	}
	ensureClusterOperatorNormal(oc, "dns")
}

//this function is to get all dns pods' names, the return is the string slice of all dns pods' names, together with an error
func getAllDNSPodsNames(oc *exutil.CLI) []string {
	podList := []string{}
	output_pods, err := oc.AsAdmin().Run("get").Args("pods", "-n", "openshift-dns").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podsRe := regexp.MustCompile("dns-default-[a-z0-9]+")
	pods := podsRe.FindAllStringSubmatch(output_pods, -1)
	if len(pods) > 0 {
		for i := 0; i < len(pods); i++ {
			podList = append(podList, pods[i][0])
		}
	} else {
		o.Expect(errors.New("Can't find a dns pod")).NotTo(o.HaveOccurred())
	}
	return podList
}

//this function is to select a dns pod randomly
func getRandomDNSPodName(podList []string) string {
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	index := seed.Intn(len(podList))
	return podList[index]
}

//this function to get one dns pod's Corefile info related to the modified time, it looks like {{"dns-default-0001", "2021-12-30 18.011111 Modified"}}
func getOneCorefileStat(oc *exutil.CLI, dnspodname string) [][]string {
	attrList := [][]string{}
	cmd := "stat /etc/coredns/..data/Corefile | grep Modify"
	output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnspodname, "-c", "dns", "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return append(attrList, []string{dnspodname, output})
}

//this function is to make sure all Corefiles(or one Corefile) of the dns pods are updated
//the value of parameter attrList should be from the getOneCorefileStat or getAllCorefilesStat function, it is related to the time before patching something to the dns operator
func waitAllCorefilesUpdated(oc *exutil.CLI, attrList [][]string) [][]string {
	cmd := "stat /etc/coredns/..data/Corefile | grep Modify"
	updated_attrList := [][]string{}
	for _, dnspod := range attrList {
		dnspodname := dnspod[0]
		dnspodattr := dnspod[1]
		count := 0
		waitErr := wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnspodname, "-c", "dns", "--", "bash", "-c", cmd).Output()
			count++
			if dnspodattr != output {
				e2e.Logf(dnspodname + " Corefile is updated")
				updated_attrList = append(updated_attrList, []string{dnspodname, output})
				return true, nil
			} else {
				//reduce the logs
				if count%10 == 1 {
					e2e.Logf(dnspodname + " Corefile isn't updated , wait and try again...")
				}
				return false, nil
			}
		})
		if waitErr != nil {
			updated_attrList = append(updated_attrList, []string{dnspodname, dnspodattr})
		}
		exutil.AssertWaitPollNoErr(waitErr, dnspodname+" Corefile isn't updated")
	}
	return updated_attrList
}

//this function is to wait for Corefile(s) is updated
func waitCorefileUpdated(oc *exutil.CLI, attrList [][]string) [][]string {
	updated_attrList := waitAllCorefilesUpdated(oc, attrList)
	return updated_attrList
}

// this fucntion will return the master pod who has the virtual ip
func getVipOwnerPod(oc *exutil.CLI, ns string, podname []string, vip string) string {
	cmd := fmt.Sprintf("ip address |grep %s", vip)
	var primary_node string
	for i := 0; i < len(podname); i++ {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, podname[i], "--", "bash", "-c", cmd).Output()
		if len(podname) == 1 && output == "command terminated with exit code 1" {
			e2e.Failf("The given pod is not master")
		}
		if output == "command terminated with exit code 1" {
			e2e.Logf("This Pod %v does not have the VIP", podname[i])
		} else if o.Expect(output).To(o.ContainSubstring(vip)) {
			e2e.Logf("The pod owning the VIP is %v", podname[i])
			primary_node = podname[i]
			break
		} else {
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	}
	return primary_node
}

// this function will remove the given element from the slice
func slicingElement(element string, podList []string) []string {
	var newPodList []string
	for index, pod := range podList {
		if pod == element {
			newPodList = append(podList[:index], podList[index+1:]...)
			break
		}
	}
	e2e.Logf("The remaining pod/s in the list is %v", newPodList)
	return newPodList
}

//this function checks whether given pod becomes primary
func waitForPreemptPod(oc *exutil.CLI, ns string, pod string, vip string) {
	cmd := fmt.Sprintf("ip address |grep %s", vip)
	waitErr := wait.Poll(5*time.Second, 30*time.Second, func() (bool, error) {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, pod, "--", "bash", "-c", cmd).Output()
		if o.Expect(output).To(o.ContainSubstring(vip)) {
			e2e.Logf("The new pod %v preempt to become Primary", pod)
			return true, nil
		} else {
			e2e.Logf("pod failed to become Primary yet, retrying...", output)
			return false, nil
		}
	})
	exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached, pod failed to become Primary"))
}
