package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
	netutils "k8s.io/utils/net"
)

type pingPodResource struct {
	name      string
	namespace string
	template  string
}

type pingPodResourceNode struct {
	name      string
	namespace string
	nodename  string
	template  string
}

type pingPodResourceWinNode struct {
	name      string
	namespace string
	image     string
	nodename  string
	template  string
}

type egressIPResource1 struct {
	name          string
	template      string
	egressIP1     string
	egressIP2     string
	nsLabelKey    string
	nsLabelValue  string
	podLabelKey   string
	podLabelValue string
}

type egressFirewall1 struct {
	name      string
	namespace string
	template  string
}

type egressFirewall2 struct {
	name      string
	namespace string
	ruletype  string
	cidr      string
	template  string
}

type ipBlockCIDRsDual struct {
	name      string
	namespace string
	cidrIpv4  string
	cidrIpv6  string
	cidr2Ipv4 string
	cidr2Ipv6 string
	cidr3Ipv4 string
	cidr3Ipv6 string
	template  string
}

type ipBlockCIDRsSingle struct {
	name      string
	namespace string
	cidr      string
	cidr2     string
	cidr3     string
	template  string
}
type ipBlockCIDRsExceptDual struct {
	name            string
	namespace       string
	cidrIpv4        string
	cidrIpv4Except  string
	cidrIpv6        string
	cidrIpv6Except  string
	cidr2Ipv4       string
	cidr2Ipv4Except string
	cidr2Ipv6       string
	cidr2Ipv6Except string
	cidr3Ipv4       string
	cidr3Ipv4Except string
	cidr3Ipv6       string
	cidr3Ipv6Except string
	template        string
}
type ipBlockCIDRsExceptSingle struct {
	name      string
	namespace string
	cidr      string
	except    string
	cidr2     string
	except2   string
	cidr3     string
	except3   string
	template  string
}

type genericServiceResource struct {
	servicename           string
	namespace             string
	protocol              string
	selector              string
	serviceType           string
	ipFamilyPolicy        string
	externalTrafficPolicy string
	internalTrafficPolicy string
	template              string
}

type windowGenericServiceResource struct {
	servicename           string
	namespace             string
	protocol              string
	selector              string
	serviceType           string
	ipFamilyPolicy        string
	externalTrafficPolicy string
	internalTrafficPolicy string
	template              string
}

type testPodMultinetwork struct {
	name      string
	namespace string
	nodename  string
	nadname   string
	labelname string
	template  string
}

type externalIPService struct {
	name       string
	namespace  string
	externalIP string
	template   string
}

type externalIPPod struct {
	name      string
	namespace string
	template  string
}

type nodePortService struct {
	name      string
	namespace string
	nodeName  string
	template  string
}

type egressPolicy struct {
	name         string
	namespace    string
	cidrSelector string
	template     string
}
type aclSettings struct {
	DenySetting  string `json:"deny"`
	AllowSetting string `json:"allow"`
}

type egressrouterMultipleDst struct {
	name           string
	namespace      string
	reservedip     string
	gateway        string
	destinationip1 string
	destinationip2 string
	destinationip3 string
	template       string
}

type egressrouterRedSDN struct {
	name          string
	namespace     string
	reservedip    string
	gateway       string
	destinationip string
	labelkey      string
	labelvalue    string
	template      string
}

type egressFirewall5 struct {
	name        string
	namespace   string
	ruletype1   string
	rulename1   string
	rulevalue1  string
	protocol1   string
	portnumber1 int
	ruletype2   string
	rulename2   string
	rulevalue2  string
	protocol2   string
	portnumber2 int
	template    string
}

type egressNetworkpolicy struct {
	name      string
	namespace string
	ruletype  string
	rulename  string
	rulevalue string
	template  string
}

type svcEndpontDetails struct {
	ovnKubeNodePod string
	nodeName       string
	podIP          string
}

type migrationDetails struct {
	name                   string
	template               string
	namespace              string
	virtualmachinesintance string
}

type kubeletKillerPod struct {
	name      string
	namespace string
	nodename  string
	template  string
}

func (pod *pingPodResource) createPingPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *pingPodResourceNode) createPingPodNode(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "NODENAME="+pod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *pingPodResourceWinNode) createPingPodWinNode(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "IMAGE="+pod.image, "NODENAME="+pod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func (pod *testPodMultinetwork) createTestPodMultinetwork(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace, "NODENAME="+pod.nodename, "LABELNAME="+pod.labelname, "NADNAME="+pod.nadname)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create pod %v", pod.name))
}

func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(getRandomString() + "ping-pod.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

func (egressIP *egressIPResource1) createEgressIPObject1(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressIP.template, "-p", "NAME="+egressIP.name, "EGRESSIP1="+egressIP.egressIP1, "EGRESSIP2="+egressIP.egressIP2)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressIP %v", egressIP.name))
}

func (egressIP *egressIPResource1) deleteEgressIPObject1(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressip", egressIP.name)
}

func (egressIP *egressIPResource1) createEgressIPObject2(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressIP.template, "-p", "NAME="+egressIP.name, "EGRESSIP1="+egressIP.egressIP1, "NSLABELKEY="+egressIP.nsLabelKey, "NSLABELVALUE="+egressIP.nsLabelValue, "PODLABELKEY="+egressIP.podLabelKey, "PODLABELVALUE="+egressIP.podLabelValue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressIP %v", egressIP.name))
}

func (egressFirewall *egressFirewall1) createEgressFWObject1(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressFirewall.template, "-p", "NAME="+egressFirewall.name, "NAMESPACE="+egressFirewall.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressFW %v", egressFirewall.name))
}

func (egressFirewall *egressFirewall1) deleteEgressFWObject1(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressfirewall", egressFirewall.name, "-n", egressFirewall.namespace)
}

func (egressFirewall *egressFirewall2) createEgressFW2Object(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressFirewall.template, "-p", "NAME="+egressFirewall.name, "NAMESPACE="+egressFirewall.namespace, "RULETYPE="+egressFirewall.ruletype, "CIDR="+egressFirewall.cidr)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressFW2 %v", egressFirewall.name))
}

func (EFW *egressFirewall5) createEgressFW5Object(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		parameters := []string{"--ignore-unknown-parameters=true", "-f", EFW.template, "-p", "NAME=" + EFW.name, "NAMESPACE=" + EFW.namespace, "RULETYPE1=" + EFW.ruletype1, "RULENAME1=" + EFW.rulename1, "RULEVALUE1=" + EFW.rulevalue1, "PROTOCOL1=" + EFW.protocol1, "PORTNUMBER1=" + strconv.Itoa(EFW.portnumber1), "RULETYPE2=" + EFW.ruletype2, "RULENAME2=" + EFW.rulename2, "RULEVALUE2=" + EFW.rulevalue2, "PROTOCOL2=" + EFW.protocol2, "PORTNUMBER2=" + strconv.Itoa(EFW.portnumber2)}
		err1 := applyResourceFromTemplateByAdmin(oc, parameters...)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create EgressFW2 %v", EFW.name))
}

func (eNPL *egressNetworkpolicy) createEgressNetworkPolicyObj(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		parameters := []string{"--ignore-unknown-parameters=true", "-f", eNPL.template, "-p", "NAME=" + eNPL.name, "NAMESPACE=" + eNPL.namespace, "RULETYPE=" + eNPL.ruletype, "RULENAME=" + eNPL.rulename, "RULEVALUE=" + eNPL.rulevalue}
		err1 := applyResourceFromTemplateByAdmin(oc, parameters...)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Failed to create EgressNetworkPolicy %v in Namespace %v", eNPL.name, eNPL.namespace))
}

// Single CIDR on Dual stack
func (ipBlock_policy *ipBlockCIDRsDual) createipBlockCIDRObjectDual(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_policy.template, "-p", "NAME="+ipBlock_policy.name, "NAMESPACE="+ipBlock_policy.namespace, "cidrIpv6="+ipBlock_policy.cidrIpv6, "cidrIpv4="+ipBlock_policy.cidrIpv4)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_policy.name))
}

// Single CIDR on single stack
func (ipBlock_policy *ipBlockCIDRsSingle) createipBlockCIDRObjectSingle(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_policy.template, "-p", "NAME="+ipBlock_policy.name, "NAMESPACE="+ipBlock_policy.namespace, "CIDR="+ipBlock_policy.cidr)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_policy.name))
}

// Single IP Block with except clause on Dual stack
func (ipBlock_except_policy *ipBlockCIDRsExceptDual) createipBlockExceptObjectDual(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {

		policyApplyError := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_except_policy.template, "-p", "NAME="+ipBlock_except_policy.name, "NAMESPACE="+ipBlock_except_policy.namespace, "CIDR_IPv6="+ipBlock_except_policy.cidrIpv6, "EXCEPT_IPv6="+ipBlock_except_policy.cidrIpv6Except, "CIDR_IPv4="+ipBlock_except_policy.cidrIpv4, "EXCEPT_IPv4="+ipBlock_except_policy.cidrIpv4Except)
		if policyApplyError != nil {
			e2e.Logf("the err:%v, and try next round", policyApplyError)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_except_policy.name))
}

// Single IP Block with except clause on Single stack
func (ipBlock_except_policy *ipBlockCIDRsExceptSingle) createipBlockExceptObjectSingle(oc *exutil.CLI, except bool) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {

		policyApplyError := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_except_policy.template, "-p", "NAME="+ipBlock_except_policy.name, "NAMESPACE="+ipBlock_except_policy.namespace, "CIDR="+ipBlock_except_policy.cidr, "EXCEPT="+ipBlock_except_policy.except)
		if policyApplyError != nil {
			e2e.Logf("the err:%v, and try next round", policyApplyError)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_except_policy.name))
}

// Function to create ingress or egress policy with multiple CIDRs on Dual Stack Cluster
func (ipBlock_cidrs_policy *ipBlockCIDRsDual) createIPBlockMultipleCIDRsObjectDual(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_cidrs_policy.template, "-p", "NAME="+ipBlock_cidrs_policy.name, "NAMESPACE="+ipBlock_cidrs_policy.namespace, "cidrIpv6="+ipBlock_cidrs_policy.cidrIpv6, "cidrIpv4="+ipBlock_cidrs_policy.cidrIpv4, "cidr2Ipv4="+ipBlock_cidrs_policy.cidr2Ipv4, "cidr2Ipv6="+ipBlock_cidrs_policy.cidr2Ipv6, "cidr3Ipv4="+ipBlock_cidrs_policy.cidr3Ipv4, "cidr3Ipv6="+ipBlock_cidrs_policy.cidr3Ipv6)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_cidrs_policy.name))
}

// Function to create ingress or egress policy with multiple CIDRs on Single Stack Cluster
func (ipBlock_cidrs_policy *ipBlockCIDRsSingle) createIPBlockMultipleCIDRsObjectSingle(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", ipBlock_cidrs_policy.template, "-p", "NAME="+ipBlock_cidrs_policy.name, "NAMESPACE="+ipBlock_cidrs_policy.namespace, "CIDR="+ipBlock_cidrs_policy.cidr, "CIDR2="+ipBlock_cidrs_policy.cidr2, "CIDR3="+ipBlock_cidrs_policy.cidr3)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create network policy %v", ipBlock_cidrs_policy.name))
}

func (service *genericServiceResource) createServiceFromParams(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "SERVICENAME="+service.servicename, "NAMESPACE="+service.namespace, "PROTOCOL="+service.protocol, "SELECTOR="+service.selector, "serviceType="+service.serviceType, "ipFamilyPolicy="+service.ipFamilyPolicy, "internalTrafficPolicy="+service.internalTrafficPolicy, "externalTrafficPolicy="+service.externalTrafficPolicy)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create svc %v", service.servicename))
}

func (service *windowGenericServiceResource) createWinServiceFromParams(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", service.template, "-p", "SERVICENAME="+service.servicename, "NAMESPACE="+service.namespace, "PROTOCOL="+service.protocol, "SELECTOR="+service.selector, "serviceType="+service.serviceType, "ipFamilyPolicy="+service.ipFamilyPolicy, "internalTrafficPolicy="+service.internalTrafficPolicy, "externalTrafficPolicy="+service.externalTrafficPolicy)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create svc %v", service.servicename))
}

func (egressrouter *egressrouterMultipleDst) createEgressRouterMultipeDst(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressrouter.template, "-p", "NAME="+egressrouter.name, "NAMESPACE="+egressrouter.namespace, "RESERVEDIP="+egressrouter.reservedip, "GATEWAY="+egressrouter.gateway, "DSTIP1="+egressrouter.destinationip1, "DSTIP2="+egressrouter.destinationip2, "DSTIP3="+egressrouter.destinationip3)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create egressrouter %v", egressrouter.name))
}

func (egressrouter *egressrouterRedSDN) createEgressRouterRedSDN(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", egressrouter.template, "-p", "NAME="+egressrouter.name, "NAMESPACE="+egressrouter.namespace, "RESERVEDIP="+egressrouter.reservedip, "GATEWAY="+egressrouter.gateway, "DSTIP="+egressrouter.destinationip, "LABELKEY="+egressrouter.labelkey, "LABELVALUE="+egressrouter.labelvalue)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create egressrouter %v", egressrouter.name))
}

func (egressFirewall *egressFirewall2) deleteEgressFW2Object(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressfirewall", egressFirewall.name, "-n", egressFirewall.namespace)
}

func (pod *pingPodResource) deletePingPod(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pod.name, "-n", pod.namespace)
}

func (pod *pingPodResourceNode) deletePingPodNode(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pod.name, "-n", pod.namespace)
}

