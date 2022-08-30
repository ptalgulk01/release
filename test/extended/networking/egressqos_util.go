package networking

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type egressQosResource struct {
	name      string
	namespace string
	tempfile  string
	kind      string
}

//delete egressqos resource
func (rs *egressQosResource) delete(oc *exutil.CLI) {
	e2e.Logf("delete %s %s in namespace %s", rs.kind, rs.name, rs.namespace)
	oc.AsAdmin().WithoutNamespace().Run("delete").Args(rs.kind, rs.name, "-n", rs.namespace, "--ignore-not-found=true").Execute()
}

//create egressqos resource
func (rs *egressQosResource) create(oc *exutil.CLI, parameters ...string) {

	paras := []string{"-f", rs.tempfile, "--ignore-unknown-parameters=true", "-p"}
	for _, para := range parameters {
		paras = append(paras, para)
	}
	exutil.ApplyNsResourceFromTemplate(oc, rs.namespace, paras...)
}

//create egressqos resource with output
func (rs *egressQosResource) createWithOutput(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	cmd := []string{"-f", rs.tempfile, "--ignore-unknown-parameters=true", "-p"}
	for _, para := range parameters {
		cmd = append(cmd, para)
	}
	e2e.Logf("parameters list is %s\n", cmd)

	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(cmd...).OutputToFile(getRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v resource: %v", rs.kind, cmd))
	e2e.Logf("the file of resource is %s\n", configFile)

	output, err1 := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", rs.namespace).Output()
	return output, err1
}

func runSSHCmdOnAWS(host string, cmd string) (string, error) {
	user := os.Getenv("SSH_CLOUD_PRIV_AWS_USER")
	if user == "" {
		user = "core"
	}
	sshkey := os.Getenv("SSH_CLOUD_PRIV_KEY")
	if sshkey == "" {
		sshkey = "../internal/config/keys/openshift-qe.pem"
	}

	sshClient := exutil.SshClient{User: user, Host: host, Port: 22, PrivateKey: sshkey}
	return sshClient.RunOutput(cmd)
}

func installDscpServiceOnAWS(a *exutil.AwsClient, oc *exutil.CLI, publicIP string) error {

	command := "sudo netstat -ntlp | grep 9096 || sudo podman run --name dscpecho -d -p 9096:8080 quay.io/openshifttest/hello-sdn@sha256:c89445416459e7adea9a5a416b3365ed3d74f2491beb904d61dc8d1eb89a72a4"
	e2e.Logf("Run command %s", command)

	outPut, err := runSSHCmdOnAWS(publicIP, command)
	if err != nil {
		e2e.Logf("Failed to run %v: %v", command, outPut)
		return err
	}

	updateAwsIntSvcSecurityRule(a, oc, 9096)

	return nil
}

func startTcpdumpOnDscpService(a *exutil.AwsClient, oc *exutil.CLI, publicIP string, pktfile string) {
	//start tcpdump
	tcpdumpCmd := "'tcpdump tcp -c 5 -vvv -i eth0 -n and dst port 8080 > '" + fmt.Sprintf("%s", pktfile)
	command := "sudo podman exec -d dscpecho bash -c  " + tcpdumpCmd
	e2e.Logf("Run command %s", command)

	outPut, err := runSSHCmdOnAWS(publicIP, command)
	if err != nil {
		e2e.Logf("Failed to run %v: %v", command, outPut)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func chkDSCPinPkts(a *exutil.AwsClient, oc *exutil.CLI, publicIP string, pktfile string, dscp int) bool {
	command := "sudo podman exec -- dscpecho cat " + fmt.Sprintf("%s", pktfile)
	outPut, err := runSSHCmdOnAWS(publicIP, command)

	if err != nil {
		e2e.Logf("Failed to run %v: %v", command, outPut)
		return false
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Captured packets are %s", outPut)
	tosHex := dscpDecConvertToHex(dscp)
	dscpString := "tos 0x" + tosHex

	if !strings.Contains(outPut, dscpString) {
		e2e.Logf("Captured packets doesn't contain dscp value %s", dscpString)
		return false
	}
	e2e.Logf("Captured packets contains dscp value %s", dscpString)
	return true
}

func rmPktsFile(a *exutil.AwsClient, oc *exutil.CLI, publicIP string, pktfile string) {
	command := "sudo podman exec -- dscpecho rm " + fmt.Sprintf("%s", pktfile)
	outPut, err := runSSHCmdOnAWS(publicIP, command)
	if err != nil {
		e2e.Logf("Failed to run %v: %v", command, outPut)
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func dscpDecConvertToHex(dscp int) string {
	tosInt := dscp * 4
	tosHex := fmt.Sprintf("%x", tosInt)
	e2e.Logf("The dscp hex value is %v", tosHex)
	return tosHex
}

func startCurlTraffic(oc *exutil.CLI, ns string, pod string, dstip string, dstport string) {
	e2e.Logf("start curl traffic")
	dstURL := net.JoinHostPort(dstip, dstport)
	cmd := "curl -k " + dstURL
	outPut, err := exutil.RemoteShPodWithBash(oc, ns, pod, cmd)

	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(outPut).Should(o.ContainSubstring("Hello OpenShift"))

}
