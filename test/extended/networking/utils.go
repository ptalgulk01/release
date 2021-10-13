package networking

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type pingPodResource struct {
	name      string
	namespace string
	template  string
}

type egressIPResource1 struct {
	name      string
	template  string
	egressIP1 string
	egressIP2 string
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

func (egressFirewall *egressFirewall2) deleteEgressFW2Object(oc *exutil.CLI) {
	removeResource(oc, true, true, "egressfirewall", egressFirewall.name, "-n", egressFirewall.namespace)
}

func (pingPod *pingPodResource) deletePingPod(oc *exutil.CLI) {
	removeResource(oc, false, true, "pod", pingPod.name, "-n", pingPod.namespace)
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
	podStatus, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s status in namespace %s is %q", podName, namespace, podStatus)
	return podStatus, err
}

func checkPodReady(oc *exutil.CLI, namespace string, podName string) (bool, error) {
	podOutPut, err := getPodStatus(oc, namespace, podName)
	status := []string{"Running", "Ready", "Complete"}
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
	command1 := []string{"-n", namespace, podName, "--", "/bin/sh", "-c", command}
	msg, err := oc.WithoutNamespace().Run("exec").Args(command1...).Output()
	if err != nil {
		e2e.Logf("Execute command failed with  err:%v .", err)
		return msg, err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func execCommandInOVNPod(oc *exutil.CLI, command string) (string, error) {
	ovnPodName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-ovn-kubernetes", "-l", "app=ovnkube-node", "-o=jsonpath={.items[0].metadata.name}").Output()
	if err != nil {
		e2e.Logf("Cannot get onv-kubernetes pods, errors: %v", err)
		return "", err
	}
	ovnCmd := []string{"-n", "openshift-ovn-kubernetes", "-c", "ovnkube-node", ovnPodName, "--", "/bin/sh", "-c", command}

	msg, err := oc.WithoutNamespace().AsAdmin().Run("exec").Args(ovnCmd...).Output()
	if err != nil {
		e2e.Logf("Execute ovn command failed with  err:%v .", err)
		return "", err
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return msg, nil
}

func getDefaultInterface(oc *exutil.CLI) (string, error) {
	getDefaultInterfaceCmd := "/usr/sbin/ip -4 route show default"
	int1, err := execCommandInOVNPod(oc, getDefaultInterfaceCmd)
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
	subnet1, err := execCommandInOVNPod(oc, getDefaultSubnetCmd)
	if err != nil {
		e2e.Logf("Cannot get default subnet, errors: %v", err)
		return "", err
	}
	defSubnet := strings.Fields(subnet1)[2]
	e2e.Logf("Get the default subnet: %s", defSubnet)
	return defSubnet, nil
}

func Hosts(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
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
		if len(ipUnused) <= number {
			pingCmd := "ping -c4 -t1 " + ip
			_, err := execCommandInOVNPod(oc, pingCmd)
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
	subnet1, err := execCommandInOVNPod(oc, getDefaultSubnetCmd)
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
		pingCmd := "ping -c4 -t1 -6 " + ip.String()
		_, err := execCommandInOVNPod(oc, pingCmd)
		if err != nil && i < number {
			e2e.Logf("%s is not used!\n", ip)
			ips = append(ips, ip.String())
		} else if i >= number {
			break
		}
		i++
	}

	return ips, nil
}

func ipv6EchoServer(isIPv6 bool) string {
	if isIPv6 {
		return "[2620:52:0:4974:def4:1ff:fee7:8144]:8085"
	} else {
		return "10.73.116.56:8085"
	}
}

func checkIpStackType(oc *exutil.CLI) string {
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

func checkSctpModule(oc *exutil.CLI, nodeName string) {
	err := wait.Poll(30*time.Second, 15*time.Minute, func() (bool, error) {
		// Check nodes status to make sure all nodes are up after rebooting caused by load-sctp-module
		nodes_status, err := oc.AsAdmin().Run("get").Args("node").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("oc_get_nodes: %v", nodes_status)
		status, _ := oc.AsAdmin().Run("debug").Args("node/"+nodeName, "--", "cat", "/sys/module/sctp/initstate").Output()
		if strings.Contains(status, "live") {
			e2e.Logf("stcp module is installed in the %s", nodeName)
			return true, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, "stcp module is installed in the nodes")
}

func getPodIPv4(oc *exutil.CLI, namespace string, podName string) (string) {
	podIPv4, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv4)
	return podIPv4
}

func getPodIPv6(oc *exutil.CLI, namespace string, podName string, ipStack string) (string) {
	if ipStack == "ipv6single" {
		podIPv6, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[0].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6
	} else if ipStack == "dualstack" {
		podIPv6, err := oc.WithoutNamespace().Run("get").Args("pod", "-n", namespace, podName, "-o=jsonpath={.status.podIPs[1].ip}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("The pod  %s IP in namespace %s is %q", podName, namespace, podIPv6)
		return podIPv6	
	}
	return ""
}

// For normal user to create resources in the specified namespace from the file (not template)
func createResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
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

func getSvcIPv4(oc *exutil.CLI, namespace string, svcName string) (string) {
	svcIPv4, err := oc.WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
	return svcIPv4
}

func getSvcIPv6(oc *exutil.CLI, namespace string, svcName string) (string) {
	svcIPv6, err := oc.WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv6
}

func getSvcIPdualstack(oc *exutil.CLI, namespace string, svcName string) (string, string) {
	svcIPv4, err := oc.WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[0]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv4 in namespace %s is %q", svcName, namespace, svcIPv4)
    svcIPv6, err := oc.WithoutNamespace().Run("get").Args("service", "-n", namespace, svcName, "-o=jsonpath={.spec.clusterIPs[1]}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The service %s IPv6 in namespace %s is %q", svcName, namespace, svcIPv6)
	return svcIPv4, svcIPv6
}