func removeResource(oc *exutil.CLI, asAdmin bool, withoutNamespace bool, parameters ...string) {
	output, err := doAction(oc, "delete", asAdmin, withoutNamespace, parameters...)
	if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
		e2e.Logf("the resource is deleted already")
		return
	}
	o.Expect(err).NotTo(o.HaveOccurred())

	err = wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
		output, err := doAction(oc, "get", asAdmin, withoutNamespace, parameters...)
		if err != nil && (strings.Contains(output, "NotFound") || strings.Contains(output, "No resources found")) {
			e2e.Logf("the resource is delete successfully")
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to delete resource %v", parameters))
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

func applyResourceFromTemplateByAdmin(oc *exutil.CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "resource.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("as admin fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().AsAdmin().Run("apply").Args("-f", configFile).Execute()
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

func getPodStatus(oc *exutil.CLI, namespace string, podName string) (string, error) {
	podStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status in namespace %s is %q", podName, namespace, podStatus)
	return podStatus, err
}

func checkPodReady(oc *exutil.CLI, namespace string, podName string) (bool, error) {
	podOutPut, err := getPodStatus(oc, namespace, podName)
	status := []string{"Running", "Ready", "Complete", "Succeeded"}
	return contains(status, podOutPut), err
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

func waitPodReady(oc *exutil.CLI, namespace string, podName string) {
	err := wait.Poll(10*time.Second, 100*time.Second, func() (bool, error) {
		status, err1 := checkPodReady(oc, namespace, podName)
		if err1 != nil {
			e2e.Logf("the err:%v, wait for pod %v to become ready.", err1, podName)
			return status, err1
		}
		if !status {
			return status, nil
		}
		return status, nil
	})

	if err != nil {
		podDescribe := describePod(oc, namespace, podName)
		e2e.Logf("oc describe pod %v.", podName)
		e2e.Logf(podDescribe)
	}
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("pod %v is not ready", podName))
}

func describePod(oc *exutil.CLI, namespace string, podName string) string {
	podDescribe, err := oc.WithoutNamespace().Run("describe").Args("pod", "-n", namespace, podName).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status is %q", podName, podDescribe)
	return podDescribe
}

func execCommandInSpecificPod(oc *exutil.CLI, namespace string, podName string, command string) (string, error) {
	e2e.Logf("The command is: %v", command)
	command1 := []string{"-n", namespace, podName, "--", "bash", "-c", command}
	msg, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(command1...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with  err:%v  and output is %v.", err, msg)
		return msg, err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func execCommandInNetworkingPod(oc *exutil.CLI, command string) (string, error) {
	networkType := checkNetworkType(oc)
	var cmd []string
	if strings.Contains(networkType, "ovn") {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-ovn-kubernetes", "-l", "app=ovnkube-node", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Cannot get onv-kubernetes pods, errors: %v", err)
			return "", err
		}
		cmd = []string{"-n", "openshift-ovn-kubernetes", "-c", "ovnkube-controller", podName, "--", "/bin/sh", "-c", command}
	} else if strings.Contains(networkType, "sdn") {
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-sdn", "-l", "app=sdn", "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Cannot get openshift-sdn pods, errors: %v", err)
			return "", err
		}
		cmd = []string{"-n", "openshift-sdn", "-c", "sdn", podName, "--", "/bin/sh", "-c", command}
	}

	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(cmd...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with  err:%v .", err)
		return "", err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func getDefaultInterface(oc *exutil.CLI) (string, error) {
	getDefaultInterfaceCmd := "/usr/sbin/ip -4 route show default"
	int1, err := execCommandInNetworkingPod(oc, getDefaultInterfaceCmd)
	if err != nil {
		e2e.Logf("Cannot get default interface, errors: %v", err)
		return "", err
	}
	defInterface := strings.Split(int1, " ")[4]
	e2e.Logf("Get the default inteface: %s", defInterface)
	return defInterface, nil
}

func getDefaultSubnet(oc *exutil.CLI) (string, error) {
	int1, _ := getDefaultInterface(oc)
	getDefaultSubnetCmd := "/usr/sbin/ip -4 -brief a show " + int1
	subnet1, err := execCommandInNetworkingPod(oc, getDefaultSubnetCmd)
	defSubnet := strings.Fields(subnet1)[2]
	if err != nil {
		e2e.Logf("Cannot get default subnet, errors: %v", err)
		return "", err
	}
	e2e.Logf("Get the default subnet: %s", defSubnet)
	return defSubnet, nil
}

// Hosts function return the host network CIDR
func Hosts(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	e2e.Logf("in Hosts function, ip: %v, ipnet: %v", ip, ipnet)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}
	// remove network address and broadcast address
	return ips[1 : len(ips)-1], nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func findUnUsedIPs(oc *exutil.CLI, cidr string, number int) []string {
	ipRange, _ := Hosts(cidr)
	var ipUnused = []string{}
	//shuffle the ips slice
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(ipRange), func(i, j int) { ipRange[i], ipRange[j] = ipRange[j], ipRange[i] })
	for _, ip := range ipRange {
		if len(ipUnused) < number {
			pingCmd := "ping -c4 -t1 " + ip
			_, err := execCommandInNetworkingPod(oc, pingCmd)
			if err != nil {
				e2e.Logf("%s is not used!\n", ip)
				ipUnused = append(ipUnused, ip)
			}
		} else {
			break
		}

	}
	return ipUnused
}

func ipEchoServer() string {
	return "172.31.249.80:9095"
}

func checkPlatform(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
	return strings.ToLower(output)
}

func checkNetworkType(oc *exutil.CLI) string {
	output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.type}").Output()
	return strings.ToLower(output)
}

func getDefaultIPv6Subnet(oc *exutil.CLI) (string, error) {
	int1, _ := getDefaultInterface(oc)
	getDefaultSubnetCmd := "/usr/sbin/ip -6 -brief a show " + int1
	subnet1, err := execCommandInNetworkingPod(oc, getDefaultSubnetCmd)
	if err != nil {
		e2e.Logf("Cannot get default ipv6 subnet, errors: %v", err)
		return "", err
	}
	defSubnet := strings.Fields(subnet1)[2]
	e2e.Logf("Get the default ipv6 subnet: %s", defSubnet)
	return defSubnet, nil
}

func findUnUsedIPv6(oc *exutil.CLI, cidr string, number int) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	number += 2
	var ips []string
	var i = 0
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		//Not use the first two IPv6 addresses , such as 2620:52:0:4e::  , 2620:52:0:4e::1
		if i == 0 || i == 1 {
			i++
			continue
		}
		//Start to detect the IPv6 adress is used or not
		if i < number {
			pingCmd := "ping -c4 -t1 -6 " + ip.String()
			_, err := execCommandInNetworkingPod(oc, pingCmd)
			if err != nil {
				e2e.Logf("%s is not used!\n", ip)
				ips = append(ips, ip.String())
				i++
			}
		} else {
			break
		}

	}

	return ips, nil
}

func ipv6EchoServer(isIPv6 bool) string {
	if isIPv6 {
		return "[2620:52:0:4974:def4:1ff:fee7:8144]:8085"
	}
	return "10.73.116.56:8085"
}

func checkIPStackType(oc *exutil.CLI) string {
	svcNetwork, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.serviceNetwork}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Count(svcNetwork, ":") >= 2 && strings.Count(svcNetwork, ".") >= 2 {
		return "dualstack"
	} else if strings.Count(svcNetwork, ":") >= 2 {
		return "ipv6single"
	} else if strings.Count(svcNetwork, ".") >= 2 {
		return "ipv4single"
	}
	return ""
}

func installSctpModule(oc *exutil.CLI, configFile string) {
	status, _ := oc.AsAdmin().Run("get").Args("machineconfigs").Output()
	if !strings.Contains(status, "load-sctp-module") {
		err := oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func checkSctpModule(oc *exutil.CLI, nodeName, namespace string) {
	defer exutil.RecoverNamespaceRestricted(oc, namespace)
	exutil.SetNamespacePrivileged(oc, namespace)
	err := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
		// Check nodes status to make sure all nodes are up after rebooting caused by load-sctp-module
		nodesStatus, err := oc.AsAdmin().Run("get").Args("node").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("oc_get_nodes: %v", nodesStatus)
		status, _ := oc.AsAdmin().Run("debug").Args("node/"+nodeName, "--", "cat", "/sys/module/sctp/initstate").Output()
		if strings.Contains(status, "live") {
			e2e.Logf("stcp module is installed in the %s", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "stcp module is installed in the nodes")
}

func getPodIPv4(oc *exutil.CLI, namespace string, podName string) string {
	podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv4)
	return podIPv4
}

func getPodIPv6(oc *exutil.CLI, namespace string, podName string, ipStack string) string {
	if ipStack == "ipv6single" {
		podIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	} else if ipStack == "dualstack" {
		podIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	}
	return ""
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(15*time.Second, 10*time.Minute, func() (bool, error) {
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

func getSvcIPv4(oc *exutil.CLI, namespace string, svcName string) string {
	svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
	return svcIPv4
}

func getSvcIPv6(oc *exutil.CLI, namespace string, svcName string) string {
	svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv6
}

func getSvcIPdualstack(oc *exutil.CLI, namespace string, svcName string) (string, string) {
	svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
	svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv4, svcIPv6
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

func sshRunCmd(host string, user string, cmd string) error {
	privateKey := os.Getenv("SSH_CLOUD_PRIV_KEY")
	if privateKey == "" {
		privateKey = "../internal/config/keys/openshift-qe.pem"
	}
	sshClient := exutil.SshClient{User: user, Host: host, Port: 22, PrivateKey: privateKey}
	return sshClient.Run(cmd)
}

// For Admin to patch a resource in the specified namespace
func patchResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type=merge").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// Check network operator status in intervals until timeout
func checkNetworkOperatorState(oc *exutil.CLI, interval int, timeout int) {
	errCheck := wait.Poll(time.Duration(interval)*time.Second, time.Duration(timeout)*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "network").Output()
		if err != nil {
			e2e.Logf("Fail to get clusteroperator network, error:%s. Trying again", err)
			return false, nil
		}
		matched, _ := regexp.MatchString("True.*False.*False", output)
		e2e.Logf("Network operator state is:%s", output)
		o.Expect(matched).To(o.BeTrue())
		return false, nil
	})
	o.Expect(errCheck.Error()).To(o.ContainSubstring("timed out waiting for the condition"))
}

func getNodeIPv4(oc *exutil.CLI, namespace, nodeName string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", oc.Namespace(), "node", nodeName, "-o=jsonpath={.status.addresses[?(@.type==\"InternalIP\")].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if err != nil {
		e2e.Logf("Cannot get node default interface ipv4 address, errors: %v", err)
	}

	// when egressIP is applied to a node, it would be listed as internal IP for the node, thus, there could be more than one IPs shown as internal IP
	// use RE to match out to first internal IP
	re := regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	nodeipv4 := re.FindAllString(output, -1)[0]
	e2e.Logf("The IPv4 of node's default interface is %q", nodeipv4)
	return nodeipv4
}

// Return IPv6 and IPv4 in vars respectively for Dual Stack and IPv4/IPv6 in 2nd var for single stack Clusters, and var1 will be nil in those cases
func getNodeIP(oc *exutil.CLI, nodeName string) (string, string) {
	ipStack := checkIPStackType(oc)
	if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
		e2e.Logf("Its a Single Stack Cluster, either IPv4 or IPv6")
		InternalIP, err := oc.AsAdmin().Run("get").Args("node", nodeName, "-o=jsonpath={.status.addresses[?(@.type==\"InternalIP\")].address}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The node's Internal IP is %q", InternalIP)
		return "", InternalIP
	}
	e2e.Logf("Its a Dual Stack Cluster")
	InternalIP1, err := oc.AsAdmin().Run("get").Args("node", nodeName, "-o=jsonpath={.status.addresses[0].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The node's 1st Internal IP is %q", InternalIP1)
	InternalIP2, err := oc.AsAdmin().Run("get").Args("node", nodeName, "-o=jsonpath={.status.addresses[1].address}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The node's 2nd Internal IP is %q", InternalIP2)
	if netutils.IsIPv6String(InternalIP1) {
		return InternalIP1, InternalIP2
	}
	return InternalIP2, InternalIP1
}

// get CLuster Manager's leader info
func getLeaderInfo(oc *exutil.CLI, namespace string, cmName string, networkType string) string {
	if networkType == "ovnkubernetes" {
		nodeName, getNodeErr := exutil.GetFirstWorkerNode(oc)
		o.Expect(getNodeErr).NotTo(o.HaveOccurred())
		o.Expect(nodeName).NotTo(o.BeEmpty())
		podName, getPodNameErr := exutil.GetPodName(oc, namespace, cmName, nodeName)
		o.Expect(getPodNameErr).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.BeEmpty())
		return podName
	}
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "openshift-network-controller", "-n", namespace, "-o=jsonpath={.metadata.annotations.control-plane\\.alpha\\.kubernetes\\.io\\/leader}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	var sdnAnnotations map[string]interface{}
	json.Unmarshal([]byte(output), &sdnAnnotations)
	leaderNodeName := sdnAnnotations["holderIdentity"].(string)
	o.Expect(leaderNodeName).NotTo(o.BeEmpty())
	ocGetPods, podErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-sdn", "pod", "-l app=sdn", "-o=wide").OutputToFile("ocgetpods.txt")
	defer os.RemoveAll(ocGetPods)
	o.Expect(podErr).NotTo(o.HaveOccurred())
	rawGrepOutput, rawGrepErr := exec.Command("bash", "-c", "cat "+ocGetPods+" | grep "+leaderNodeName+" | awk '{print $1}'").Output()
	o.Expect(rawGrepErr).NotTo(o.HaveOccurred())
	leaderPodName := strings.TrimSpace(string(rawGrepOutput))
	e2e.Logf("The leader Pod's name: %v", leaderPodName)
	return leaderPodName
}

func checkSDNMetrics(oc *exutil.CLI, url string, metrics string) {
	var metricsOutput []byte
	var metricsLog []byte
	olmToken, err := exutil.GetSAToken(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(olmToken).NotTo(o.BeEmpty())
	metricsErr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), fmt.Sprintf("%s", url)).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metricsLog, _ = exec.Command("bash", "-c", "cat "+output+" ").Output()
		metricsString := string(metricsLog)
		if strings.Contains(metricsString, "ovnkube_controller_pod") {
			metricsOutput, _ = exec.Command("bash", "-c", "cat "+output+" | grep "+metrics+" | awk 'NR==1{print $2}'").Output()
		} else {
			metricsOutput, _ = exec.Command("bash", "-c", "cat "+output+" | grep "+metrics+" | awk 'NR==3{print $2}'").Output()
		}
		metricsValue := strings.TrimSpace(string(metricsOutput))
		if metricsValue != "" {
			e2e.Logf("The output of the metrics for %s is : %v", metrics, metricsValue)
		} else {
			e2e.Logf("Can't get metrics for %s:", metrics)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))
}

func getEgressCIDRs(oc *exutil.CLI, node string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node, "-o=jsonpath={.egressCIDRs}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("egressCIDR for hostsubnet node %v is: %v", node, output)
	return output
}

// get egressIP from a node
// When they are multiple egressIPs on the node, egressIp list is in format of ["10.0.247.116","10.0.156.51"]
// as an example from the output of command "oc get hostsubnet <node> -o=jsonpath={.egressIPs}"
// convert the iplist into an array of ip addresses
func getEgressIPByKind(oc *exutil.CLI, kind string, kindName string, expectedNum int) ([]string, error) {
	var ip = []string{}
	iplist, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, kindName, "-o=jsonpath={.egressIPs}").Output()
	isIPListEmpty := (iplist == "" || iplist == "[]")
	if expectedNum == 0 {
		// Add waiting time for egressIP removed
		egressIPEmptyErr := wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			iplist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, kindName, "-o=jsonpath={.egressIPs}").Output()
			if iplist == "" || iplist == "[]" {
				e2e.Logf("EgressIP list is empty")
				return true, nil
			}
			e2e.Logf("EgressIP list is %s, not removed, or have err:%v, and try next round", iplist, err)
			return false, nil
		})
		return ip, egressIPEmptyErr
	}
	if !isIPListEmpty && iplist != "[]" {
		ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
	}
	if isIPListEmpty || len(ip) < expectedNum || err != nil {
		err = wait.Poll(30*time.Second, 5*time.Minute, func() (bool, error) {
			iplist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(kind, kindName, "-o=jsonpath={.egressIPs}").Output()
			if len(iplist) > 0 && iplist != "[]" {
				ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
			}
			if len(ip) < expectedNum || err != nil {
				e2e.Logf("only got %d egressIP, or have err:%v, and try next round", len(ip), err)
				return false, nil
			}
			if len(iplist) > 0 && len(ip) == expectedNum {
				e2e.Logf("Found egressIP list for %v %v is: %v", kind, kindName, iplist)
				return true, nil
			}
			return false, nil
		})
		e2e.Logf("Only got %d egressIP, or have err:%v", len(ip), err)
		return ip, err
	}
	return ip, nil
}

