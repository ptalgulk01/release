package monitoring

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const platformLoadTime = 120
const uwmLoadTime = 180

type monitoringConfig struct {
	name               string
	namespace          string
	enableUserWorkload bool
	template           string
}

func (cm *monitoringConfig) create(oc *exutil.CLI) {
	if !checkConfigMap(oc, "openshift-monitoring", "cluster-monitoring-config") {
		e2e.Logf("Create configmap: cluster-monitoring-config")
		output, err := applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", cm.template, "-p", "NAME="+cm.name, "NAMESPACE="+cm.namespace, "ENABLEUSERWORKLOAD="+fmt.Sprintf("%v", cm.enableUserWorkload))
		if err != nil {
			if strings.Contains(output, "AlreadyExists") {
				err = nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func createUWMConfig(oc *exutil.CLI, uwmMonitoringConfig string) {
	if !checkConfigMap(oc, "openshift-user-workload-monitoring", "user-workload-monitoring-config") {
		e2e.Logf("Create configmap: user-workload-monitoring-config")
		output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", uwmMonitoringConfig).Output()
		if err != nil {
			if strings.Contains(output, "AlreadyExists") {
				err = nil
			}
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

// check if a configmap is created in specific namespace [usage: checkConfigMap(oc, namespace, configmapName)]
func checkConfigMap(oc *exutil.CLI, ns, configmapName string) bool {
	searchOutput, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", configmapName, "-n", ns, "-o=jsonpath={.data.config\\.yaml}").Output()
	if err != nil {
		return false
	}
	if strings.Contains(searchOutput, "retention") {
		return true
	}
	return false
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

// the method is to create one resource with template
func applyResourceFromTemplate(oc *exutil.CLI, parameters ...string) (string, error) {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().Run("process").Args(parameters...).OutputToFile(getRandomString() + "cluster-monitoring.json")
		if err != nil {
			return false, nil
		}
		configFile = output
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))
	return oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Output()
}

func labelNameSpace(oc *exutil.CLI, namespace string, label string) {
	err := oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", namespace, label, "--overwrite").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The namespace %s is labeled by %q", namespace, label)

}

func getSAToken(oc *exutil.CLI, account, ns string) string {
	e2e.Logf("Getting a token assigned to specific serviceaccount from %s namespace...", ns)
	token, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("token", account, "-n", ns).Output()
	if err != nil {
		if strings.Contains(token, "unknown command") {
			token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", account, "-n", ns).Output()
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(token).NotTo(o.BeEmpty())
	return token
}

// check data by running curl on a pod
func checkMetric(oc *exutil.CLI, url, token, metricString string, timeout time.Duration) {
	var metrics string
	var err error
	getCmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err = wait.Poll(3*time.Second, timeout*time.Second, func() (bool, error) {
		metrics, err = exutil.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", getCmd)
		if err != nil || !strings.Contains(metrics, metricString) {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The metrics %s failed to contain %s", metrics, metricString))
}

func createResourceFromYaml(oc *exutil.CLI, ns, yamlFile string) {
	var err error
	err = oc.AsAdmin().Run("apply").Args("-n", ns, "-f", yamlFile).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func deleteBindMonitoringViewRoleToDefaultSA(oc *exutil.CLI, uwmFederateRBACViewName string) {
	err := oc.AdminKubeClient().RbacV1().ClusterRoleBindings().Delete(context.Background(), uwmFederateRBACViewName, metav1.DeleteOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}

func bindMonitoringViewRoleToDefaultSA(oc *exutil.CLI, ns, uwmFederateRBACViewName string) (*rbacv1.ClusterRoleBinding, error) {
	return oc.AdminKubeClient().RbacV1().ClusterRoleBindings().Create(context.Background(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: uwmFederateRBACViewName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-monitoring-view",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: ns,
			},
		},
	}, metav1.CreateOptions{})
}
func deleteClusterRoleBinding(oc *exutil.CLI, clusterRoleBindingName string) {
	err := oc.AdminKubeClient().RbacV1().ClusterRoleBindings().Delete(context.Background(), clusterRoleBindingName, metav1.DeleteOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
}
func bindClusterRoleToUser(oc *exutil.CLI, clusterRoleName, userName, clusterRoleBindingName string) (*rbacv1.ClusterRoleBinding, error) {
	return oc.AdminKubeClient().RbacV1().ClusterRoleBindings().Create(context.Background(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "User",
				Name: userName,
			},
		},
	}, metav1.CreateOptions{})
}

func checkRoute(oc *exutil.CLI, ns, name, token, queryString, metricString string, timeout time.Duration) {
	var metrics string
	var err error
	err = wait.Poll(5*time.Second, timeout*time.Second, func() (bool, error) {
		path, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", name, "-n", ns, "-o=jsonpath={.spec.path}").Output()
		if err != nil {
			return false, nil
		}
		host, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", name, "-n", ns, "-o=jsonpath={.spec.host}").Output()
		if err != nil {
			return false, nil
		}
		metricCMD := fmt.Sprintf("curl -G -s -k -H \"Authorization: Bearer %s\" https://%s%s --data-urlencode '%s'", token, host, path, queryString)
		curlOutput, err := exec.Command("bash", "-c", metricCMD).Output()
		if err != nil {
			return false, nil
		}
		metrics = string(curlOutput)
		if !strings.Contains(metrics, metricString) {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The metrics %s failed to contain %s", metrics, metricString))
}

// check thanos_ruler retention
func checkRetention(oc *exutil.CLI, ns string, sts string, expectedRetention string, timeout time.Duration) {
	err := wait.Poll(5*time.Second, timeout*time.Second, func() (bool, error) {
		stsObject, err := oc.AdminKubeClient().AppsV1().StatefulSets(ns).Get(context.Background(), sts, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		args := stsObject.Spec.Template.Spec.Containers[0].Args
		for _, v := range args {
			if strings.Contains(v, expectedRetention) {
				return true, nil
			}
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("the retention of %s is not expected %s", sts, expectedRetention))
}

func deleteConfig(oc *exutil.CLI, configName, ns string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ConfigMap", configName, "-n", ns, "--ignore-not-found").Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// patch&check enforcedBodySizeLimit value in cluster-monitoring-config
func patchAndCheckBodySizeLimit(oc *exutil.CLI, limitValue string, checkValue string) {
	patchLimit := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "cluster-monitoring-config", "-p", `{"data": {"config.yaml": "prometheusK8s:\n enforcedBodySizeLimit: `+limitValue+`"}}`, "--type=merge", "-n", "openshift-monitoring").Execute()
	o.Expect(patchLimit).NotTo(o.HaveOccurred())
	e2e.Logf("failed to patch enforcedBodySizeLimit value: %v", limitValue)

	checkLimit := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		limit, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "bash", "-c", "cat /etc/prometheus/config_out/prometheus.env.yaml | grep body_size_limit | uniq").Output()
		if err != nil || !strings.Contains(limit, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(checkLimit, "failed to check limit")
}

// check remote write config in the pod
func checkRmtWrtConfig(oc *exutil.CLI, ns string, podName string, checkValue string) {
	envCheck := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		envOutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, "-c", "prometheus", podName, "--", "bash", "-c", fmt.Sprintf(`cat /etc/prometheus/config_out/prometheus.env.yaml | grep '%s'`, checkValue)).Output()
		if err != nil || !strings.Contains(envOutput, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(envCheck, "failed to check remote write config")
}

// check Alerts or Metrics are not exist, Metrics is more recommended to use util `checkMetric`
func checkAlertNotExist(oc *exutil.CLI, url, token, alertName string, timeout time.Duration) {
	cmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err := wait.Poll(3*time.Second, timeout*time.Second, func() (bool, error) {
		chk, err := exutil.RemoteShPod(oc, "openshift-monitoring", "prometheus-k8s-0", "sh", "-c", cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		if err != nil || strings.Contains(chk, alertName) {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Target alert found: %s", alertName))
}

// check alertmanager config in the pod
func checkAlertmanagerConfig(oc *exutil.CLI, ns string, podName string, checkValue string, expectExist bool) {
	envCheck := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		envOutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, "-c", "alertmanager", podName, "--", "bash", "-c", fmt.Sprintf(`cat /etc/alertmanager/config_out/alertmanager.env.yaml | grep '%s'`, checkValue)).Output()
		if expectExist {
			if err != nil || !strings.Contains(envOutput, checkValue) {
				return false, nil
			}
			return true, nil
		}
		if !expectExist {
			if !strings.Contains(envOutput, checkValue) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(envCheck, "failed to check alertmanager config")
}

// check prometheus config in the pod
func checkPrometheusConfig(oc *exutil.CLI, ns string, podName string, checkValue string, expectExist bool) {
	envCheck := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		envOutput, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, "-c", "prometheus", podName, "--", "bash", "-c", fmt.Sprintf(`cat /etc/prometheus/config_out/prometheus.env.yaml | grep '%s'`, checkValue)).Output()
		if expectExist {
			if err != nil || !strings.Contains(envOutput, checkValue) {
				return false, nil
			}
			return true, nil
		}
		if !expectExist {
			if err != nil || !strings.Contains(envOutput, checkValue) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(envCheck, "failed to check prometheus config")
}

// check configuration in the pod in the given time for specific container
func checkConfigInPod(oc *exutil.CLI, namespace string, podName string, containerName string, cmd string, checkValue string) {
	podCheck := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		Output, err := exutil.RemoteShPodWithBashSpecifyContainer(oc, namespace, podName, containerName, cmd)
		if err != nil || !strings.Contains(Output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(podCheck, "failed to check configuration in the pod")
}

// check specific pod logs in container
func checkLogsInContainer(oc *exutil.CLI, namespace string, podName string, containerName string, checkValue string) {
	err := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		Output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", namespace, podName, "-c", containerName).Output()
		if err != nil || !strings.Contains(Output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("failed to find \"%s\" in the pod logs", checkValue))
}

// get specific pod name with label then describe pod info
func getSpecPodInfo(oc *exutil.CLI, ns string, label string, checkValue string) {
	envCheck := wait.Poll(5*time.Second, 180*time.Second, func() (bool, error) {
		podName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, "-ojsonpath={.items[].metadata.name}").Output()
		output, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", podName, "-n", ns).Output()
		if err != nil || !strings.Contains(output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(envCheck, fmt.Sprintf("failed to find \"%s\" in the pod yaml", checkValue))
}

// check pods with label that are fully deleted
func checkPodDeleted(oc *exutil.CLI, ns string, label string, checkValue string) {
	podCheck := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		if err != nil || strings.Contains(output, checkValue) {
			return false, nil
		}
		return true, nil
	})
	exutil.AssertWaitPollNoErr(podCheck, fmt.Sprintf("found \"%s\" exist or not fully deleted", checkValue))
}

// query monitoring metrics, alerts from a specific pod
func queryFromPod(oc *exutil.CLI, url, token, ns, pod, container, metricString string, timeout time.Duration) {
	var metrics string
	var err error
	getCmd := "curl -G -k -s -H \"Authorization:Bearer " + token + "\" " + url
	err = wait.Poll(3*time.Second, timeout*time.Second, func() (bool, error) {
		metrics, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, "-c", container, pod, "--", "bash", "-c", getCmd).Output()
		if err != nil || !strings.Contains(metrics, metricString) {
			return false, nil
		}
		return true, err
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("The metrics %s failed to contain %s", metrics, metricString))
}

// check config exist or absent in yaml/json
func checkYamlconfig(oc *exutil.CLI, ns string, components string, componentsName string, cmd string, checkValue string, expectExist bool) {
	configCheck := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(components, componentsName, cmd, "-n", ns).Output()
		if expectExist {
			if err != nil || !strings.Contains(output, checkValue) {
				return false, nil
			}
			return true, nil
		}
		if !expectExist {
			if err != nil || !strings.Contains(output, checkValue) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(configCheck, fmt.Sprintf("base on `expectExist=%v`, did (not) find \"%s\" exist", expectExist, checkValue))
}

// check logs through label
func checkLogWithLabel(oc *exutil.CLI, namespace string, label string, containerName string, checkValue string, expectExist bool) {
	err := wait.Poll(5*time.Second, 240*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", namespace, "-l", label, "-c", containerName, "--tail=-1").Output()
		if expectExist {
			if err != nil || !strings.Contains(output, checkValue) {
				return false, nil
			}
			return true, nil
		}
		if !expectExist {
			if err != nil || !strings.Contains(output, checkValue) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("failed to find \"%s\" in the pod logs", checkValue))
}
