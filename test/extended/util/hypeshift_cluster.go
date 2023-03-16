package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ValidHypershiftAndGetGuestKubeConf check if it is hypershift env and get kubeconf of the hosted cluster
// the first return is hosted cluster name
// the second return is the file of kubeconfig of the hosted cluster
// the third return is the hostedcluster namespace in mgmt cluster which contains the generated resources
// if it is not hypershift env, it will skip test.
func ValidHypershiftAndGetGuestKubeConf(oc *CLI) (string, string, string) {
	if IsROSA() {
		e2e.Logf("there is a ROSA env")
		hostedClusterName, hostedclusterKubeconfig, hostedClusterNs := ROSAValidHypershiftAndGetGuestKubeConf(oc)
		if len(hostedClusterName) == 0 || len(hostedclusterKubeconfig) == 0 || len(hostedClusterNs) == 0 {
			g.Skip("there is a ROSA env, but the env is problematic, skip test run")
		}
		return hostedClusterName, hostedclusterKubeconfig, hostedClusterNs
	}
	operatorNS := GetHyperShiftOperatorNameSpace(oc)
	if len(operatorNS) <= 0 {
		g.Skip("there is no hypershift operator on host cluster, skip test run")
	}

	hostedclusterNS := GetHyperShiftHostedClusterNameSpace(oc)
	if len(hostedclusterNS) <= 0 {
		g.Skip("there is no hosted cluster NS in mgmt cluster, skip test run")
	}

	clusterNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", hostedclusterNS, "hostedclusters", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(clusterNames) <= 0 {
		g.Skip("there is no hosted cluster, skip test run")
	}

	hypersfhitPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", operatorNS, "pod", "-l", "hypershift.openshift.io/operator-component=operator", "-l", "app=operator", "-o=jsonpath={.items[*].status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(hypersfhitPodStatus).To(o.ContainSubstring("Running"))

	//get first hosted cluster to run test
	e2e.Logf("the hosted cluster names: %s, and will select the first", clusterNames)
	clusterName := strings.Split(clusterNames, " ")[0]

	var hostedClusterKubeconfigFile string
	if os.Getenv("GUEST_KUBECONFIG") != "" {
		e2e.Logf("the kubeconfig you set GUEST_KUBECONFIG must be that of the hosted cluster %s in namespace %s", clusterName, hostedclusterNS)
		hostedClusterKubeconfigFile = os.Getenv("GUEST_KUBECONFIG")
		e2e.Logf(fmt.Sprintf("use a known hosted cluster kubeconfig: %v", hostedClusterKubeconfigFile))
	} else {
		hostedClusterKubeconfigFile = "/tmp/guestcluster-kubeconfig-" + clusterName + "-" + getRandomString()
		output, err := exec.Command("bash", "-c", fmt.Sprintf("hypershift create kubeconfig --name %s --namespace %s > %s",
			clusterName, hostedclusterNS, hostedClusterKubeconfigFile)).Output()
		e2e.Logf("the cmd output: %s", string(output))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(fmt.Sprintf("create a new hosted cluster kubeconfig: %v", hostedClusterKubeconfigFile))
	}
	e2e.Logf("if you want hostedcluster controlplane namespace, you could get it by combining %s and %s with -", hostedclusterNS, clusterName)
	return clusterName, hostedClusterKubeconfigFile, hostedclusterNS
}