func getPodName(oc *exutil.CLI, namespace string, label string) []string {
	var podName []string
	podNameAll, err := oc.AsAdmin().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podName = strings.Split(podNameAll, " ")
	e2e.Logf("The pod(s) are  %v ", podName)
	return podName
}

// starting from first node, compare its subnet with subnet of subsequent nodes in the list
// until two nodes with same subnet found, otherwise, return false to indicate that no two nodes with same subnet found
func findTwoNodesWithSameSubnet(oc *exutil.CLI, nodeList *v1.NodeList) (bool, [2]string) {
	var nodes [2]string
	for i := 0; i < (len(nodeList.Items) - 1); i++ {
		for j := i + 1; j < len(nodeList.Items); j++ {
			firstSub := getIfaddrFromNode(nodeList.Items[i].Name, oc)
			secondSub := getIfaddrFromNode(nodeList.Items[j].Name, oc)
			if firstSub == secondSub {
				e2e.Logf("Found nodes with same subnet.")
				nodes[0] = nodeList.Items[i].Name
				nodes[1] = nodeList.Items[j].Name
				return true, nodes
			}
		}
	}
	return false, nodes
}

func getSDNMetrics(oc *exutil.CLI, podName string) string {
	var metricsLog string
	metricsErr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-sdn", fmt.Sprintf("%s", podName), "--", "curl", "localhost:29100/metrics").OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metricsLog = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))
	return metricsLog
}

func getOVNMetrics(oc *exutil.CLI, url string) string {
	var metricsLog string
	olmToken, err := exutil.GetSAToken(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(olmToken).NotTo(o.BeEmpty())
	metricsErr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), fmt.Sprintf("%s", url)).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metricsLog = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))
	return metricsLog
}

func checkIPsec(oc *exutil.CLI) string {
	output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.defaultNetwork.ovnKubernetesConfig.ipsecConfig}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The ipsec state is === %v ===", output)
	e2e.Logf("{} means ipsec is enabled, empty means ipsec is disabled")
	return output
}

func getAssignedEIPInEIPObject(oc *exutil.CLI, egressIPObject string) []map[string]string {
	timeout := estimateTimeoutForEgressIP(oc)
	var egressIPs string
	egressipErr := wait.Poll(10*time.Second, timeout, func() (bool, error) {
		egressIPStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", egressIPObject, "-ojsonpath={.status.items}").Output()
		if err != nil {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		if egressIPStatus == "" {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		egressIPs = egressIPStatus
		e2e.Logf("egressIPStatus: %v", egressIPs)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to apply egressIPs:%s", egressipErr))

	var egressIPJsonMap []map[string]string
	json.Unmarshal([]byte(egressIPs), &egressIPJsonMap)
	e2e.Logf("egressIPJsonMap:%v", egressIPJsonMap)
	return egressIPJsonMap
}

func rebootNode(oc *exutil.CLI, nodeName string) {
	e2e.Logf("\nRebooting node %s....", nodeName)
	_, err1 := exutil.DebugNodeWithChroot(oc, nodeName, "shutdown", "-r", "+1")
	o.Expect(err1).NotTo(o.HaveOccurred())
}

func checkNodeStatus(oc *exutil.CLI, nodeName string, expectedStatus string) {
	var expectedStatus1 string
	if expectedStatus == "Ready" {
		expectedStatus1 = "True"
	} else if expectedStatus == "NotReady" {
		expectedStatus1 = "Unknown"
	} else {
		err1 := fmt.Errorf("TBD supported node status")
		o.Expect(err1).NotTo(o.HaveOccurred())
	}
	err := wait.Poll(5*time.Second, 15*time.Minute, func() (bool, error) {
		statusOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", nodeName, "-ojsonpath={.status.conditions[-1].status}").Output()
		if err != nil {
			e2e.Logf("\nGet node status with error : %v", err)
			return false, nil
		}
		e2e.Logf("Expect Node %s in state %v, kubelet status is %s", nodeName, expectedStatus, statusOutput)
		if statusOutput != expectedStatus1 {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Node %s is not in expected status %s", nodeName, expectedStatus))
}

func updateEgressIPObject(oc *exutil.CLI, egressIPObjectName string, egressIP string) {
	patchResourceAsAdmin(oc, "egressip/"+egressIPObjectName, "{\"spec\":{\"egressIPs\":[\""+egressIP+"\"]}}")
	egressipErr := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("egressip", egressIPObjectName, "-o=jsonpath={.status.items[*]}").Output()
		if err != nil {
			e2e.Logf("Wait to get EgressIP object applied,try next round. %v", err)
			return false, nil
		}
		if !strings.Contains(output, egressIP) {
			e2e.Logf("Wait for new IP %s applied,try next round.", egressIP)
			e2e.Logf(output)
			return false, nil
		}
		e2e.Logf(output)
		return true, nil
	})
	exutil.AssertWaitPollNoErr(egressipErr, fmt.Sprintf("Failed to apply new egressIP %s:%v", egressIP, egressipErr))
}

func getTwoNodesSameSubnet(oc *exutil.CLI, nodeList *v1.NodeList) (bool, []string) {
	var egressNodes []string
	if len(nodeList.Items) < 2 {
		e2e.Logf("Not enough nodes available for the test, skip the case!!")
		return false, nil
	}
	platform := exutil.CheckPlatform(oc)
	if strings.Contains(platform, "aws") {
		e2e.Logf("find the two nodes that have same subnet")
		check, nodes := findTwoNodesWithSameSubnet(oc, nodeList)
		if check {
			egressNodes = nodes[:2]
		} else {
			e2e.Logf("No more than 2 worker nodes in same subnet, skip the test!!!")
			return false, nil
		}
	} else {
		e2e.Logf("since worker nodes all have same subnet, just pick first two nodes as egress nodes")
		egressNodes = append(egressNodes, nodeList.Items[0].Name)
		egressNodes = append(egressNodes, nodeList.Items[1].Name)
	}
	return true, egressNodes
}

/*
getSvcIP returns IPv6 and IPv4 in vars in order on dual stack respectively and main Svc IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var.
LoadBalancer svc will return Ingress VIP in var1, v4 or v6 and NodePort svc will return Ingress SvcIP in var1 and NodePort in var2
*/
func getSvcIP(oc *exutil.CLI, namespace string, svcName string) (string, string) {
	ipStack := checkIPStackType(oc)
	svctype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.type}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	ipFamilyType, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ipFamilyPolicy}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if (svctype == "ClusterIP") || (svctype == "NodePort") {
		if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
			svcIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if svctype == "ClusterIP" {
				e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIP)
				return svcIP, ""
			}
			nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The NodePort service %s IP and NodePort in namespace %s is %s %s", svcName, namespace, svcIP, nodePort)
			return svcIP, nodePort

		} else if (ipStack == "dualstack" && ipFamilyType == "PreferDualStack") || (ipStack == "dualstack" && ipFamilyType == "RequireDualStack") {
			ipFamilyPrecedence, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ipFamilies[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			//if IPv4 is listed first in ipFamilies then clustrIPs allocation will take order as Ipv4 first and then Ipv6 else reverse
			svcIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIPv4)
			svcIPv6, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIPv6)
			/*As stated Nodeport type svc will return node port value in 2nd var. We don't care about what svc address is coming in 1st var as we evetually going to get
			node IPs later and use that in curl operation to node_ip:nodeport*/
			if ipFamilyPrecedence == "IPv4" {
				e2e.Logf("The ipFamilyPrecedence is Ipv4, Ipv6")
				switch svctype {
				case "NodePort":
					nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					e2e.Logf("The Dual Stack NodePort service %s IP and NodePort in namespace %s is %s %s", svcName, namespace, svcIPv4, nodePort)
					return svcIPv4, nodePort
				default:
					return svcIPv6, svcIPv4
				}
			} else {
				e2e.Logf("The ipFamilyPrecedence is Ipv6, Ipv4")
				switch svctype {
				case "NodePort":
					nodePort, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.ports[*].nodePort}").Output()
					o.Expect(err).NotTo(o.HaveOccurred())
					e2e.Logf("The Dual Stack NodePort service %s IP and NodePort in namespace %s is %s %s", svcName, namespace, svcIPv6, nodePort)
					return svcIPv6, nodePort
				default:
					svcIPv4, svcIPv6 = svcIPv6, svcIPv4
					return svcIPv6, svcIPv4
				}
			}
		} else {
			//Its a Dual Stack Cluster with SingleStack ipFamilyPolicy
			svcIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The service %s IP in namespace %s is %q", svcName, namespace, svcIP)
			return svcIP, ""
		}
	} else {
		//Loadbalancer will be supported for single stack Ipv4 here for mostly GCP,Azure. We can take further enhancements wrt Metal platforms in Metallb utils later
		e2e.Logf("The serviceType is LoadBalancer")
		platform := exutil.CheckPlatform(oc)
		var jsonString string
		if platform == "aws" {
			jsonString = "-o=jsonpath={.status.loadBalancer.ingress[0].hostname}"
		} else {
			jsonString = "-o=jsonpath={.status.loadBalancer.ingress[0].ip}"
		}

		err := wait.Poll(30*time.Second, 300*time.Second, func() (bool, error) {
			svcIP, er := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, jsonString).Output()
			o.Expect(er).NotTo(o.HaveOccurred())
			if svcIP == "" {
				e2e.Logf("Waiting for lb service IP assignment. Trying again...")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to assign lb svc IP to %v", svcName))
		lbSvcIP, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, jsonString).Output()
		e2e.Logf("The %s lb service Ingress VIP in namespace %s is %q", svcName, namespace, lbSvcIP)
		return lbSvcIP, ""
	}
}

// getPodIP returns IPv6 and IPv4 in vars in order on dual stack respectively and main IP in case of single stack (v4 or v6) in 1st var, and nil in 2nd var
func getPodIP(oc *exutil.CLI, namespace string, podName string) (string, string) {
	ipStack := checkIPStackType(oc)
	if (ipStack == "ipv6single") || (ipStack == "ipv4single") {
		podIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIP)
		return podIP, ""
	} else if ipStack == "dualstack" {
		podIP1, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod's %s 1st IP in namespace %s is %q", podName, namespace, podIP1)
		podIP2, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod's %s 2nd IP in namespace %s is %q", podName, namespace, podIP2)
		if netutils.IsIPv6String(podIP1) {
			e2e.Logf("This is IPv6 primary dual stack cluster")
			return podIP1, podIP2
		}
		return podIP2, podIP1
	}
	return "", ""
}

// CurlPod2PodPass checks connectivity across pods regardless of network addressing type on cluster
func CurlPod2PodPass(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CurlPod2PodFail ensures no connectivity from a pod to pod regardless of network addressing type on cluster
func CurlPod2PodFail(oc *exutil.CLI, namespaceSrc string, podNameSrc string, namespaceDst string, podNameDst string) {
	podIP1, podIP2 := getPodIP(oc, namespaceDst, podNameDst)
	if podIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP2, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(podIP1, "8080"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

// CurlNode2PodPass checks node to pod connectivity regardless of network addressing type on cluster
func CurlNode2PodPass(oc *exutil.CLI, nodeName string, namespace string, podName string) {
	//getPodIP returns IPv6 and IPv4 in order on dual stack in PodIP1 and PodIP2 respectively and main IP in case of single stack (v4 or v6) in PodIP1, and nil in PodIP2
	podIP1, podIP2 := getPodIP(oc, namespace, podName)
	if podIP2 != "" {
		podv6URL := net.JoinHostPort(podIP1, "8080")
		podv4URL := net.JoinHostPort(podIP2, "8080")
		_, err := exutil.DebugNode(oc, nodeName, "curl", podv4URL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNode(oc, nodeName, "curl", podv6URL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		podURL := net.JoinHostPort(podIP1, "8080")
		_, err := exutil.DebugNode(oc, nodeName, "curl", podURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CurlNode2SvcPass checks node to svc connectivity regardless of network addressing type on cluster
func CurlNode2SvcPass(oc *exutil.CLI, nodeName string, namespace string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespace, svcName)
	if svcIP2 != "" {
		svc6URL := net.JoinHostPort(svcIP1, "27017")
		svc4URL := net.JoinHostPort(svcIP2, "27017")
		_, err := exutil.DebugNode(oc, nodeName, "curl", svc4URL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = exutil.DebugNode(oc, nodeName, "curl", svc6URL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		svcURL := net.JoinHostPort(svcIP1, "27017")
		_, err := exutil.DebugNode(oc, nodeName, "curl", svcURL, "-s", "--connect-timeout", "5")
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CurlNode2SvcFail checks node to svc connectivity regardless of network addressing type on cluster
func CurlNode2SvcFail(oc *exutil.CLI, nodeName string, namespace string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespace, svcName)
	if svcIP2 != "" {
		svc6URL := net.JoinHostPort(svcIP1, "27017")
		svc4URL := net.JoinHostPort(svcIP2, "27017")
		output, _ := exutil.DebugNode(oc, nodeName, "curl", svc4URL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("Failed")))
		output, _ = exutil.DebugNode(oc, nodeName, "curl", svc6URL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("Failed")))
	} else {
		svcURL := net.JoinHostPort(svcIP1, "27017")
		output, _ := exutil.DebugNode(oc, nodeName, "curl", svcURL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("Failed")))
	}
}

// CurlPod2SvcPass checks pod to svc connectivity regardless of network addressing type on cluster
func CurlPod2SvcPass(oc *exutil.CLI, namespaceSrc string, namespaceSvc string, podNameSrc string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespaceSvc, svcName)
	if svcIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// CurlPod2SvcFail ensures no connectivity from a pod to svc regardless of network addressing type on cluster
func CurlPod2SvcFail(oc *exutil.CLI, namespaceSrc string, namespaceSvc string, podNameSrc string, svcName string) {
	svcIP1, svcIP2 := getSvcIP(oc, namespaceSvc, svcName)
	if svcIP2 != "" {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).To(o.HaveOccurred())
		_, err = e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP2, "27017"))
		o.Expect(err).To(o.HaveOccurred())
	} else {
		_, err := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl --connect-timeout 5 -s "+net.JoinHostPort(svcIP1, "27017"))
		o.Expect(err).To(o.HaveOccurred())
	}
}

func checkProxy(oc *exutil.CLI) bool {
	httpProxy, err := doAction(oc, "get", true, true, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	httpsProxy, err := doAction(oc, "get", true, true, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
	o.Expect(err).NotTo(o.HaveOccurred())
	if httpProxy != "" || httpsProxy != "" {
		return true
	}
	return false
}

// SDNHostwEgressIP find out which egress node has the egressIP
func SDNHostwEgressIP(oc *exutil.CLI, node []string, egressip string) string {
	var ip []string
	var foundHost string
	for i := 0; i < len(node); i++ {
		iplist, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node[i], "-o=jsonpath={.egressIPs}").Output()
		e2e.Logf("iplist for node %v: %v", node, iplist)
		if iplist != "" && iplist != "[]" {
			ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
		}
		if iplist == "" || iplist == "[]" || err != nil {
			err = wait.Poll(30*time.Second, 3*time.Minute, func() (bool, error) {
				iplist, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", node[i], "-o=jsonpath={.egressIPs}").Output()
				if iplist != "" && iplist != "[]" {
					e2e.Logf("Found egressIP list for node %v is: %v", node, iplist)
					ip = strings.Split(iplist[2:len(iplist)-2], "\",\"")
					return true, nil
				}
				if err != nil {
					e2e.Logf("only got %d egressIP, or have err:%v, and try next round", len(ip), err)
					return false, nil
				}
				return false, nil
			})
		}
		if isValueInList(egressip, ip) {
			foundHost = node[i]
			break
		}
	}
	return foundHost
}

func isValueInList(value string, list []string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// getPodMultiNetwork is designed to get both v4 and v6 addresses from pod's secondary interface(net1) which is not in the cluster's SDN or OVN network
// currently the v4 address of pod's secondary interface is always displyed before v6 address no matter the order configred in the net-attach-def YAML file
func getPodMultiNetwork(oc *exutil.CLI, namespace string, podName string) (string, string) {
	cmd1 := "ip a sho net1 | awk 'NR==3{print $2}' |grep -Eo '((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])'"
	cmd2 := "ip a sho net1 | awk 'NR==5{print $2}' |grep -Eo '([A-Fa-f0-9]{1,4}::?){1,7}[A-Fa-f0-9]{1,4}'"
	podIPv4, err := e2eoutput.RunHostCmd(namespace, podName, cmd1)
	o.Expect(err).NotTo(o.HaveOccurred())
	pod2ns1IPv4 := strings.TrimSpace(podIPv4)
	podIPv6, err1 := e2eoutput.RunHostCmd(namespace, podName, cmd2)
	o.Expect(err1).NotTo(o.HaveOccurred())
	pod2ns1IPv6 := strings.TrimSpace(podIPv6)
	return pod2ns1IPv4, pod2ns1IPv6
}

// Pinging pod's secondary interfaces should pass
func curlPod2PodMultiNetworkPass(oc *exutil.CLI, namespaceSrc string, podNameSrc string, podIPv4 string, podIPv6 string) {
	err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
		msg, _ := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl  "+podIPv4+":8080  --connect-timeout 5")
		if !strings.Contains(msg, "Hello OpenShift!") {
			e2e.Logf("The curl should pass but fail, and try next round")
			return false, nil
		}
		return true, nil
	})
	//MultiNetworkPolicy not support ipv6 yet, disabel ipv6 curl right now
	//msg1, _ := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl -g -6 [" +podIPv6+ "]:8080  --connect-timeout 5")
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test fail with err:%s", err))
}

// Pinging pod's secondary interfaces should fail
func curlPod2PodMultiNetworkFail(oc *exutil.CLI, namespaceSrc string, podNameSrc string, podIPv4 string, podIPv6 string) {
	err := wait.Poll(2*time.Second, 30*time.Second, func() (bool, error) {
		msg, _ := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl  "+podIPv4+":8080  --connect-timeout 5")
		if strings.Contains(msg, "Hello OpenShift!") {
			e2e.Logf("The curl should fail but pass, and try next round")
			return false, nil
		}
		return true, nil
	})
	//MultiNetworkPolicy not support ipv6 yet, disabel ipv6 curl right now
	//msg1, _ := e2eoutput.RunHostCmd(namespaceSrc, podNameSrc, "curl -g -6 [" +podIPv6+ "]:8080  --connect-timeout 5")
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Test fail with err:%s", err))
}

// This function will bring 2 namespaces, 5 pods and 2 NADs for all multus multinetworkpolicy cases
func prepareMultinetworkTest(oc *exutil.CLI, ns1 string, ns2 string, patchInfo string) {

	buildPruningBaseDir := exutil.FixturePath("testdata", "networking/multinetworkpolicy")
	netAttachDefFile1 := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-NAD1.yaml")
	netAttachDefFile2 := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-NAD2.yaml")
	pingPodTemplate := filepath.Join(buildPruningBaseDir, "MultiNetworkPolicy-pod-template.yaml")
	patchSResource := "networks.operator.openshift.io/cluster"

	g.By("Enable MacvlanNetworkpolicy in the cluster")
	patchResourceAsAdmin(oc, patchSResource, patchInfo)

	g.By("Create first namespace")
	nserr1 := oc.Run("new-project").Args(ns1).Execute()
	o.Expect(nserr1).NotTo(o.HaveOccurred())
	_, proerr1 := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns1, "user="+ns1).Output()
	o.Expect(proerr1).NotTo(o.HaveOccurred())

	g.By("Create MultiNetworkPolicy-NAD1 in ns1")
	err1 := oc.AsAdmin().Run("create").Args("-f", netAttachDefFile1, "-n", ns1).Execute()
	o.Expect(err1).NotTo(o.HaveOccurred())
	output, err2 := oc.Run("get").Args("net-attach-def", "-n", ns1).Output()
	o.Expect(err2).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("macvlan-nad1"))

	g.By("Create 1st pod in ns1")
	pod1ns1 := testPodMultinetwork{
		name:      "blue-pod-1",
		namespace: ns1,
		nodename:  "worker-0",
		nadname:   "macvlan-nad1",
		labelname: "blue-openshift",
		template:  pingPodTemplate,
	}
	pod1ns1.createTestPodMultinetwork(oc)
	waitPodReady(oc, pod1ns1.namespace, pod1ns1.name)

	g.By("Create second pod in ns1")
	pod2ns1 := testPodMultinetwork{
		name:      "blue-pod-2",
		namespace: ns1,
		nodename:  "worker-1",
		nadname:   "macvlan-nad1",
		labelname: "blue-openshift",
		template:  pingPodTemplate,
	}
	pod2ns1.createTestPodMultinetwork(oc)
	waitPodReady(oc, pod2ns1.namespace, pod2ns1.name)

	g.By("Create third pod in ns1")
	pod3ns1 := testPodMultinetwork{
		name:      "red-pod-1",
		namespace: ns1,
		nodename:  "worker-0",
		nadname:   "macvlan-nad1",
		labelname: "red-openshift",
		template:  pingPodTemplate,
	}
	pod3ns1.createTestPodMultinetwork(oc)
	waitPodReady(oc, pod3ns1.namespace, pod3ns1.name)

	g.By("Create second namespace")
	nserr2 := oc.Run("new-project").Args(ns2).Execute()
	o.Expect(nserr2).NotTo(o.HaveOccurred())
	_, proerr2 := oc.AsAdmin().WithoutNamespace().Run("label").Args("ns", ns2, "user="+ns2).Output()
	o.Expect(proerr2).NotTo(o.HaveOccurred())

	g.By("Create MultiNetworkPolicy-NAD2 in ns2")
	err4 := oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", netAttachDefFile2, "-n", ns2).Execute()
	o.Expect(err4).NotTo(o.HaveOccurred())
	output, err5 := oc.Run("get").Args("net-attach-def", "-n", ns2).Output()
	o.Expect(err5).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring("macvlan-nad2"))

	g.By("Create 1st pod in ns2")
	pod1ns2 := testPodMultinetwork{
		name:      "blue-pod-3",
		namespace: ns2,
		nodename:  "worker-0",
		nadname:   "macvlan-nad2",
		labelname: "blue-openshift",
		template:  pingPodTemplate,
	}
	pod1ns2.createTestPodMultinetwork(oc)
	waitPodReady(oc, pod1ns2.namespace, pod1ns2.name)

	g.By("Create second pod in ns2")
	pod2ns2 := testPodMultinetwork{
		name:      "red-pod-2",
		namespace: ns2,
		nodename:  "worker-0",
		nadname:   "macvlan-nad2",
		labelname: "red-openshift",
		template:  pingPodTemplate,
	}
	pod2ns2.createTestPodMultinetwork(oc)
	waitPodReady(oc, pod2ns2.namespace, pod2ns2.name)
}

// check if an ip address is added to node's NIC, or removed from node's NIC
func checkPrimaryNIC(oc *exutil.CLI, nodeName string, ip string, flag bool) {
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, err := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", "/usr/sbin/ip -4 -brief address show")
		if err != nil {
			e2e.Logf("Cannot get primary NIC interface, errors: %v, try again", err)
			return false, nil
		}
		if flag && !strings.Contains(output, ip) {
			e2e.Logf("egressIP has not been added to node's NIC correctly, try again")
			return false, nil
		}
		if !flag && strings.Contains(output, ip) {
			e2e.Logf("egressIP has not been removed from node's NIC correctly, try again")
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("Failed to get NIC on the host:%s", checkErr))
}

func checkEgressIPonSDNHost(oc *exutil.CLI, node string, expectedEgressIP []string) {
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		ip, err := getEgressIPByKind(oc, "hostsubnet", node, len(expectedEgressIP))
		if err != nil {
			e2e.Logf("\n got the error: %v\n, try again", err)
			return false, nil
		}
		if !unorderedEqual(ip, expectedEgressIP) {
			e2e.Logf("\n got egressIP as %v while expected egressIP is %v, try again", ip, expectedEgressIP)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("Failed to get egressIP on the host:%s", checkErr))
}

func unorderedEqual(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for _, value := range first {
		if !contains(second, value) {
			return false
		}
	}
	return true
}

func checkovnkubeMasterNetworkProgrammingetrics(oc *exutil.CLI, url string, metrics string) {
	var metricsOutput []byte
	olmToken, err := exutil.GetSAToken(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(olmToken).NotTo(o.BeEmpty())
	metricsErr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), fmt.Sprintf("%s", url)).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		metricsOutput, _ = exec.Command("bash", "-c", "cat "+output+" | grep "+metrics+" | awk 'NR==2{print $2}'").Output()
		metricsValue := strings.TrimSpace(string(metricsOutput))
		if metricsValue != "" {
			e2e.Logf("The output of the metrics for %s is : %v", metrics, metricsValue)
		} else {
			e2e.Logf("Can't get metrics for %s:", metrics)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))
}