// ValidHypershiftAndGetGuestKubeConfWithNoSkip check if it is hypershift env and get kubeconf of the hosted cluster
// the first return is hosted cluster name
// the second return is the file of kubeconfig of the hosted cluster
// the third return is the hostedcluster namespace in mgmt cluster which contains the generated resources
// if it is not hypershift env, it will not skip the testcase and return null string.
func ValidHypershiftAndGetGuestKubeConfWithNoSkip(oc *CLI) (string, string, string) {
	if IsROSA() {
		e2e.Logf("there is a ROSA env")
		return ROSAValidHypershiftAndGetGuestKubeConf(oc)
	}
	operatorNS := GetHyperShiftOperatorNameSpace(oc)
	if len(operatorNS) <= 0 {
		return "", "", ""
	}

	hostedclusterNS := GetHyperShiftHostedClusterNameSpace(oc)
	if len(hostedclusterNS) <= 0 {
		return "", "", ""
	}

	clusterNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", hostedclusterNS, "hostedclusters", "-o=jsonpath={.items[*].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(clusterNames) <= 0 {
		return "", "", ""
	}

	hypersfhitPodStatus, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", operatorNS, "pod", "-l", "hypershift.openshift.io/operator-component=operator", "-l", "app=operator", "-o=jsonpath={.items[*].status.phase}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(hypersfhitPodStatus).To(o.ContainSubstring("Running"))

	//get first hosted cluster to run test
	e2e.Logf("the hosted cluster names: %s, and will select the first", clusterNames)
	clusterName := strings.Split(clusterNames, " ")[0]

	var hostedClusterKubeconfigFile string
	if os.Getenv("GUEST_KUBECONFIG") != "" {
		e2e.Logf("the kubeconfig you set GUEST_KUBECONFIG must be that of the guestcluster %s in namespace %s", clusterName, hostedclusterNS)
		hostedClusterKubeconfigFile = os.Getenv("GUEST_KUBECONFIG")
		e2e.Logf(fmt.Sprintf("use a known hosted cluster kubeconfig: %v", hostedClusterKubeconfigFile))
	} else {
		hostedClusterKubeconfigFile = "/tmp/guestcluster-kubeconfig-" + clusterName + "-" + getRandomString()
		output, err := exec.Command("bash", "-c", fmt.Sprintf("hypershift create kubeconfig --name %s --namespace %s > %s",
			clusterName, hostedclusterNS, hostedClusterKubeconfigFile)).Output()
		e2e.Logf("the cmd output: %s", string(output))
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(fmt.Sprintf("create a new hosted cluster kubeconfig: %v", hostedClusterKubeconfigFile))
	}
	e2e.Logf("if you want hostedcluster controlplane namespace, you could get it by combining %s and %s with -", hostedclusterNS, clusterName)
	return clusterName, hostedClusterKubeconfigFile, hostedclusterNS
}

// GetHyperShiftOperatorNameSpace get hypershift operator namespace
// if not exist, it will return empty string.
func GetHyperShiftOperatorNameSpace(oc *CLI) string {
	args := []string{
		"pods", "-A",
		"-l", "hypershift.openshift.io/operator-component=operator",
		"-l", "app=operator",
		"--ignore-not-found",
		"-ojsonpath={.items[0].metadata.namespace}",
	}
	namespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(args...).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return namespace
}

// GetHyperShiftHostedClusterNameSpace get hypershift hostedcluster namespace
// if not exist, it will return empty string. If more than one exists, it will return the first one.
func GetHyperShiftHostedClusterNameSpace(oc *CLI) string {
	namespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"hostedcluster", "-A", "--ignore-not-found", "-ojsonpath={.items[*].metadata.namespace}").Output()

	if err != nil && !strings.Contains(namespace, "the server doesn't have a resource type") {
		o.Expect(err).NotTo(o.HaveOccurred(), "get hostedcluster fail: %v", err)
	}

	if len(namespace) <= 0 {
		return namespace
	}
	namespaces := strings.Fields(namespace)
	if len(namespaces) == 1 {
		return namespaces[0]
	}
	ns := ""
	for _, ns = range namespaces {
		if ns != "clusters" {
			break
		}
	}
	return ns
}

// ROSAValidHypershiftAndGetGuestKubeConf check if it is ROSA-hypershift env and get kubeconf of the hosted cluster, only support prow
// the first return is hosted cluster name
// the second return is the file of kubeconfig of the hosted cluster
// the third return is the hostedcluster namespace in mgmt cluster which contains the generated resources
// if it is not hypershift env, it will skip test.
func ROSAValidHypershiftAndGetGuestKubeConf(oc *CLI) (string, string, string) {
	operatorNS := GetHyperShiftOperatorNameSpace(oc)
	if len(operatorNS) <= 0 {
		e2e.Logf("there is no hypershift operator on host cluster")
		return "", "", ""
	}

	data, err := ioutil.ReadFile(os.Getenv("SHARED_DIR") + "/cluster-name")
	if err != nil {
		e2e.Logf("can't get hostedcluster name %s SHARE_DIR: %s", err.Error(), os.Getenv("SHARED_DIR"))
		return "", "", ""
	}
	clusterName := string(data)
	hostedclusterNS, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-A", "hostedclusters", `-o=jsonpath={.items[?(@.metadata.name=="`+clusterName+`")].metadata.namespace}`).Output()
	if len(hostedclusterNS) <= 0 {
		e2e.Logf("there is no hosted cluster NS in mgmt cluster")
	}

	hostedClusterKubeconfigFile := os.Getenv("SHARED_DIR") + "/nested_kubeconfig"
	return clusterName, hostedClusterKubeconfigFile, hostedclusterNS
}