func getControllerManagerLeaderIP(oc *exutil.CLI, namespace string, cmName string) string {
	leaderJSONPayload, getJSONPayloadErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", cmName, "-n", namespace, "-o=jsonpath={.metadata.annotations.control-plane\\.alpha\\.kubernetes\\.io/leader}").Output()
	o.Expect(getJSONPayloadErr).NotTo(o.HaveOccurred())
	holderID, parseHolderIDErr := exec.Command("bash", "-c", fmt.Sprintf("echo '%s' | jq -r .holderIdentity", leaderJSONPayload)).Output()
	o.Expect(parseHolderIDErr).NotTo(o.HaveOccurred())
	leaderPodName := strings.TrimSuffix(string(holderID), "\n")
	o.Expect(leaderPodName).ShouldNot(o.BeEmpty(), "leader pod name is empty")
	e2e.Logf("The leader pod name is %s", leaderPodName)
	leaderPodIP := getPodIPv4(oc, namespace, leaderPodName)
	return leaderPodIP
}

func describeCheckEgressIPByKind(oc *exutil.CLI, kind string, kindName string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args(kind, kindName).Output()

	o.Expect(err).NotTo(o.HaveOccurred())
	egressIPReg, _ := regexp.Compile(".*Egress IPs.*")
	egressIPStr := egressIPReg.FindString(output)
	egressIPArr := strings.Split(egressIPStr, ":")

	//remove whitespace in front of the ip address
	ip := strings.TrimSpace(egressIPArr[1])
	e2e.Logf("get egressIP from oc describe %v %v: --->%s<---", kind, kindName, ip)
	return ip
}

func findUnUsedIPsOnNodeOrFail(oc *exutil.CLI, nodeName, cidr string, expectedNum int) []string {
	freeIPs := findUnUsedIPsOnNode(oc, nodeName, cidr, expectedNum)
	o.Expect(len(freeIPs) == expectedNum).Should(o.BeTrue())
	return freeIPs
}

func (pod *externalIPPod) createExternalIPPod(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", pod.template, "-p", "NAME="+pod.name, "NAMESPACE="+pod.namespace)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create the externalIP pod %v", pod.name))
}

func checkParameter(oc *exutil.CLI, namespace string, kind string, kindName string, parameter string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, kind, kindName, parameter).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return output
}

func patchReplaceResourceAsAdmin(oc *exutil.CLI, ns, resource, rsname, patch string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, rsname, "--type=json", "-p", patch, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// For SingleStack function returns IPv6 or IPv4 hostsubnet in case OVN
// For SDN plugin returns only IPv4 hostsubnet
// Dual stack not supported on openshiftSDN
// IPv6 single stack not supported on openshiftSDN
func getNodeSubnet(oc *exutil.CLI, nodeName string) string {

	networkType := checkNetworkType(oc)

	if networkType == "ovnkubernetes" {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.annotations.k8s\\.ovn\\.org/node-subnets}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var data map[string]interface{}
		json.Unmarshal([]byte(output), &data)
		hostSubnets := data["default"].([]interface{})
		hostSubnet := hostSubnets[0].(string)
		return hostSubnet
	}
	nodeSubnet, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("hostsubnet", nodeName, "-o=jsonpath={.subnet}").Output()
	o.Expect(nodeSubnet).NotTo(o.BeEmpty())
	o.Expect(err1).NotTo(o.HaveOccurred())
	return nodeSubnet

}

func getNodeSubnetDualStack(oc *exutil.CLI, nodeName string) (string, string) {

	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.annotations.k8s\\.ovn\\.org/node-subnets}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	var data map[string]interface{}
	json.Unmarshal([]byte(output), &data)
	hostSubnets := data["default"].([]interface{})
	hostSubnetIPv4 := hostSubnets[0].(string)
	hostSubnetIPv6 := hostSubnets[1].(string)

	e2e.Logf("Host subnet is %v and %v", hostSubnetIPv4, hostSubnetIPv6)

	return hostSubnetIPv4, hostSubnetIPv6
}

func getIPv4Capacity(oc *exutil.CLI, nodeName string) string {
	ipv4Capacity := ""
	egressIPConfig, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.annotations.cloud\\.network\\.openshift\\.io/egress-ipconfig}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The egressipconfig is %v \n", egressIPConfig)
	switch exutil.CheckPlatform(oc) {
	case "aws":
		ipv4Capacity = strings.Split(strings.Split(egressIPConfig, ":")[5], ",")[0]
	case "gcp":
		ipv4Capacity = strings.Split(egressIPConfig, ":")[5]
		ipv4Capacity = ipv4Capacity[:len(ipv4Capacity)-3]
	default:
		e2e.Logf("Not support cloud provider for auto egressip cases for now.")
		g.Skip("Not support cloud provider for auto egressip cases for now.")
	}

	return ipv4Capacity
}

func (aclSettings *aclSettings) getJSONString() string {
	jsonACLSetting, _ := json.Marshal(aclSettings)
	annotationString := "k8s.ovn.org/acl-logging=" + string(jsonACLSetting)
	return annotationString
}

func enableACLOnNamespace(oc *exutil.CLI, namespace, denyLevel, allowLevel string) {
	e2e.Logf("Enable ACL looging on the namespace %s", namespace)
	aclSettings := aclSettings{DenySetting: denyLevel, AllowSetting: allowLevel}
	err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("--overwrite", "ns", namespace, aclSettings.getJSONString()).Execute()
	o.Expect(err1).NotTo(o.HaveOccurred())
}

func disableACLOnNamespace(oc *exutil.CLI, namespace string) {
	e2e.Logf("Disable ACL looging on the namespace %s", namespace)
	err1 := oc.AsAdmin().WithoutNamespace().Run("annotate").Args("ns", namespace, "k8s.ovn.org/acl-logging-").Execute()
	o.Expect(err1).NotTo(o.HaveOccurred())
}

func getNodeMacAddress(oc *exutil.CLI, nodeName string) string {
	networkType := checkNetworkType(oc)
	var macAddress string
	if networkType == "ovnkubernetes" {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", nodeName, "-o=jsonpath={.metadata.annotations.k8s\\.ovn\\.org/l3-gateway-config}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		var data map[string]interface{}
		json.Unmarshal([]byte(output), &data)
		l3GatewayConfigAnnotations := data["default"].(interface{})
		l3GatewayConfigAnnotationsJSON := l3GatewayConfigAnnotations.(map[string]interface{})
		macAddress = l3GatewayConfigAnnotationsJSON["mac-address"].(string)
		return macAddress
	}
	macAddress, err1 := exutil.DebugNodeWithOptionsAndChroot(oc, nodeName, []string{"-q"}, "bin/sh", "-c", "cat /sys/class/net/br0/address")
	o.Expect(err1).NotTo(o.HaveOccurred())
	return macAddress
}

// check if an env is in a configmap in specific namespace [usage: checkConfigMap(oc, namesapce, configmapName, envString)]
func checkEnvInConfigMap(oc *exutil.CLI, ns, configmapName string, envString string) error {
	err := checkConfigMap(oc, ns, configmapName)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("cm %v is not found in namespace %v", configmapName, ns))

	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmap", "-n", ns, configmapName, "-oyaml").Output()
		if err != nil {
			e2e.Logf("Failed to get configmap %v, error: %s. Trying again", configmapName, err)
			return false, nil
		}
		if !strings.Contains(output, envString) {
			e2e.Logf("Did not find %v in ovnkube-config configmap,try next round.", envString)
			return false, nil
		}
		return true, nil
	})
	return checkErr
}

// check if certain log message is in a pod in specific namespace
func checkLogMessageInPod(oc *exutil.CLI, namespace string, containerName string, podName string, filter string) (string, error) {
	var podLogs string
	var err, checkErr error
	checkErr = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		podLogs, err = exutil.GetSpecificPodLogs(oc, namespace, containerName, podName, filter)
		if len(podLogs) == 0 || err != nil {
			e2e.Logf("did not get expected podLogs: %v, or have err:%v, try again", podLogs, err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("fail to get expected log in pod %v, err: %v", podName, checkErr))
	return podLogs, nil
}

// get OVN-Kubernetes management interface (ovn-k8s-mp0) IP for the node
func getOVNK8sNodeMgmtIPv4(oc *exutil.CLI, nodeName string) string {
	var output string
	var err error
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, err = exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", "/usr/sbin/ip -4 -brief address show | grep ovn-k8s-mp0")
		if output == "" || err != nil {
			e2e.Logf("Did not get node's management interface, errors: %v, try again", err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("fail to get management interface for node %v, err: %v", nodeName, checkErr))

	e2e.Logf("Match out the OVN-Kubernetes management IP address for the node")
	re := regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	nodeOVNK8sMgmtIP := re.FindAllString(output, -1)[0]
	e2e.Logf("Got ovn-k8s management interface IP for node %v as: %v", nodeName, nodeOVNK8sMgmtIP)
	return nodeOVNK8sMgmtIP
}

// findLogFromPod will search logs for a specific string in the specific container of the pod or just the pod
func findLogFromPod(oc *exutil.CLI, searchString string, namespace string, podLabel string, podContainer ...string) bool {
	findLog := false
	podNames := getPodName(oc, namespace, podLabel)
	var cargs []string
	for _, podName := range podNames {
		if len(podContainer) > 0 {
			cargs = []string{podName, "-c", podContainer[0], "-n", namespace}
		} else {
			cargs = []string{podName, "-n", namespace}
		}
		output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(cargs...).OutputToFile("podlog")
		o.Expect(err).NotTo(o.HaveOccurred())
		grepOutput, err := exec.Command("bash", "-c", "cat "+output+" | grep -i '"+searchString+"' | wc -l").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		grepOutputString := strings.TrimSpace(string(grepOutput))
		if grepOutputString != "0" {
			e2e.Logf("Found the '%s' string in %s number of lines.", searchString, grepOutputString)
			findLog = true
			break
		}
	}
	return findLog
}

// searchOVNDBForSpecCmd This is used for lr-policy-list and snat rules check in ovn db.
func searchOVNDBForSpecCmd(oc *exutil.CLI, cmd, searchKeyword string, times int) error {
	ovnPod := getOVNKMasterOVNkubeNode(oc)
	o.Expect(ovnPod).ShouldNot(o.Equal(""))
	var cmdOutput string
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		output, cmdErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, cmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try next ...,", cmdErr)
			return false, nil
		}
		cmdOutput = output
		if strings.Count(output, searchKeyword) == times {
			return true, nil
		}
		return false, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The command check result in ovndb is not expected ! See below output \n %s ", cmdOutput)
	}
	return checkOVNDbErr
}

// waitEgressFirewallApplied Wait egressfirewall applied
func waitEgressFirewallApplied(oc *exutil.CLI, efName, ns string) error {
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, efErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressfirewall", "-n", ns, efName).Output()
		if efErr != nil {
			e2e.Logf("Failed to get egressfirewall %v, error: %s. Trying again", efName, efErr)
			return false, nil
		}
		if !strings.Contains(output, "EgressFirewall Rules applied") {
			e2e.Logf("The egressfirewall was not applied, trying again. \n %s", output)
			return false, nil
		}
		return true, nil
	})
	return checkErr
}

// switchOVNGatewayMode will switch to requested mode, shared or local
func switchOVNGatewayMode(oc *exutil.CLI, mode string) {
	currentMode := getOVNGatewayMode(oc)
	if currentMode == "local" && mode == "shared" {
		e2e.Logf("Migrating cluster to shared gateway mode")
		patchResourceAsAdmin(oc, "network.operator/cluster", "{\"spec\":{\"defaultNetwork\":{\"ovnKubernetesConfig\":{\"gatewayConfig\":{\"routingViaHost\": false}}}}}")
	} else if currentMode == "shared" && mode == "local" {
		e2e.Logf("Migrating cluster to Local gw mode")
		patchResourceAsAdmin(oc, "network.operator/cluster", "{\"spec\":{\"defaultNetwork\":{\"ovnKubernetesConfig\":{\"gatewayConfig\":{\"routingViaHost\": true}}}}}")
	} else {
		e2e.Logf("Cluster is already on requested gateway mode")
	}
	_, err := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", "openshift-ovn-kubernetes", "ds", "ovnkube-node").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	//on OVN IC it takes upto 660 seconds for nodes ds to rollout so lets poll with timeout of 700 seconds
	waitForNetworkOperatorState(oc, 100, 18, "True.*False.*False")
}

// getOVNGatewayMode will return configured OVN gateway mode, shared or local
func getOVNGatewayMode(oc *exutil.CLI) string {
	nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(nodeList.Items) < 1 {
		g.Skip("This case requires at least one schedulable node")
	}
	output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("node", nodeList.Items[0].Name).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	str := "local"
	modeString := strconv.Quote(str)
	if strings.Contains(output, modeString) {
		e2e.Logf("Cluster is running on OVN Local Gateway Mode")
		return str
	}
	return "shared"
}

func getEgressCIDRsForNode(oc *exutil.CLI, nodeName string) string {
	var sub1 string
	platform := exutil.CheckPlatform(oc)
	if strings.Contains(platform, "vsphere") || strings.Contains(platform, "baremetal") || strings.Contains(platform, "nutanix") {
		defaultSubnetV4, err := getDefaultSubnet(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, ipNet, err1 := net.ParseCIDR(defaultSubnetV4)
		o.Expect(err1).NotTo(o.HaveOccurred())
		e2e.Logf("ipnet: %v", ipNet)
		sub1 = ipNet.String()
		e2e.Logf("\n\n\n sub1 as -->%v<--\n\n\n", sub1)
	} else {
		sub1 = getIfaddrFromNode(nodeName, oc)
	}
	return sub1
}

// get routerID by node name
func getRouterID(oc *exutil.CLI, nodeName string) (string, error) {
	// get the ovnkube-node pod on the node
	ovnKubePod, podErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeName)
	o.Expect(podErr).NotTo(o.HaveOccurred())
	o.Expect(ovnKubePod).ShouldNot(o.Equal(""))
	var cmdOutput, routerName, routerID string
	var cmdErr error
	routerName = "GR_" + nodeName
	cmd := "ovn-nbctl show | grep " + routerName + " | grep 'router '|awk '{print $2}'"
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		cmdOutput, cmdErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnKubePod, cmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try again ...,", cmdErr)
			return false, nil
		}

		// Command output always has first line as: Defaulted container "northd" out of: northd, nbdb, kube-rbac-proxy, sbdb, ovnkube-master, ovn-dbchecker
		// Take result from the second line
		cmdOutputLines := strings.Split(cmdOutput, "\n")
		if len(cmdOutputLines) >= 2 {
			routerID = cmdOutputLines[1]
			return true, nil
		}
		e2e.Logf("%v,Waiting for expected result to be synced, try again ...,")
		return false, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The command check result in ovndb is not expected ! See below output \n %s ", cmdOutput)
	}
	return routerID, checkOVNDbErr
}

func getSNATofEgressIP(oc *exutil.CLI, nodeName, egressIP string) (string, error) {
	// get the ovnkube-node pod on the node
	ovnKubePod, podErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", nodeName)
	o.Expect(podErr).NotTo(o.HaveOccurred())
	o.Expect(ovnKubePod).ShouldNot(o.Equal(""))
	var cmdOutput, snatIP string
	var cmdErr error

	routerName := "GR_" + nodeName
	cmd := "ovn-nbctl lr-nat-list " + routerName + " | grep " + egressIP + " |awk '{print $3}'"
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		cmdOutput, cmdErr = exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnKubePod, cmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try again ...,", cmdErr)
			return false, nil
		}

		// Command output always has first line as: Defaulted container "northd" out of: northd, nbdb, kube-rbac-proxy, sbdb, ovnkube-master, ovn-dbchecker
		// Take result from the second line
		if cmdOutput != "" {
			cmdOutputLines := strings.Split(cmdOutput, "\n")
			if len(cmdOutputLines) >= 2 {
				snatIP = cmdOutputLines[1]
				return true, nil
			}
		}
		e2e.Logf("%v,Waiting for expected result to be synced, try again ...")
		return false, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The command check result in ovndb is not expected ! See below output \n %s ", cmdOutput)
	}
	return snatIP, checkOVNDbErr
}

// enableSCTPModuleOnNode Manual way to enable sctp in a cluster
func enableSCTPModuleOnNode(oc *exutil.CLI, nodeName, role string) {
	e2e.Logf("This is %s worker node: %s", role, nodeName)
	checkSCTPCmd := "cat /sys/module/sctp/initstate"
	output, err := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", checkSCTPCmd)
	var installCmd string
	if err != nil || !strings.Contains(output, "live") {
		e2e.Logf("No sctp module installed, will enable sctp module!!!")
		if strings.EqualFold(role, "rhel") {
			// command for rhel nodes
			installCmd = "yum install -y kernel-modules-extra-`uname -r` && insmod /usr/lib/modules/`uname -r`/kernel/net/sctp/sctp.ko.xz"
		} else {
			// command for rhcos nodes
			installCmd = "modprobe sctp"
		}
		e2e.Logf("Install command is %s", installCmd)

		// Try 3 times to enable sctp
		o.Eventually(func() error {
			_, installErr := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", installCmd)
			if installErr != nil && strings.EqualFold(role, "rhel") {
				e2e.Logf("%v", installErr)
				g.Skip("Yum insall to enable sctp cannot work in a disconnected cluster, skip the test!!!")
			}
			return installErr
		}, "15s", "5s").ShouldNot(o.HaveOccurred(), fmt.Sprintf("Failed to install sctp module on node %s", nodeName))

		// Wait for sctp applied
		o.Eventually(func() string {
			output, err := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", checkSCTPCmd)
			if err != nil {
				e2e.Logf("Wait for sctp applied, %v", err)
			}
			return output
		}, "60s", "10s").Should(o.ContainSubstring("live"), fmt.Sprintf("Failed to load sctp module on node %s", nodeName))
	} else {
		e2e.Logf("sctp module is loaded on node %s\n%s", nodeName, output)
	}

}

func prepareSCTPModule(oc *exutil.CLI, sctpModule string) {
	nodesOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Contains(nodesOutput, "SchedulingDisabled") || strings.Contains(nodesOutput, "NotReady") {
		g.Skip("There are already some nodes in NotReady or SchedulingDisabled status in cluster, skip the test!!! ")
	}

	workerNodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	if err != nil || len(workerNodeList.Items) == 0 {
		g.Skip("Can not find any woker nodes in the cluster")
	}

	// Will enable sctp by command
	rhelWorkers, err := exutil.GetAllWorkerNodesByOSID(oc, "rhel")
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(rhelWorkers) > 0 {
		e2e.Logf("There are %v number rhel workers in this cluster, will use manual way to load sctp module.", len(rhelWorkers))
		for _, worker := range rhelWorkers {
			enableSCTPModuleOnNode(oc, worker, "rhel")
		}

	}
	rhcosWorkers, err := exutil.GetAllWorkerNodesByOSID(oc, "rhcos")
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("%v", rhcosWorkers)
	if len(rhcosWorkers) > 0 {
		for _, worker := range rhcosWorkers {
			enableSCTPModuleOnNode(oc, worker, "rhcos")
		}
	}

}

// getIPv4Gateway get ipv4 gateway address
func getIPv4Gateway(oc *exutil.CLI, nodeName string) string {
	cmd := "ip -4 route | grep default | awk '{print $3}'"
	output, err := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	re := regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	ips := re.FindAllString(output, -1)
	if len(ips) == 0 {
		return ""
	}
	e2e.Logf("The default gateway of node %s is %s", nodeName, ips[0])
	return ips[0]
}

// getInterfacePrefix return the prefix of the primary interface IP
func getInterfacePrefix(oc *exutil.CLI, nodeName string) string {
	defInf, err := getDefaultInterface(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	cmd := fmt.Sprintf("ip -4 -brief a show %s | awk '{print $3}' ", defInf)
	output, err := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("IP address for default interface %s is %s", defInf, output)
	sli := strings.Split(output, "/")
	if len(sli) > 0 {
		return strings.Split(sli[1], "\n")[0]
	}
	return "24"
}

func excludeSriovNodes(oc *exutil.CLI) []string {
	// In rdu1 and rdu2 clusters, there are two sriov nodes with mlx nic, by default, egressrouter case cannot run on it
	// So here exclude sriov nodes in rdu1 and rdu2 clusters, just use the other common worker nodes
	var workers []string
	nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, node := range nodeList.Items {
		_, ok := node.Labels["node-role.kubernetes.io/sriov"]
		if !ok {
			e2e.Logf("node %s is not sriov node,add it to worker list.", node.Name)
			workers = append(workers, node.Name)
		}
	}
	return workers
}

func checkClusterStatus(oc *exutil.CLI, expectedStatus string) {
	// get all master nodes
	masterNodes, getAllMasterNodesErr := exutil.GetClusterNodesBy(oc, "master")
	o.Expect(getAllMasterNodesErr).NotTo(o.HaveOccurred())
	o.Expect(masterNodes).NotTo(o.BeEmpty())

	// check master nodes status, expect Ready status for them
	for _, masterNode := range masterNodes {
		checkNodeStatus(oc, masterNode, "Ready")
	}

	// get all worker nodes
	workerNodes, getAllWorkerNodesErr := exutil.GetClusterNodesBy(oc, "worker")
	o.Expect(getAllWorkerNodesErr).NotTo(o.HaveOccurred())
	o.Expect(workerNodes).NotTo(o.BeEmpty())

	// check worker nodes status, expect Ready status for them
	for _, workerNode := range masterNodes {
		checkNodeStatus(oc, workerNode, "Ready")
	}
}

func getOVNKCtrlPlanePodOnHostedCluster(oc *exutil.CLI, namespace, cmName, hyperShiftMgmtNS string) string {
	// get leader ovnkube-control-plane pod on hypershift hosted cluster
	ovnkCtrlPlanePodLead, leaderErr := oc.AsGuestKubeconf().Run("get").Args("lease", "ovn-kubernetes-master", "-n", "openshift-ovn-kubernetes", "-o=jsonpath={.spec.holderIdentity}").Output()
	o.Expect(leaderErr).NotTo(o.HaveOccurred())
	e2e.Logf("ovnkube-control-plane pod of the hosted cluster is %s", ovnkCtrlPlanePodLead)
	return ovnkCtrlPlanePodLead
}

func waitForPodWithLabelReadyOnHostedCluster(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(15*time.Second, 10*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().AsGuestKubeconf().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[*].status.conditions[?(@.type==\"Ready\")].status}").Output()
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

func getPodNameOnHostedCluster(oc *exutil.CLI, namespace, label string) []string {
	var podName []string
	podNameAll, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podName = strings.Split(podNameAll, " ")
	e2e.Logf("The pod(s) are  %v ", podName)
	return podName
}

func getReadySchedulableNodesOnHostedCluster(oc *exutil.CLI) ([]string, error) {
	output, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("node", "-ojsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var nodesOnHostedCluster, schedulableNodes []string
	nodesOnHostedCluster = strings.Split(output, " ")
	for _, nodeName := range nodesOnHostedCluster {
		err := wait.Poll(10*time.Second, 15*time.Minute, func() (bool, error) {
			statusOutput, err := oc.AsAdmin().AsGuestKubeconf().Run("get").Args("nodes", nodeName, "-ojsonpath={.status.conditions[-1].status}").Output()
			if err != nil {
				e2e.Logf("\nGet node status with error : %v", err)
				return false, nil
			}
			if statusOutput != "True" {
				return false, nil
			}
			schedulableNodes = append(schedulableNodes, nodeName)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Node %s is not in expected status %s", nodeName, "Ready"))
	}
	e2e.Logf("Scheduleable nodes on hosted cluster are:  %v ", schedulableNodes)
	return schedulableNodes, nil
}

func checkLogMessageInPodOnHostedCluster(oc *exutil.CLI, namespace string, containerName string, podName string, filter string) (string, error) {
	var podLogs string
	var err, checkErr error
	checkErr = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		podLogs, err = exutil.GetSpecificPodLogs(oc.AsAdmin().AsGuestKubeconf(), namespace, containerName, podName, filter)
		if len(podLogs) == 0 || err != nil {
			e2e.Logf("did not get expected podLog: %v, or have err:%v, try again", podLogs, err)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("fail to get expected log in pod %v, err: %v", podName, checkErr))
	return podLogs, nil

}

// get OVN-Kubernetes management interface (ovn-k8s-mp0) IP for the node on hosted cluster
func getOVNK8sNodeMgmtIPv4OnHostedCluster(oc *exutil.CLI, nodeName string) string {
	var output string
	var outputErr error
	defer exutil.RecoverNamespaceRestricted(oc.AsGuestKubeconf(), "default")
	exutil.SetNamespacePrivileged(oc.AsGuestKubeconf(), "default")
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		output, outputErr = oc.AsGuestKubeconf().WithoutNamespace().Run("debug").Args("-n", "default", "node/"+nodeName, "--", "chroot", "/host", "bash", "-c", "/usr/sbin/ip -4 -brief address show | grep ovn-k8s-mp0").Output()
		if output == "" || outputErr != nil {
			e2e.Logf("Did not get node's management interface on hosted cluster, errors: %v, try again", outputErr)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkErr, fmt.Sprintf("fail to get management interface for node %v, err: %v", nodeName, checkErr))

	e2e.Logf("Match out the OVN-Kubernetes management IP address for the node on hosted cluster")
	re := regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	nodeOVNK8sMgmtIPOnHostedCluster := re.FindAllString(output, -1)[0]
	e2e.Logf("Got ovn-k8s management interface IP for node on hosted cluster %v as: %v", nodeName, nodeOVNK8sMgmtIPOnHostedCluster)
	return nodeOVNK8sMgmtIPOnHostedCluster
}

// execute command on debug node with chroot on node of hosted cluster
func execCmdOnDebugNodeOfHostedCluster(oc *exutil.CLI, nodeName string, cmdOptions []string) error {
	cargs := []string{"node/" + nodeName, "--", "chroot", "/host"}
	if len(cmdOptions) > 0 {
		cargs = append(cargs, cmdOptions...)
	}

	debugErr := oc.AsGuestKubeconf().WithoutNamespace().Run("debug").Args(cargs...).Execute()

	return debugErr
}

// check the cronjobs in the openshift-multus namespace
func getMultusCronJob(oc *exutil.CLI) string {
	cronjobLog, cronjobErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("cronjobs", "-n", "openshift-multus").Output()
	o.Expect(cronjobErr).NotTo(o.HaveOccurred())
	return cronjobLog
}

// get name of OVN egressIP object(s)
func getOVNEgressIPObject(oc *exutil.CLI) []string {
	var egressIPObjects = []string{}
	egressIPObjectsAll, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressip", "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(egressIPObjectsAll) > 0 {
		egressIPObjects = strings.Split(egressIPObjectsAll, " ")
	}
	e2e.Logf("egressIPObjects are  %v ", egressIPObjects)
	return egressIPObjects
}

// Pod's seconary interface can be assigned with ipv4 only, ipv6 only or dualstack address. getPodMultiNetwork can get ipv4 only and dualstack address but not ipv6 only address
// getPodMultiNetworkIPv6 will defined to get ipv6 only address.
func getPodMultiNetworkIPv6(oc *exutil.CLI, namespace string, podName string) string {
	cmd1 := "ip a sho net1 | awk 'NR==3{print $2}' |grep -Eo '([A-Fa-f0-9]{1,4}::?){1,7}[A-Fa-f0-9]{1,4}'"
	podIPv6, err1 := e2eoutput.RunHostCmd(namespace, podName, cmd1)
	o.Expect(err1).NotTo(o.HaveOccurred())
	MultiNetworkIPv6 := strings.TrimSpace(podIPv6)
	return MultiNetworkIPv6
}

// get node that hosts the egressIP
func getHostsubnetByEIP(oc *exutil.CLI, expectedEIP string) string {
	var nodeHostsEIP string
	nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	o.Expect(err).NotTo(o.HaveOccurred())

	for i, v := range nodeList.Items {
		ip, err := getEgressIPByKind(oc, "hostsubnet", nodeList.Items[i].Name, 1)
		o.Expect(err).NotTo(o.HaveOccurred())
		if ip[0] == expectedEIP {
			e2e.Logf("Found node %v host egressip %v ", v.Name)
			nodeHostsEIP = nodeList.Items[i].Name
			break
		}
	}
	return nodeHostsEIP
}

// find the ovn-K cluster manager master pod
func getOVNKMasterPod(oc *exutil.CLI) string {
	leaderCtrlPlanePod, leaderNodeLogerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("lease", "ovn-kubernetes-master", "-n", "openshift-ovn-kubernetes", "-o=jsonpath={.spec.holderIdentity}").Output()
	o.Expect(leaderNodeLogerr).NotTo(o.HaveOccurred())
	return leaderCtrlPlanePod
}

// find the cluster-manager's ovnkube-node for accessing master components
func getOVNKMasterOVNkubeNode(oc *exutil.CLI) string {
	leaderPod, leaderNodeLogerr := oc.AsAdmin().WithoutNamespace().Run("get").Args("lease", "ovn-kubernetes-master", "-n", "openshift-ovn-kubernetes", "-o=jsonpath={.spec.holderIdentity}").Output()
	o.Expect(leaderNodeLogerr).NotTo(o.HaveOccurred())
	leaderNodeName, getNodeErr := exutil.GetPodNodeName(oc, "openshift-ovn-kubernetes", leaderPod)
	o.Expect(getNodeErr).NotTo(o.HaveOccurred())
	ovnKubePod, podErr := exutil.GetPodName(oc, "openshift-ovn-kubernetes", "app=ovnkube-node", leaderNodeName)
	o.Expect(podErr).NotTo(o.HaveOccurred())
	return ovnKubePod
}

// enable multicast on specific namespace
func enableMulticast(oc *exutil.CLI, ns string) {
	_, err := runOcWithRetry(oc.AsAdmin().WithoutNamespace(), "annotate", "namespace", ns, "k8s.ovn.org/multicast-enabled=true")
	o.Expect(err).NotTo(o.HaveOccurred())
}

func getCNOStatusCondition(oc *exutil.CLI) string {
	CNOStatusCondition, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("clusteroperators", "network", "-o=jsonpath={.status.conditions}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return CNOStatusCondition
}

// return severity, expr and runbook of specific ovn alert in networking-rules
func getOVNAlertNetworkingRules(oc *exutil.CLI, alertName string) (string, string, string) {
	// get all ovn alert names in networking-rules
	ns := "openshift-ovn-kubernetes"
	allAlerts, nameErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", ns, "networking-rules", "-o=jsonpath={.spec.groups[*].rules[*].alert}").Output()
	o.Expect(nameErr).NotTo(o.HaveOccurred())
	e2e.Logf("The alert are %v", allAlerts)

	if !strings.Contains(allAlerts, alertName) {
		e2e.Failf("Target alert %v is not found", alertName)
		return "", "", ""
	} else {
		var severity, expr string
		severity, severityErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", ns, "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\""+alertName+"\")].labels.severity}").Output()
		o.Expect(severityErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alert severity is %v", severity)
		expr, exprErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", ns, "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\""+alertName+"\")].expr}").Output()
		o.Expect(exprErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alert expr is %v", expr)
		runbook, runbookErr := oc.AsAdmin().Run("get").Args("prometheusrule", "-n", ns, "networking-rules", "-o=jsonpath={.spec.groups[*].rules[?(@.alert==\""+alertName+"\")].annotations.runbook_url}").Output()
		o.Expect(runbookErr).NotTo(o.HaveOccurred())
		e2e.Logf("The alert runbook is %v", runbook)

		return severity, expr, runbook
	}
}

// returns all the logical routers and switches on all the nodes
func getOVNConstructs(oc *exutil.CLI, constructType string, nodeNames []string) []string {
	var ovnConstructs []string
	var matchStr string
	//var cmdOutput string

	getCmd := "ovn-nbctl --no-leader-only " + constructType
	ovnPod := getOVNKMasterOVNkubeNode(oc)
	o.Expect(ovnPod).ShouldNot(o.Equal(""))
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		cmdOutput, cmdErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", ovnPod, getCmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try again ...,", cmdErr)
			return false, nil
		}
		o.Expect(cmdOutput).ShouldNot(o.Equal(""))
		for _, index := range strings.Split(cmdOutput, "\n") {
			for _, node := range nodeNames {
				if constructType == "ls-list" {
					matchStr = fmt.Sprintf("\\((%s\\))", node)
				} else {
					matchStr = fmt.Sprintf("\\((GR_%s\\))", node)
				}
				re := regexp.MustCompile(matchStr)
				if re.FindString(index) != "" {
					ovnConstruct := strings.Fields(index)
					ovnConstructs = append(ovnConstructs, ovnConstruct[0])
				}
			}
		}
		return true, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The result in ovndb is not expected ! See below output \n %s ", checkOVNDbErr)
	}
	return ovnConstructs
}

// Returns the logical router or logical switch on a node
func (svcEndpontDetails *svcEndpontDetails) getOVNConstruct(oc *exutil.CLI, constructType string) string {
	var ovnConstruct string
	var matchStr string
	getCmd := "ovn-nbctl " + constructType
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		cmdOutput, cmdErr := exutil.RemoteShPodWithBash(oc, "openshift-ovn-kubernetes", svcEndpontDetails.ovnKubeNodePod, getCmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try again ...,", cmdErr)
			return false, nil
		}
		if cmdOutput == "" {
			return true, nil
		}
		for _, index := range strings.Split(cmdOutput, "\n") {

			if constructType == "ls-list" {
				matchStr = fmt.Sprintf("\\((%s\\))", svcEndpontDetails.nodeName)
			} else {
				matchStr = fmt.Sprintf("\\((GR_%s\\))", svcEndpontDetails.nodeName)
			}
			re := regexp.MustCompile(matchStr)
			if re.FindString(index) != "" {
				matchedStr := strings.Fields(index)
				ovnConstruct = matchedStr[0]
			}
		}
		return true, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The result in ovndb is not expected ! See below output \n %s ", checkOVNDbErr)
	}
	return ovnConstruct
}

// returns load balancer entries created for LB service type on routers or switches on all nodes
func getOVNLBContructs(oc *exutil.CLI, constructType string, endPoint string, ovnConstruct []string) bool {
	var result bool
	ovnPod := getOVNKMasterOVNkubeNode(oc)
	o.Expect(ovnPod).ShouldNot(o.Equal(""))
	//only if the count for any of output is less than three the success will be false
	result = true
	for _, construct := range ovnConstruct {
		checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			getCmd := "ovn-nbctl --no-leader-only " + constructType + " " + construct + " | grep " + endPoint
			cmdOutput, cmdErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", ovnPod, "northd", getCmd)
			if cmdErr != nil {
				e2e.Logf("%v,Waiting for expected result to be synced, try next ...,", cmdErr)
				result = false
				return false, nil
			}
			if len(strings.Split(cmdOutput, "\n")) >= 2 {
				e2e.Logf("Required entries %s were created for service on %s", constructType, construct)
				result = true
			} else {
				e2e.Logf("Required entries %s were not created for service on %s", constructType, construct)
				result = false
			}
			return true, nil
		})
		if checkOVNDbErr != nil {
			e2e.Logf("The command check result in ovndb is not expected ! See below output \n %s ", checkOVNDbErr)
			result = false
		}

	}
	return result
}

// returns load balancer entries created for LB service type on routers or switches on a single node
func (svcEndpontDetails *svcEndpontDetails) getOVNLBContruct(oc *exutil.CLI, constructType string, construct string) bool {
	var result bool
	//only if the count for any of output is less than three the success will be false
	result = true
	checkOVNDbErr := wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
		getCmd := "ovn-nbctl " + constructType + " " + construct + " | grep " + svcEndpontDetails.podIP
		cmdOutput, cmdErr := exutil.RemoteShPodWithBashSpecifyContainer(oc, "openshift-ovn-kubernetes", svcEndpontDetails.ovnKubeNodePod, "northd", getCmd)
		if cmdErr != nil {
			e2e.Logf("%v,Waiting for expected result to be synced, try next ...,", cmdErr)
			result = false
			return false, nil
		}
		if len(strings.Split(cmdOutput, "\n")) >= 2 {
			e2e.Logf("Required entries %s were created for service on %s", constructType, construct)
			result = true
		} else {
			e2e.Logf("Required entries %s were not created for service on %s", constructType, construct)
			result = false
		}
		return true, nil
	})
	if checkOVNDbErr != nil {
		e2e.Logf("The command check result in ovndb is not expected ! See below output \n %s ", checkOVNDbErr)
		result = false
	}

	return result
}

func getServiceEndpoints(oc *exutil.CLI, serviceName string, serviceNamespace string) string {
	serviceEndpoint, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ep", serviceName, "-n", serviceNamespace, "--no-headers").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(serviceEndpoint).ShouldNot(o.BeEmpty())
	e2e.Logf("Service endpoint %v", serviceEndpoint)
	result := strings.Fields(serviceEndpoint)
	return result[1]
}

func getOVNMetricsInSpecificContainer(oc *exutil.CLI, containerName string, podName string, url string, metricName string) string {
	var metricValue string
	metricsErr := wait.Poll(5*time.Second, 10*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-ovn-kubernetes", "-c", containerName, podName, "--", "curl", url).OutputToFile("metrics.txt")
		if err != nil {
			e2e.Logf("Can't get metrics and try again, the error is:%s", err)
			return false, nil
		}
		if strings.Contains(metricName, "ovnkube_controller_pod") {
			metricOutput, getMetricErr := exec.Command("bash", "-c", "cat "+output+" | grep "+metricName+" | awk 'NR==1{print $2}'").Output()
			o.Expect(getMetricErr).NotTo(o.HaveOccurred())
			metricValue = strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue)
			return true, nil
		} else {
			metricOutput, getMetricErr := exec.Command("bash", "-c", "cat "+output+" | grep "+metricName+" | awk 'NR==3{print $2}'").Output()
			o.Expect(getMetricErr).NotTo(o.HaveOccurred())
			metricValue = strings.TrimSpace(string(metricOutput))
			e2e.Logf("The output of the %s is : %v", metricName, metricValue)
			return true, nil
		}

	})
	exutil.AssertWaitPollNoErr(metricsErr, fmt.Sprintf("Fail to get metric and the error is:%s", metricsErr))
	return metricValue
}

// CurlNodePortPass checks nodeport svc reacability from a node regardless of network addressing type on cluster
func CurlNodePortPass(oc *exutil.CLI, nodeNameFrom string, nodeNameTo string, nodePort string) {
	nodeIP1, nodeIP2 := getNodeIP(oc, nodeNameTo)
	if nodeIP1 != "" {
		nodev6URL := net.JoinHostPort(nodeIP1, nodePort)
		nodev4URL := net.JoinHostPort(nodeIP2, nodePort)
		output, _ := exutil.DebugNode(oc, nodeNameFrom, "curl", nodev4URL, "-s", "--connect-timeout", "5")
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
		output, _ = exutil.DebugNode(oc, nodeNameFrom, "curl", nodev6URL, "-s", "--connect-timeout", "5")
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
	} else {
		nodeURL := net.JoinHostPort(nodeIP2, nodePort)
		output, _ := exutil.DebugNode(oc, nodeNameFrom, "curl", nodeURL, "-s", "--connect-timeout", "5")
		o.Expect(output).Should(o.ContainSubstring("Hello OpenShift"))
	}
}

// CurlNodePortFail checks nodeport svc unreacability from a node regardless of network addressing type on cluster
func CurlNodePortFail(oc *exutil.CLI, nodeNameFrom string, nodeNameTo string, nodePort string) {
	nodeIP1, nodeIP2 := getNodeIP(oc, nodeNameTo)
	if nodeIP1 != "" {
		nodev6URL := net.JoinHostPort(nodeIP1, nodePort)
		nodev4URL := net.JoinHostPort(nodeIP2, nodePort)
		output, _ := exutil.DebugNode(oc, nodeNameFrom, "curl", nodev4URL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("timed out")))
		output, _ = exutil.DebugNode(oc, nodeNameFrom, "curl", nodev6URL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("timed out")))
	} else {
		nodeURL := net.JoinHostPort(nodeIP2, nodePort)
		output, _ := exutil.DebugNode(oc, nodeNameFrom, "curl", nodeURL, "--connect-timeout", "5")
		o.Expect(output).To(o.Or(o.ContainSubstring("28"), o.ContainSubstring("timed out")))
	}
}

// get primary NIC interface name
func getPrimaryNICname(oc *exutil.CLI) string {
	masterNode, getMasterNodeErr := exutil.GetFirstMasterNode(oc)
	o.Expect(getMasterNodeErr).NotTo(o.HaveOccurred())
	primary_int, err := exutil.DebugNodeWithChroot(oc, masterNode, "bash", "-c", "nmcli -g connection.interface-name c show ovs-if-phys0")
	o.Expect(err).NotTo(o.HaveOccurred())
	primary_inf_name := strings.Split(primary_int, "\n")
	e2e.Logf("Primary Inteface name is : %s", primary_inf_name[0])
	return primary_inf_name[0]
}

// get file contents to be modified for SCTP
func getFileContentforSCTP(baseDir string, name string) (fileContent string) {
	filePath := filepath.Join(exutil.FixturePath("testdata", "networking", baseDir), name)
	fileOpen, err := os.Open(filePath)
	defer fileOpen.Close()
	if err != nil {
		e2e.Failf("Failed to open file: %s", filePath)
	}
	fileRead, _ := io.ReadAll(fileOpen)
	if err != nil {
		e2e.Failf("Failed to read file: %s", filePath)
	}
	return string(fileRead)
}

// get generic sctpclient pod yaml file, replace variables as per requirements
func createSCTPclientOnNode(oc *exutil.CLI, pod_pmtrs map[string]string) (err error) {
	PodGenericYaml := getFileContentforSCTP("sctp", "sctpclientspecificnode.yaml")
	for rep, value := range pod_pmtrs {
		PodGenericYaml = strings.ReplaceAll(PodGenericYaml, rep, value)
	}
	podFileName := "temp-sctp-client-pod-" + getRandomString() + ".yaml"
	defer os.Remove(podFileName)
	os.WriteFile(podFileName, []byte(PodGenericYaml), 0644)
	// create ping pod for Microshift
	_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", podFileName).Output()
	return err
}

// get generic sctpserver pod yaml file, replace variables as per requirements
func createSCTPserverOnNode(oc *exutil.CLI, pod_pmtrs map[string]string) (err error) {
	PodGenericYaml := getFileContentforSCTP("sctp", "sctpserverspecificnode.yaml")
	for rep, value := range pod_pmtrs {
		PodGenericYaml = strings.ReplaceAll(PodGenericYaml, rep, value)
	}
	podFileName := "temp-sctp-server-pod-" + getRandomString() + ".yaml"
	defer os.Remove(podFileName)
	os.WriteFile(podFileName, []byte(PodGenericYaml), 0644)
	// create ping pod for Microshift
	_, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", podFileName).Output()
	return err
}

// configure IPSec at runtime
func configIPSecAtRuntime(oc *exutil.CLI, targetStatus string) (err error) {
	var targetConfig, currentStatus string
	ipsecState := checkIPsec(oc)
	if ipsecState == "{}" {
		currentStatus = "enabled"
	} else if ipsecState == "" {
		currentStatus = "disabled"
	}
	if currentStatus == targetStatus {
		e2e.Logf("The IPSec is already in %v state", targetStatus)
		return
	} else if targetStatus == "enabled" {
		targetConfig = "true"
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("networks.operator.openshift.io", "cluster", "-p", "{\"spec\":{\"defaultNetwork\":{\"ovnKubernetesConfig\":{\"ipsecConfig\":{ }}}}}", "--type=merge").Output()
	} else if targetStatus == "disabled" {
		targetConfig = "false"
		_, err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("networks.operator.openshift.io", "cluster", "-p", "{\"spec\":{\"defaultNetwork\":{\"ovnKubernetesConfig\":{\"ipsecConfig\":null}}}}", "--type=merge").Output()
	}

	if err != nil {
		e2e.Failf("Failed to configure IPSec at runtime")
	} else {
		// need to restart "north" leader after configuring ipsec to make sure use correct "north" leader
		ovnLeaderpod := getOVNKMasterOVNkubeNode(oc)
		removeResource(oc, true, true, "pod", ovnLeaderpod, "-n", "openshift-ovn-kubernetes")
		waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
		checkErr := checkIPSecInDB(oc, targetConfig)
		exutil.AssertWaitPollNoErr(checkErr, "check IPSec configuration failed")
		e2e.Logf("The IPSec is %v in the cluster.", targetStatus)
	}
	return err
}

// check IPSec configuration in northd, targetConfig should be "true" or "false"
func checkIPSecInDB(oc *exutil.CLI, targetConfig string) error {
	ovnLeaderpod := getOVNKMasterOVNkubeNode(oc)
	return wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		getIPSec, getErr := execCommandInSpecificPod(oc, "openshift-ovn-kubernetes", ovnLeaderpod, "ovn-nbctl --no-leader-only get nb_global . ipsec | grep "+targetConfig)
		o.Expect(getErr).NotTo(o.HaveOccurred())
		if strings.Contains(getIPSec, targetConfig) {
			return true, nil
		}
		e2e.Logf("Can't get expected ipsec configuration and try again")
		return false, nil
	})
}

// IsIPv4 check if the string is an IPv4 address.
func IsIPv4(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ".")
}

// IsIPv6 check if the string is an IPv6 address.
func IsIPv6(str string) bool {
	ip := net.ParseIP(str)
	return ip != nil && strings.Contains(str, ":")
}

// checkSCTPResultPASS
func checkSCTPResultPASS(oc *exutil.CLI, namespace, sctpServerPodName, sctpClientPodname, dstIP, dstPort string) {
	exutil.By("sctpserver pod start to wait for sctp traffic")
	_, _, _, err1 := oc.Run("exec").Args("-n", oc.Namespace(), sctpServerPodName, "--", "/usr/bin/ncat", "-l", "30102", "--sctp").Background()
	o.Expect(err1).NotTo(o.HaveOccurred())
	time.Sleep(5 * time.Second)

	exutil.By("check sctp process enabled in the sctp server pod")
	msg, err2 := e2eoutput.RunHostCmd(oc.Namespace(), sctpServerPodName, "ps aux | grep sctp")
	o.Expect(err2).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(msg, "/usr/bin/ncat -l 30102 --sctp")).To(o.BeTrue())

	exutil.By("sctpclient pod start to send sctp traffic")
	_, err3 := e2eoutput.RunHostCmd(oc.Namespace(), sctpClientPodname, "echo 'Test traffic using sctp port from sctpclient to sctpserver' | { ncat -v "+dstIP+" "+dstPort+" --sctp; }")
	o.Expect(err3).NotTo(o.HaveOccurred())

	exutil.By("server sctp process will end after get sctp traffic from sctp client")
	time.Sleep(5 * time.Second)
	msg1, err4 := e2eoutput.RunHostCmd(oc.Namespace(), sctpServerPodName, "ps aux | grep sctp")
	o.Expect(err4).NotTo(o.HaveOccurred())
	o.Expect(msg1).NotTo(o.ContainSubstring("/usr/bin/ncat -l 30102 --sctp"))
}

func ovnkubeNodePod(oc *exutil.CLI, nodeName string) string {
	// get OVNkubeNode pod on specific node.
	ovnNodePod, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-ovn-kubernetes", "pod", "-l app=ovnkube-node", "--field-selector", "spec.nodeName="+nodeName, "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The ovnkube-node pod on node %s is %s", nodeName, ovnNodePod)
	o.Expect(ovnNodePod).NotTo(o.BeEmpty())
	return ovnNodePod
}

func waitForNetworkOperatorState(oc *exutil.CLI, interval int, timeout int, expectedStatus string) {
	errCheck := wait.Poll(time.Duration(interval)*time.Second, time.Duration(timeout)*time.Minute, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "network").Output()
		if err != nil {
			e2e.Logf("Fail to get clusteroperator network, error:%s. Trying again", err)
			return false, nil
		}
		if matched, _ := regexp.MatchString(expectedStatus, output); !matched {
			e2e.Logf("Network operator state is:%s", output)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(errCheck, fmt.Sprintf("Timed out waiting for the expected condition"))
}

func enableIPForwardingOnSpecNodeNIC(oc *exutil.CLI, worker, secNIC string) {
	cmd := fmt.Sprintf("sysctl net.ipv4.conf.%s.forwarding", secNIC)
	output, debugNodeErr := exutil.DebugNode(oc, worker, "bash", "-c", cmd)
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	if !strings.Contains(output, ".forwarding = 1") {
		e2e.Logf("Enable IP forwarding for NIC %s on node %s ...", secNIC, worker)
		enableCMD := fmt.Sprintf("sysctl -w net.ipv4.conf.%s.forwarding=1", secNIC)
		_, debugNodeErr = exutil.DebugNode(oc, worker, "bash", "-c", enableCMD)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	}
	e2e.Logf("IP forwarding was enabled for NIC %s on node %s!", secNIC, worker)
}

func disableIPForwardingOnSpecNodeNIC(oc *exutil.CLI, worker, secNIC string) {
	cmd := fmt.Sprintf("sysctl net.ipv4.conf.%s.forwarding", secNIC)
	output, debugNodeErr := exutil.DebugNode(oc, worker, "bash", "-c", cmd)
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	if strings.Contains(output, ".forwarding = 1") {
		e2e.Logf("Disable IP forwarding for NIC %s on node %s ...", secNIC, worker)
		disableCMD := fmt.Sprintf("sysctl -w net.ipv4.conf.%s.forwarding=0", secNIC)
		_, debugNodeErr = exutil.DebugNode(oc, worker, "bash", "-c", disableCMD)
		o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	}
	e2e.Logf("IP forwarding was disabled for NIC %s on node %s!", secNIC, worker)
}

func nbContructToMap(nbConstruct string) map[string]string {
	listKeyValues := strings.Split(nbConstruct, "\n")
	var tempMap map[string]string
	tempMap = make(map[string]string)
	for _, keyValPair := range listKeyValues {
		keyValItem := strings.Split(keyValPair, ":")
		key := strings.Trim(keyValItem[0], " ")
		val := strings.TrimLeft(keyValItem[1], " ")
		tempMap[key] = val
	}
	return tempMap
}

// Create live migration job on Kubevirt cluster
func (migrationjob *migrationDetails) createMigrationJob(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", migrationjob.template, "-p", "NAME="+migrationjob.name, "NAMESPACE="+migrationjob.namespace, "VMI="+migrationjob.virtualmachinesintance)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create migration job %v", migrationjob.name))
}

// Delete migration job on Kubevirt cluster
func (migrationjob *migrationDetails) deleteMigrationJob(oc *exutil.CLI) {
	removeResource(oc, true, true, "virtualmachineinstancemigration.kubevirt.io", migrationjob.name, "-n", migrationjob.namespace)
}

// Check all cluster operators status on the cluster
func checkAllClusterOperatorsState(oc *exutil.CLI, interval int, timeout int) {
	operatorsString, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	var clusterOperators []string
	if operatorsString != "" {
		clusterOperators = strings.Split(operatorsString, " ")
	}

	for _, clusterOperator := range clusterOperators {
		errCheck := wait.Poll(time.Duration(interval)*time.Second, time.Duration(timeout)*time.Minute, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", clusterOperator).Output()
			if err != nil {
				e2e.Logf("Fail to get state for operator %s, error:%s. Trying again", clusterOperator, err)
				return false, err
			}
			if matched, _ := regexp.MatchString("True.*False.*False", output); !matched {
				e2e.Logf("Operator %s on hosted cluster is in state:%s", output)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errCheck, "Timed out waiting for the expected condition")
	}
}

// Check OVNK health: OVNK pods health and ovnkube-node DS health
func checkOVNKState(oc *exutil.CLI) {
	// check all OVNK pods
	waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-node")
	waitForPodWithLabelReady(oc, "openshift-ovn-kubernetes", "app=ovnkube-control-plane")

	// check ovnkube-node ds rollout status
	dsStatus, dsStatusErr := oc.AsAdmin().WithoutNamespace().Run("rollout").Args("status", "-n", "openshift-ovn-kubernetes", "ds", "ovnkube-node", "--timeout", "5m").Output()
	o.Expect(dsStatusErr).NotTo(o.HaveOccurred())
	o.Expect(strings.Contains(dsStatus, "successfully rolled out")).To(o.BeTrue())

}

func addDummyInferface(oc *exutil.CLI, nodeName, IP, nicName string) {
	e2e.Logf("Add a dummy interface %s on node %s \n", nicName, nodeName)
	cmd := fmt.Sprintf("ip link a %s type dummy && ip link set dev %s up && ip a add %s dev %s && ip a show %s", nicName, nicName, IP, nicName, nicName)
	output, debugNodeErr := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	e2e.Logf("The dummy interface was added. \n %s", output)

}

func addIPtoInferface(oc *exutil.CLI, nodeName, IP, nicName string) {
	e2e.Logf("Add IP address %s to interface %s on node %s \n", IP, nicName, nodeName)
	cmd := fmt.Sprintf("ip a show %s && ip a add %s dev %s", nicName, IP, nicName)
	_, debugNodeErr := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
}

func delIPFromInferface(oc *exutil.CLI, nodeName, IP, nicName string) {
	e2e.Logf("Remove IP address %s from interface %s on node %s \n", IP, nicName, nodeName)
	cmd := fmt.Sprintf("ip a show %s && ip a del %s dev %s", nicName, IP, nicName)
	_, debugNodeErr := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
}

func removeDummyInterface(oc *exutil.CLI, nodeName, nicName string) {
	e2e.Logf("Remove a dummy interface %s on node %s \n", nicName, nodeName)
	cmd := fmt.Sprintf("ip a show %s && ip link del %s ", nicName, nicName)
	output, debugNodeErr := exutil.DebugNode(oc, nodeName, "bash", "-c", cmd)
	nicNotExistStr := fmt.Sprintf("Device \"%s\" does not exist", nicName)
	if debugNodeErr != nil && strings.Contains(output, nicNotExistStr) {
		e2e.Logf("The dummy interface %s does not exist on node %s ! \n", nicName, nodeName)
		return
	}
	o.Expect(debugNodeErr).NotTo(o.HaveOccurred())
	e2e.Logf("The dummy interface %s was removed from node %s ! \n", nicName, nodeName)
}

func (kkPod *kubeletKillerPod) createKubeletKillerPodOnNode(oc *exutil.CLI) {
	err := wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
		err1 := applyResourceFromTemplateByAdmin(oc, "--ignore-unknown-parameters=true", "-f", kkPod.template, "-p", "NAME="+kkPod.name, "NAMESPACE="+kkPod.namespace, "NODENAME="+kkPod.nodename)
		if err1 != nil {
			e2e.Logf("the err:%v, and try next round", err1)
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to create Kubelet-Killer pod %v", kkPod.name))
}

func getNodeNameByIPv4(oc *exutil.CLI, nodeIPv4 string) (nodeName string) {
	nodeList, err := e2enode.GetReadySchedulableNodes(context.TODO(), oc.KubeFramework().ClientSet)
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, node := range nodeList.Items {
		_, IPv4 := getNodeIP(oc, node.Name)
		if IPv4 == nodeIPv4 {
			nodeName = node.Name
			break
		}
	}
	return nodeName
}
