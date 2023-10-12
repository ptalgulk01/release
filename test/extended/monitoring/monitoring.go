package monitoring

import (
	"os/exec"
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-monitoring] Cluster_Observability parallel monitoring", func() {

	defer g.GinkgoRecover()

	var (
		oc                = exutil.NewCLI("monitor-"+getRandomString(), exutil.KubeConfigPath())
		monitoringCM      monitoringConfig
		monitoringBaseDir string
	)

	g.BeforeEach(func() {
		monitoringBaseDir = exutil.FixturePath("testdata", "monitoring")
		monitoringCMTemplate := filepath.Join(monitoringBaseDir, "cluster-monitoring-cm.yaml")
		// enable user workload monitoring and load other configurations from cluster-monitoring-config configmap
		monitoringCM = monitoringConfig{
			name:               "cluster-monitoring-config",
			namespace:          "openshift-monitoring",
			enableUserWorkload: true,
			template:           monitoringCMTemplate,
		}
		monitoringCM.create(oc)
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-High-49073-Retention size settings for platform", func() {
		checkRetention(oc, "openshift-monitoring", "prometheus-k8s", "storage.tsdb.retention.size=10GiB", platformLoadTime)
		checkRetention(oc, "openshift-monitoring", "prometheus-k8s", "storage.tsdb.retention.time=45d", 20)
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-High-49514-federate service endpoint and route of platform Prometheus", func() {
		var err error
		g.By("Bind cluster-monitoring-view cluster role to current user")
		clusterRoleBindingName := "clusterMonitoringViewFederate"
		defer deleteClusterRoleBinding(oc, clusterRoleBindingName)
		clusterRoleBinding, err := bindClusterRoleToUser(oc, "cluster-monitoring-view", oc.Username(), clusterRoleBindingName)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Created: %v %v", "ClusterRoleBinding", clusterRoleBinding.Name)

		g.By("Get token of current user")
		token := oc.UserConfig().BearerToken
		g.By("check federate endpoint service")
		checkMetric(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/federate --data-urlencode 'match[]=prometheus_build_info'", token, "prometheus_build_info", 3*platformLoadTime)

		g.By("check federate route")
		checkRoute(oc, "openshift-monitoring", "prometheus-k8s-federate", token, "match[]=prometheus_build_info", "prometheus_build_info", 3*platformLoadTime)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-49172-Enable validating webhook for AlertmanagerConfig customer resource", func() {
		var (
			err                       error
			output                    string
			namespace                 string
			invalidAlertmanagerConfig = filepath.Join(monitoringBaseDir, "invalid-alertmanagerconfig.yaml")
			validAlertmanagerConfig   = filepath.Join(monitoringBaseDir, "valid-alertmanagerconfig.yaml")
		)

		g.By("Get prometheus-operator-admission-webhook deployment")
		err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "prometheus-operator-admission-webhook", "-n", "openshift-monitoring").Execute()
		if err != nil {
			e2e.Logf("Unable to get deployment prometheus-operator-admission-webhook.")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		oc.SetupProject()
		namespace = oc.Namespace()

		g.By("Create invalid AlertmanagerConfig, should throw out error")
		output, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", invalidAlertmanagerConfig, "-n", namespace).Output()
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The AlertmanagerConfig \"invalid-test-config\" is invalid"))

		g.By("Create valid AlertmanagerConfig, should not have error")
		output, err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", validAlertmanagerConfig, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("valid-test-config created"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-42800-Allow configuration of the log level for Alertmanager in the CMO configmap", func() {
		g.By("Check alertmanager container logs")
		exutil.WaitAndGetSpecificPodLogs(oc, "openshift-monitoring", "alertmanager", "alertmanager-main-0", "level=debug")
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-43748-Ensure label namespace exists on all alerts", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check alerts, should have label namespace exists on all alerts")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="Watchdog"}'`, token, `"namespace":"openshift-monitoring"`, 2*platformLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-47307-Add external label of origin to platform alerts", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check alerts, could see the `openshift_io_alert_source` field for in-cluster alerts")
		checkMetric(oc, "https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts", token, `"openshift_io_alert_source":"platform"`, 2*platformLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-45163-Show labels for pods/nodes/namespaces/PV/PVC/PDB in metrics", func() {
		var (
			ns          string
			helloPodPvc = filepath.Join(monitoringBaseDir, "helloPodPvc.yaml")
		)
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("create project ns then attach pv/pvc")
		oc.SetupProject()
		ns = oc.Namespace()
		createResourceFromYaml(oc, ns, helloPodPvc)

		g.By("Check labels for pod")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_labels{pod="alertmanager-main-0"}'`, token, `"label_statefulset_kubernetes_io_pod_name"`, uwmLoadTime)

		g.By("Check labels for node")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_node_labels'`, token, `"label_kubernetes_io_hostname"`, uwmLoadTime)

		g.By("Check labels for namespace")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_namespace_labels{namespace="openshift-monitoring"}'`, token, `"label_kubernetes_io_metadata_name"`, uwmLoadTime)

		g.By("Check labels for PDB")
		//SNO cluster do not have PDB under openshift-monitoring
		if !exutil.IsSNOCluster(oc) {
			checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_poddisruptionbudget_labels{poddisruptionbudget="thanos-querier-pdb"}'`, token, `"label_app_kubernetes_io_name"`, uwmLoadTime)
		}

		g.By("Check labels for PV/PVC")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_persistentvolume_labels'`, token, `"persistentvolume"`, 2*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_persistentvolumeclaim_labels'`, token, `"persistentvolumeclaim"`, 2*uwmLoadTime)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-45271-Allow OpenShift users to configure audit logs for prometheus-adapter", func() {
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus-adapter")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("prometheus-adapter Pods: %v", podList)

		g.By("check the audit logs")
		for _, pod := range podList {
			exutil.AssertPodToBeReady(oc, pod, "openshift-monitoring")
			output, err := exutil.RemoteShContainer(oc, "openshift-monitoring", pod, "prometheus-adapter", "cat", "/var/log/adapter/audit.log")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(output, `"level":"Request"`)).To(o.BeTrue(), "level Request is not in audit.log")
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-48432-Allow OpenShift users to configure request logging for Thanos Querier query endpoint", func() {
		g.By("check thanos-querier pods are normal and able to see the request.logging-config setting")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "thanos-querier", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring(`request.logging-config`))

		//thanos-querier pod name will changed when cm modified, pods may not restart yet during the first check
		g.By("double confirm thanos-querier pods are ready")
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/instance=thanos-querier")
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range podList {
			exutil.AssertPodToBeReady(oc, pod, "openshift-monitoring")
		}

		g.By("query with thanos-querier svc")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=cluster_version'`, token, `"cluster_version"`, 3*uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=cluster_version'`, token, `"cluster-version-operator"`, 3*uwmLoadTime)

		g.By("check from thanos-querier logs")
		//oc -n openshift-monitoring logs -l app.kubernetes.io/instance=thanos-querier -c thanos-query --tail=-1
		checkLogWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/instance=thanos-querier", "thanos-query", "query=cluster_version", true)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Low-43038-Should not have error for loading OpenAPI spec for v1beta1.metrics.k8s.io", func() {
		var (
			searchString string
			result       string
		)
		searchString = "loading OpenAPI spec for \"v1beta1.metrics.k8s.io\" failed with:"
		podList, err := exutil.GetAllPodsWithLabel(oc, "openshift-kube-apiserver", "app=openshift-kube-apiserver")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("kube-apiserver Pods: %v", podList)

		g.By("check the kube-apiserver logs, should not have error for v1beta1.metrics.k8s.io")
		for _, pod := range podList {
			exutil.AssertPodToBeReady(oc, pod, "openshift-kube-apiserver")
			result, _ = exutil.GetSpecificPodLogs(oc, "openshift-kube-apiserver", "kube-apiserver", pod, searchString)
			e2e.Logf("output result in logs: %v", result)
			o.Expect(len(result) == 0).To(o.BeTrue(), "found the error logs which is unexpected")
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Low-55670-Prometheus should not collecting error messages for completed pods", func() {
		var output string
		g.By("check pod conditioning in openshift-kube-scheduler, all pods should be ready")
		exutil.AssertAllPodsToBeReady(oc, "openshift-kube-scheduler")

		g.By("get prometheus-adapter pod names")
		prometheusAdapterPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus-adapter")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check prometheus-adapter pod logs")
		for _, pod := range prometheusAdapterPodNames {
			output, _ = oc.AsAdmin().WithoutNamespace().Run("logs").Args(pod, "-n", "openshift-monitoring").Output()
			if strings.Contains(output, "unable to fetch CPU metrics for pod") {
				e2e.Failf("found unexpected logs: unable to fetch CPU metrics for pod")
			}
		}
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-55767-Missing metrics in kube-state-metrics", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check kube-state-metrics metrics, the following metrics should be visible")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_container_status_terminated_reason"`, uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_init_container_status_terminated_reason"`, uwmLoadTime)
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/label/__name__/values`, token, `"kube_pod_status_scheduled_time"`, uwmLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-56168-PreChkUpgrade-NonPreRelease-Prometheus never sees endpoint propagation of a deleted pod", func() {
		var (
			ns          = "56168-upgrade-ns"
			exampleApp  = filepath.Join(monitoringBaseDir, "example-app.yaml")
			roleBinding = filepath.Join(monitoringBaseDir, "sa-prometheus-k8s-access.yaml")
		)
		g.By("Create example app")
		oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", ns).Execute()
		createResourceFromYaml(oc, ns, exampleApp)
		exutil.AssertAllPodsToBeReady(oc, ns)

		g.By("add role and role binding for example app")
		createResourceFromYaml(oc, ns, roleBinding)

		g.By("label namespace")
		oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/cluster-monitoring=true").Execute()

		g.By("check target is up")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, "up", 2*uwmLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-56168-PstChkUpgrade-NonPreRelease-Prometheus never sees endpoint propagation of a deleted pod", func() {
		g.By("get the ns name in PreChkUpgrade")
		ns := "56168-upgrade-ns"

		g.By("delete related resource at the end of case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("project", ns).Execute()

		g.By("delete example app deployment")
		deleteApp, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("deploy", "prometheus-example-app", "-n", ns).Output()
		o.Expect(deleteApp).To(o.ContainSubstring(`"prometheus-example-app" deleted`))

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check metric up==0 under the test project, return null")
		checkMetric(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=up{namespace=\"56168-upgrade-ns\"}==0'", token, `"result":[]`, 2*uwmLoadTime)

		g.By("check no alert 'TargetDown'")
		checkAlertNotExist(oc, "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{namespace=\"56168-upgrade-ns\"}'", token, "TargetDown", uwmLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-57254-oc adm top node/pod output should not give negative numbers", func() {
		g.By("check on node")
		checkNode, err := exec.Command("bash", "-c", `oc adm top node | awk '{print $2,$3,$4,$5}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkNode).NotTo(o.ContainSubstring("-"))

		g.By("check on pod under specific namespace")
		checkNs, err := exec.Command("bash", "-c", `oc -n openshift-monitoring adm top pod | awk '{print $2,$3}'`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(checkNs).NotTo(o.ContainSubstring("-"))
	})

	// author: tagao@redhat.com
	g.It("ConnectedOnly-Author:tagao-Medium-55696-add telemeter alert TelemeterClientFailures", func() {
		g.By("check TelemeterClientFailures alert is added")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "telemetry", "-ojsonpath={.spec.groups}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("TelemeterClientFailures"))
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-62092-Don't fire NodeFilesystemAlmostOutOfSpace alert for certain tmpfs mount points", func() {
		g.By("check NodeFilesystemAlmostOutOfSpace alert from node-exporter-rules prometheusrules")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheusrules", "node-exporter-rules", `-ojsonpath={.spec.groups[*].rules[?(@.alert=="NodeFilesystemAlmostOutOfSpace")].expr}`, "-n", "openshift-monitoring").Output()
		e2e.Logf("NodeFilesystemAlmostOutOfSpace alert expr: %v", output)
		g.By("mountpoint /var/lib/ibmc-s3fs.* is excluded")
		o.Expect(output).To(o.ContainSubstring(`mountpoint!~"/var/lib/ibmc-s3fs.*"`))
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-48350-create alert-routing-edit role to allow end users to manage alerting CR", func() {
		var (
			alertManagerConfig = filepath.Join(monitoringBaseDir, "valid-alertmanagerconfig.yaml")
		)
		g.By("check clusterrole alert-routing-edit exists")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterrole").Output()
		o.Expect(strings.Contains(output, "alert-routing-edit")).To(o.BeTrue())

		g.By("create project, add alert-routing-edit RoleBinding to specific user")
		oc.SetupProject()
		ns := oc.Namespace()
		err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-role-to-user", "-n", ns, "alert-routing-edit", oc.Username()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create AlertmanagerConfig under the project")
		createResourceFromYaml(oc, ns, alertManagerConfig)

		g.By("check AlertmanagerConfig is created")
		output, _ = oc.WithoutNamespace().Run("get").Args("AlertmanagerConfig", "-n", ns).Output()
		o.Expect(output).To(o.ContainSubstring("valid-test-config"))

		g.By("the user should able to change AlertmanagerConfig")
		err = oc.WithoutNamespace().Run("patch").Args("AlertmanagerConfig", "valid-test-config", "-p", `{"spec":{"receivers":[{"name":"webhook","webhookConfigs":[{"url":"https://test.io/push"}]}]}}`, "--type=merge", "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check AlertmanagerConfig is updated")
		output, _ = oc.WithoutNamespace().Run("get").Args("AlertmanagerConfig", "valid-test-config", "-ojsonpath={.spec.receivers}", "-n", ns).Output()
		o.Expect(output).To(o.ContainSubstring("https://test.io/push"))

		g.By("the user should able to delete AlertmanagerConfig")
		err = oc.WithoutNamespace().Run("delete").Args("AlertmanagerConfig", "valid-test-config", "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check AlertmanagerConfig is deleted")
		output, _ = oc.WithoutNamespace().Run("get").Args("AlertmanagerConfig", "-n", ns).Output()
		o.Expect(output).NotTo(o.ContainSubstring("valid-test-config"))
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Low-62957-Prometheus and Alertmanager should configure ExternalURL correctly", func() {
		g.By("get console route")
		consoleURL, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "console", `-ojsonpath={.spec.host}`, "-n", "openshift-console").Output()
		e2e.Logf("console route is: %v", consoleURL)

		g.By("get externalUrl for alertmanager main")
		alertExternalUrl, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanager", "main", `-ojsonpath={.spec.externalUrl}`, "-n", "openshift-monitoring").Output()
		e2e.Logf("alertmanager main externalUrl is: %v", alertExternalUrl)
		o.Expect(alertExternalUrl).To(o.ContainSubstring("https://" + consoleURL))

		g.By("get externalUrl for prometheus k8s")
		prometheusExternalUrl, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheus", "k8s", `-ojsonpath={.spec.externalUrl}`, "-n", "openshift-monitoring").Output()
		e2e.Logf("prometheus k8s externalUrl is: %v", prometheusExternalUrl)
		o.Expect(prometheusExternalUrl).To(o.ContainSubstring("https://" + consoleURL))

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check from alertmanager API, the generatorURL should include https://${consoleURL}")
		checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts?&filter={alertname="Watchdog"}`, token, `"generatorURL":"https://`+consoleURL, 2*platformLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-48942-validation for scrapeTimeout and relabel configs", func() {
		var (
			invalidServiceMonitor = filepath.Join(monitoringBaseDir, "invalid-ServiceMonitor.yaml")
		)
		g.By("delete test ServiceMonitor at the end of case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("servicemonitor", "console-test-monitoring", "-n", "openshift-monitoring").Execute()

		g.By("create one ServiceMonitor, set scrapeTimeout bigger than scrapeInterval, and no targetLabel setting")
		createResourceFromYaml(oc, "openshift-monitoring", invalidServiceMonitor)

		g.By("get prometheus-operator pod name with label")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		PodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-ojsonpath={.items[*].metadata.name}", "-l", "app.kubernetes.io/name=prometheus-operator", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("able to see error in prometheus-operator logs")
		for _, pod := range strings.Fields(PodNames) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-c", "prometheus-operator", pod, "-n", "openshift-monitoring").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, `error="scrapeTimeout \"120s\" greater than scrapeInterval \"30s\""`) {
				e2e.Failf("ServiceMonitor in not contain scrapeTimeout=120s, or not take effect yet")
			}
		}

		g.By("check the configuration is not loaded to prometheus")
		checkPrometheusConfig(oc, "openshift-monitoring", "prometheus-k8s-0", `serviceMonitor/openshift-monitoring/console-test-monitoring/0`, false)

		g.By("edit ServiceMonitor, and set value for scrapeTimeout less than scrapeInterval")
		//oc patch servicemonitor console-test-monitoring --type='json' -p='[{"op": "replace", "path": "/spec/endpoints/0/scrapeTimeout", "value":"20s"}]' -n openshift-monitoring
		patchConfig := `[{"op": "replace", "path": "/spec/endpoints/0/scrapeTimeout", "value":"20s"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("servicemonitor", "console-test-monitoring", "-p", patchConfig, "--type=json", "-n", "openshift-monitoring").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("able to see error for missing targetLabel in prometheus-operator logs")
		for _, pod := range strings.Fields(PodNames) {
			checkLogsInContainer(oc, "openshift-monitoring", pod, "prometheus-operator", `error="relabel configuration for replace action needs targetLabel value"`)
		}

		g.By("add targetLabel to ServiceMonitor")
		//oc -n openshift-monitoring patch servicemonitor console-test-monitoring --type='json' -p='[{"op": "add", "path": "/spec/endpoints/0/relabelings/0/targetLabel", "value": "namespace"}]'
		patchConfig = `[{"op": "add", "path": "/spec/endpoints/0/relabelings/0/targetLabel", "value": "namespace"}]`
		err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("servicemonitor", "console-test-monitoring", "-p", patchConfig, "--type=json", "-n", "openshift-monitoring").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check the configuration loaded to prometheus")
		checkPrometheusConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "serviceMonitor/openshift-monitoring/console-test-monitoring/0", true)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-62636-Graduate alert overrides and alert relabelings to GA", func() {
		var (
			alertingRule       = filepath.Join(monitoringBaseDir, "alertingRule.yaml")
			alertRelabelConfig = filepath.Join(monitoringBaseDir, "alertRelabelConfig.yaml")
		)
		g.By("delete the created AlertingRule/AlertRelabelConfig at the end of the case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("AlertingRule", "monitoring-example", "-n", "openshift-monitoring").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("AlertRelabelConfig", "monitoring-watchdog", "-n", "openshift-monitoring").Execute()

		g.By("check AlertingRule/AlertRelabelConfig apiVersion is v1")
		_, explainErr := oc.WithoutNamespace().AsAdmin().Run("explain").Args("AlertingRule", "--api-version=monitoring.openshift.io/v1").Output()
		o.Expect(explainErr).NotTo(o.HaveOccurred())

		_, explainErr = oc.WithoutNamespace().AsAdmin().Run("explain").Args("AlertRelabelConfig", "--api-version=monitoring.openshift.io/v1").Output()
		o.Expect(explainErr).NotTo(o.HaveOccurred())

		g.By("create AlertingRule/AlertRelabelConfig under openshift-monitoring")
		createResourceFromYaml(oc, "openshift-monitoring", alertingRule)
		createResourceFromYaml(oc, "openshift-monitoring", alertRelabelConfig)

		g.By("check AlertingRule/AlertRelabelConfig are created")
		output, _ := oc.WithoutNamespace().Run("get").Args("AlertingRule/monitoring-example", "-ojsonpath={.metadata.name}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("monitoring-example"))
		output, _ = oc.WithoutNamespace().Run("get").Args("AlertRelabelConfig/monitoring-watchdog", "-ojsonpath={.metadata.name}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("monitoring-watchdog"))

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check the alert defined in AlertingRule could be found in thanos-querier API")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="ExampleAlert"}'`, token, `"alertname":"ExampleAlert"`, 2*platformLoadTime)

		g.By("Watchdog alert, the alert label is changed from \"severity\":\"none\" to \"severity\":\"critical\" in alertmanager API")
		checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts?&filter={alertname="Watchdog"}`, token, `"severity":"critical"`, 2*platformLoadTime)
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-66860-add startup probe for prometheus-adapter", func() {
		g.By("check startupProbe config in prometheus-adapter deployment")
		// % oc -n openshift-monitoring get deploy prometheus-adapter -ojsonpath='{.spec.template.spec.containers[?(@.name=="prometheus-adapter")].startupProbe}'
		output, deployErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "prometheus-adapter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-adapter\")].startupProbe}", "-n", "openshift-monitoring").Output()
		o.Expect(deployErr).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(`{"failureThreshold":18,"httpGet":{"path":"/livez","port":"https","scheme":"HTTPS"},"periodSeconds":10,"successThreshold":1,"timeoutSeconds":1}`))

		g.By("check prometheus-adapter pod logs, should not see crashlooping logs")
		// % oc -n openshift-monitoring logs -l app.kubernetes.io/name=prometheus-adapter -c prometheus-adapter --tail=-1
		output, logsErr := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-l", "app.kubernetes.io/name=prometheus-adapter", "-c", "prometheus-adapter", "--tail=-1", "-n", "openshift-monitoring").Output()
		o.Expect(logsErr).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "Shutting down controller")).NotTo(o.BeTrue())
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Low-67008-node-exporter: disable btrfs collector", func() {
		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("should not see btrfs collector related metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="btrfs"}'`, token, "\"result\":[]", uwmLoadTime)

		g.By("check btrfs collector is disabled by default")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("no-collector.btrfs"))
	})

	g.Context("user workload monitoring", func() {
		var (
			uwmMonitoringConfig string
		)
		g.BeforeEach(func() {
			monitoringBaseDir = exutil.FixturePath("testdata", "monitoring")
			uwmMonitoringConfig = filepath.Join(monitoringBaseDir, "uwm-monitoring-cm.yaml")
			createUWMConfig(oc, uwmMonitoringConfig)
		})

		g.When("Need example app", func() {
			var (
				ns         string
				exampleApp string
			)
			g.BeforeEach(func() {
				exampleApp = filepath.Join(monitoringBaseDir, "example-app.yaml")
				//create project
				oc.SetupProject()
				ns = oc.Namespace()
				//create example app and alert rule under the project
				g.By("Create example app!")
				createResourceFromYaml(oc, ns, exampleApp)
				exutil.AssertAllPodsToBeReady(oc, ns)
			})

			// author: hongyli@redhat.com
			g.It("Author:hongyli-Critical-43341-Exclude namespaces from user workload monitoring based on label", func() {
				var (
					exampleAppRule = filepath.Join(monitoringBaseDir, "example-alert-rule.yaml")
				)

				g.By("label project not being monitored")
				labelNameSpace(oc, ns, "openshift.io/user-monitoring=false")

				//create example app and alert rule under the project
				g.By("Create example alert rule!")
				createResourceFromYaml(oc, ns, exampleAppRule)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("check metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "\"result\":[]", 2*uwmLoadTime)
				g.By("check alerts")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{namespace=\""+ns+"\"}'", token, "\"result\":[]", uwmLoadTime)

				g.By("label project being monitored")
				labelNameSpace(oc, ns, "openshift.io/user-monitoring=true")

				g.By("check metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "prometheus-example-app", 2*uwmLoadTime)

				g.By("check alerts")
				checkMetric(oc, "https://thanos-ruler.openshift-user-workload-monitoring.svc:9091/api/v1/alerts", token, "TestAlert", uwmLoadTime)
			})

			// author: hongyli@redhat.com
			g.It("Author:hongyli-High-50024-High-49515-Check federate route and service of user workload Prometheus", func() {
				var err error
				g.By("Bind cluster-monitoring-view RBAC to default service account")
				uwmFederateRBACViewName := "uwm-federate-rbac-" + ns
				defer deleteBindMonitoringViewRoleToDefaultSA(oc, uwmFederateRBACViewName)
				clusterRoleBinding, err := bindMonitoringViewRoleToDefaultSA(oc, ns, uwmFederateRBACViewName)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("Created: %v %v", "ClusterRoleBinding", clusterRoleBinding.Name)
				g.By("Get token of default service account")
				token := getSAToken(oc, "default", ns)

				g.By("check uwm federate endpoint service")
				checkMetric(oc, "https://prometheus-user-workload.openshift-user-workload-monitoring.svc:9092/federate --data-urlencode 'match[]=version'", token, "prometheus-example-app", uwmLoadTime)

				g.By("check uwm federate route")
				checkRoute(oc, "openshift-user-workload-monitoring", "federate", token, "match[]=version", "prometheus-example-app", 20)

			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-50241-Prometheus (uwm) externalLabels not showing always in alerts", func() {
				var (
					exampleAppRule = filepath.Join(monitoringBaseDir, "in-cluster_query_alert_rule.yaml")
				)
				g.By("Create alert rule with expression about data provided by in-cluster prometheus")
				createResourceFromYaml(oc, ns, exampleAppRule)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("Check labelmy is in the alert")
				checkMetric(oc, "https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts", token, "labelmy", 2*uwmLoadTime)
			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-42825-Expose EnforcedTargetLimit in the CMO configuration for UWM", func() {
				g.By("check user metrics")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "prometheus-example-app", 2*uwmLoadTime)

				g.By("scale deployment replicas to 2")
				oc.WithoutNamespace().Run("scale").Args("deployment", "prometheus-example-app", "--replicas=2", "-n", ns).Execute()

				g.By("check user metrics again, the user metrics can't be found from thanos-querier")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version{namespace=\""+ns+"\"}'", token, "\"result\":[]", 2*uwmLoadTime)
			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-49189-Enforce label scrape limits for UWM [Serial]", func() {
				var (
					invalidUWM = filepath.Join(monitoringBaseDir, "invalid-uwm.yaml")
				)
				g.By("delete uwm-config/cm-config at the end of a serial case")
				defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
				defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

				g.By("Get token of SA prometheus-k8s")
				token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

				g.By("query metrics from thanos-querier")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version'", token, "prometheus-example-app", uwmLoadTime)

				g.By("trigger label_limit exceed")
				createResourceFromYaml(oc, "openshift-user-workload-monitoring", invalidUWM)

				g.By("check in thanos-querier /targets api, it should complains the label_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_limit exceeded`, 2*uwmLoadTime)

				g.By("trigger label_name_length_limit exceed")
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 8\n enforcedLabelNameLengthLimit: 1\n enforcedLabelValueLengthLimit: 1\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				g.By("check in thanos-querier /targets api, it should complains the label_name_length_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_name_length_limit exceeded`, 2*uwmLoadTime)

				g.By("trigger label_value_length_limit exceed")
				err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 8\n enforcedLabelNameLengthLimit: 8\n enforcedLabelValueLengthLimit: 1\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err2).NotTo(o.HaveOccurred())

				g.By("check in thanos-querier /targets api, it should complains the label_value_length_limit exceeded")
				checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/targets`, token, `label_value_length_limit exceeded`, 2*uwmLoadTime)

				g.By("relax restrictions")
				err3 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "prometheus:\n enforcedLabelLimit: 10\n enforcedLabelNameLengthLimit: 10\n enforcedLabelValueLengthLimit: 50\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
				o.Expect(err3).NotTo(o.HaveOccurred())

				g.By("able to see the metrics")
				checkMetric(oc, "https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=version'", token, "prometheus-example-app", 2*uwmLoadTime)
			})

			// author: tagao@redhat.com
			g.It("Author:tagao-Medium-44805-Expose tenancy-aware labels and values of api v1 label endpoints for Thanos query", func() {
				var (
					rolebinding = filepath.Join(monitoringBaseDir, "rolebinding.yaml")
				)
				g.By("add RoleBinding to specific user")
				createResourceFromYaml(oc, ns, rolebinding)
				//oc -n ns1 patch RoleBinding view -p '{"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"User","name":"${user}"}]}'
				err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("RoleBinding", "view", "-p", `{"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"User","name":"`+oc.Username()+`"}]}`, "--type=merge", "-n", ns).Execute()
				o.Expect(err).NotTo(o.HaveOccurred())

				g.By("get user API token")
				token := oc.UserConfig().BearerToken

				g.By("check namespace labels") //There are many labels, only check the few ones
				checkMetric(oc, "\"https://thanos-querier.openshift-monitoring.svc:9092/api/v1/labels?namespace="+oc.Namespace()+"\"", token, `"__name__"`, 2*uwmLoadTime)
				checkMetric(oc, "\"https://thanos-querier.openshift-monitoring.svc:9092/api/v1/labels?namespace="+oc.Namespace()+"\"", token, `"version"`, 2*uwmLoadTime)
				checkMetric(oc, "\"https://thanos-querier.openshift-monitoring.svc:9092/api/v1/labels?namespace="+oc.Namespace()+"\"", token, `"cluster_ip"`, 2*uwmLoadTime)

				g.By("show label value")
				checkMetric(oc, "\"https://thanos-querier.openshift-monitoring.svc:9092/api/v1/label/version/values?namespace="+oc.Namespace()+"\"", token, `"v0.4.1"`, 2*uwmLoadTime)

				g.By("check with a specific series")
				checkMetric(oc, "\"https://thanos-querier.openshift-monitoring.svc:9092/api/v1/series?match[]=version&namespace="+oc.Namespace()+"\"", token, `"service":"prometheus-example-app"`, 2*uwmLoadTime)
			})
		})

		// author: hongyli@redhat.com
		g.It("Author:hongyli-High-49745-High-50519-Retention for UWM Prometheus and thanos ruler", func() {
			g.By("Check retention size of prometheus user workload")
			checkRetention(oc, "openshift-user-workload-monitoring", "prometheus-user-workload", "storage.tsdb.retention.size=5GiB", uwmLoadTime)
			g.By("Check retention of prometheus user workload")
			checkRetention(oc, "openshift-user-workload-monitoring", "prometheus-user-workload", "storage.tsdb.retention.time=15d", 20)
			g.By("Check retention of thanos ruler")
			checkRetention(oc, "openshift-user-workload-monitoring", "thanos-ruler-user-workload", "retention=15d", uwmLoadTime)
		})

		// author: juzhao@redhat.com
		g.It("Author:juzhao-Medium-42956-Should not have PrometheusNotIngestingSamples alert if enabled user workload monitoring only", func() {
			g.By("Get token of SA prometheus-k8s")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

			g.By("check alerts, Should not have PrometheusNotIngestingSamples alert fired")
			checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusNotIngestingSamples"}'`, token, `"result":[]`, uwmLoadTime)
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-46301-Allow OpenShift users to configure query log file for Prometheus", func() {
			g.By("make sure all pods in openshift-monitoring/openshift-user-workload-monitoring are ready")
			exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
			exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")

			g.By("check query log file for prometheus in openshift-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "curl", "http://localhost:9090/api/v1/query?query=prometheus_build_info").Execute()
			output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "--", "bash", "-c", "cat /tmp/promethues_query.log | grep prometheus_build_info").Output()
			o.Expect(output).To(o.ContainSubstring("prometheus_build_info"))

			g.By("check query log file for prometheus in openshift-user-workload-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-user-workload-monitoring", "-c", "prometheus", "prometheus-user-workload-0", "--", "curl", "http://localhost:9090/api/v1/query?query=up").Execute()
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-user-workload-monitoring", "-c", "prometheus", "prometheus-user-workload-0", "--", "bash", "-c", "cat /tmp/uwm_query.log | grep up").Output()
			o.Expect(output2).To(o.ContainSubstring("up"))
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-50008-Expose sigv4 settings for remote write in the CMO configuration [Serial]", func() {
			var (
				sigv4ClusterCM = filepath.Join(monitoringBaseDir, "sigv4-cluster-monitoring-cm.yaml")
				sigv4UwmCM     = filepath.Join(monitoringBaseDir, "sigv4-uwm-monitoring-cm.yaml")
				sigv4Secret    = filepath.Join(monitoringBaseDir, "sigv4-secret.yaml")
				sigv4SecretUWM = filepath.Join(monitoringBaseDir, "sigv4-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "sigv4-credentials-uwm", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "sigv4-credentials", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create sigv4 secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", sigv4Secret)

			g.By("Configure remote write sigv4 and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", sigv4ClusterCM)

			g.By("Check sig4 config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "sigv4:")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "region: us-central1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "access_key: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "secret_key: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "profile: SomeProfile")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "role_arn: SomeRoleArn")

			g.By("Create sigv4 secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", sigv4SecretUWM)

			g.By("Configure remote write sigv4 setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", sigv4UwmCM)

			g.By("Check sig4 config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "sigv4:")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "region: us-east2")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "access_key: basic_user_uwm")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "secret_key: basic_pass_uwm")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "profile: umw_Profile")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "role_arn: umw_RoleArn")
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-49694-Expose OAuth2 settings for remote write in the CMO configuration [Serial]", func() {
			var (
				oauth2ClusterCM = filepath.Join(monitoringBaseDir, "oauth2-cluster-monitoring-cm.yaml")
				oauth2UwmCM     = filepath.Join(monitoringBaseDir, "oauth2-uwm-monitoring-cm.yaml")
				oauth2Secret    = filepath.Join(monitoringBaseDir, "oauth2-secret.yaml")
				oauth2SecretUWM = filepath.Join(monitoringBaseDir, "oauth2-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "oauth2-credentials", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "oauth2-credentials", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create oauth2 secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", oauth2Secret)

			g.By("Configure remote write oauth2 and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", oauth2ClusterCM)

			g.By("Check oauth2 config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://test.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "remote_timeout: 30s")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "client_id: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "client_secret: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "token_url: https://example.com/oauth2/token")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "scope1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "scope2")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "param1: value1")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "param2: value2")

			g.By("Create oauth2 secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", oauth2SecretUWM)

			g.By("Configure remote write oauth2 setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", oauth2UwmCM)

			g.By("Check oauth2 config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://test.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "remote_timeout: 30s")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "client_id: basic_user")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "client_secret: basic_pass")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "token_url: https://example.com/oauth2/token")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "scope3")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "scope4")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "param3: value3")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "param4: value4")
		})

		//author: tagao@redhat.com
		g.It("Author:tagao-Medium-47519-Platform prometheus operator should reconcile AlertmanagerConfig resources from user namespaces [Serial]", func() {
			var (
				enableAltmgrConfig = filepath.Join(monitoringBaseDir, "enableUserAlertmanagerConfig.yaml")
				wechatConfig       = filepath.Join(monitoringBaseDir, "exampleAlertConfigAndSecret.yaml")
			)
			g.By("delete uwm-config/cm-config at the end of a serial case")
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("enable alert manager config")
			createResourceFromYaml(oc, "openshift-monitoring", enableAltmgrConfig)
			exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

			g.By("check the initial alertmanager configuration")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "alertname = Watchdog", true)

			g.By("create&check alertmanagerconfig under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", wechatConfig)
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", "openshift-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("config-example"))
			o.Expect(output).To(o.ContainSubstring("wechat-config"))

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration (should not)")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)

			g.By("delete the alertmanagerconfig/secret created under openshift-monitoring")
			oc.AsAdmin().WithoutNamespace().Run("delete").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", "openshift-monitoring").Execute()

			g.By("create one new project, label the namespace and create the same AlertmanagerConfig")
			oc.SetupProject()
			ns := oc.Namespace()
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/user-monitoring=false").Execute()

			g.By("create&check alertmanagerconfig under the namespace")
			createResourceFromYaml(oc, ns, wechatConfig)
			output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanagerconfig/config-example", "secret/wechat-config", "-n", ns).Output()
			o.Expect(output2).To(o.ContainSubstring("config-example"))
			o.Expect(output2).To(o.ContainSubstring("wechat-config"))

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration (should not)")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)

			g.By("update the label to true")
			oc.AsAdmin().WithoutNamespace().Run("label").Args("namespace", ns, "openshift.io/user-monitoring=true", "--overwrite").Execute()

			g.By("check if the new created AlertmanagerConfig is reconciled in the Alertmanager configuration")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", true)

			g.By("set enableUserAlertmanagerConfig to false")
			err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "cluster-monitoring-config", "-p", `{"data": {"config.yaml": "alertmanagerMain:\n enableUserAlertmanagerConfig: false\n"}}`, "--type=merge", "-n", "openshift-monitoring").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("the AlertmanagerConfig from user project is removed")
			checkAlertmangerConfig(oc, "openshift-monitoring", "alertmanager-main-0", "wechat", false)
		})

		g.It("Author:tagao-Medium-49404-Medium-49176-Expose Authorization settings for remote write in the CMO configuration, Add the relabel config to all user-supplied remote_write configurations [Serial]", func() {
			var (
				authClusterCM = filepath.Join(monitoringBaseDir, "auth-cluster-monitoring-cm.yaml")
				authUwmCM     = filepath.Join(monitoringBaseDir, "auth-uwm-monitoring-cm.yaml")
				authSecret    = filepath.Join(monitoringBaseDir, "auth-secret.yaml")
				authSecretUWM = filepath.Join(monitoringBaseDir, "auth-secret-uwm.yaml")
			)
			g.By("delete secret/cm at the end of case")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "rw-auth", "-n", "openshift-user-workload-monitoring").Execute()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "rw-auth", "-n", "openshift-monitoring").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("Create auth secret under openshift-monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", authSecret)

			g.By("Configure remote write auth and enable user workload monitoring")
			createResourceFromYaml(oc, "openshift-monitoring", authClusterCM)

			g.By("Check auth config under openshift-monitoring")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://remote-write.endpoint")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "target_label: __tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://basicAuth.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "username: basic_user")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "password: basic_pass")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "__tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-monitoring", "prometheus-k8s-0", "target_label: cluster_id")

			g.By("Create auth secret under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", authSecretUWM)

			g.By("Configure remote write auth setting for user workload monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", authUwmCM)

			g.By("Check auth config under openshift-user-workload-monitoring")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://remote-write.endpoint")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "target_label: __tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://basicAuth.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "username: basic_user")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "password: basic_pass")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://bearerTokenFile.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "url: https://authorization.remotewrite.com/api/write")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "__tmp_openshift_cluster_id__")
			checkRmtWrtConfig(oc, "openshift-user-workload-monitoring", "prometheus-user-workload-0", "target_label: cluster_id_1")
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Low-43037-Should not have error for oc adm inspect clusteroperator monitoring command", func() {
			g.By("delete must-gather file at the end of case")
			defer exec.Command("bash", "-c", "rm -rf /tmp/must-gather-43037").Output()

			g.By("oc adm inspect clusteroperator monitoring")
			exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
			output, _ := oc.AsAdmin().WithoutNamespace().Run("adm").Args("inspect", "clusteroperator", "monitoring", "--dest-dir=/tmp/must-gather-43037").Output()
			o.Expect(output).NotTo(o.ContainSubstring("error"))
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-32224-Separate user workload configuration [Serial]", func() {
			var (
				separateUwmConf = filepath.Join(monitoringBaseDir, "separate-uwm-config.yaml")
			)
			g.By("delete uwm-config/cm-config and bound pvc at the end of a serial case")
			defer func() {
				PvcNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "-ojsonpath={.items[*].metadata.name}", "-l", "app.kubernetes.io/instance=user-workload", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				for _, pvc := range strings.Fields(PvcNames) {
					oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", pvc, "-n", "openshift-user-workload-monitoring").Execute()
				}
			}()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("this case should execute on cluster which have storage class")
			checkSc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc").Output()
			if checkSc == "{}" || !strings.Contains(checkSc, "default") {
				g.Skip("This case should execute on cluster which have default storage class!")
			}

			g.By("get master node names with label")
			NodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master", "-ojsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeNameList := strings.Fields(NodeNames)

			g.By("add labels to master nodes, and delete them at the end of case")
			for _, name := range nodeNameList {
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", name, "uwm-").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", name, "uwm=deploy").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("create the separate user workload configuration")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", separateUwmConf)

			g.By("check remoteWrite metrics")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=prometheus_remote_storage_shards'`, token, `"url":"http://localhost:1234/receive"`, 3*uwmLoadTime)

			g.By("check prometheus-user-workload pods are bound to PVCs, check cpu and memory")
			PodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-ojsonpath={.items[*].metadata.name}", "-l", "app.kubernetes.io/name=prometheus", "-n", "openshift-user-workload-monitoring").Output()
			PodNameList := strings.Fields(PodNames)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range PodNameList {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, "-ojsonpath={.spec.volumes[]}", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring("uwm-prometheus"))
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, `-ojsonpath={.spec.containers[?(@.name=="prometheus")].resources.requests}`, "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring(`"cpu":"200m","memory":"1Gi"`))
			}

			g.By("check thanos-ruler-user-workload pods are bound to PVCs, check cpu and memory")
			PodNames, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-ojsonpath={.items[*].metadata.name}", "-l", "app.kubernetes.io/name=thanos-ruler", "-n", "openshift-user-workload-monitoring").Output()
			PodNameList = strings.Fields(PodNames)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range PodNameList {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, "-ojsonpath={.spec.volumes[]}", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring("thanosruler"))
				output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, `-ojsonpath={.spec.containers[?(@.name=="thanos-ruler")].resources.requests}`, "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring(`"cpu":"20m","memory":"50Mi"`))
			}

			g.By("toleration settings check")
			PodNames, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-ojsonpath={.items[*].metadata.name}", "-n", "openshift-user-workload-monitoring").Output()
			PodNameList = strings.Fields(PodNames)
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range PodNameList {
				output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, "-ojsonpath={.spec.tolerations}", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring("node-role.kubernetes.io/master"))
				o.Expect(output).To(o.ContainSubstring(`"operator":"Exists"`))
			}
			g.By("prometheus.enforcedSampleLimit check")
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheus", "user-workload", "-ojsonpath={.spec.enforcedSampleLimit}", "-n", "openshift-user-workload-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("2"))

			g.By("prometheus.retention check")
			output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("prometheus", "user-workload", "-ojsonpath={.spec.retention}", "-n", "openshift-user-workload-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("48h"))
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-50954-Allow the deployment of a dedicated UWM Alertmanager [Serial]", func() {
			var (
				dedicatedUWMalertmanager = filepath.Join(monitoringBaseDir, "dedicated-uwm-alertmanager.yaml")
				exampleAlert             = filepath.Join(monitoringBaseDir, "example-alert-rule.yaml")
				AlertmanagerConfig       = filepath.Join(monitoringBaseDir, "exampleAlertConfigAndSecret.yaml")
			)
			g.By("delete uwm-config/cm-config and bound pvc at the end of a serial case")
			defer func() {
				PvcNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", "-ojsonpath={.items[*].metadata.name}", "-l", "alertmanager=user-workload", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				for _, pvc := range strings.Fields(PvcNames) {
					oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", pvc, "-n", "openshift-user-workload-monitoring").Execute()
				}
			}()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("this case should execute on cluster which have storage class")
			checkSc, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc").Output()
			if checkSc == "{}" || !strings.Contains(checkSc, "default") {
				g.Skip("This case should execute on cluster which have default storage class!")
			}

			g.By("get master node names with label")
			NodeNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("node", "-l", "node-role.kubernetes.io/master", "-ojsonpath={.items[*].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			nodeNameList := strings.Fields(NodeNames)

			g.By("add labels to master nodes, and delete them at the end of case")
			for _, name := range nodeNameList {
				defer oc.AsAdmin().WithoutNamespace().Run("label").Args("node", name, "uwm-").Execute()
				err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", name, "uwm=alertmanager").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("create the dedicated UWM Alertmanager configuration")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", dedicatedUWMalertmanager)

			g.By("deploy prometheusrule and alertmanagerconfig to user project")
			oc.SetupProject()
			ns := oc.Namespace()
			createResourceFromYaml(oc, ns, exampleAlert)
			createResourceFromYaml(oc, ns, AlertmanagerConfig)

			g.By("check all pods are created")
			exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")

			g.By("check the alerts could be found in alertmanager under openshift-user-workload-monitoring project")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
			checkMetric(oc, `https://alertmanager-user-workload.openshift-user-workload-monitoring.svc:9095/api/v2/alerts`, token, "TestAlert1", 3*uwmLoadTime)

			g.By("check the alerts could not be found in openshift-monitoring project")
			//same as: checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts?&filter={alertname="TestAlert1"}`, token, "[]", 3*uwmLoadTime)
			checkAlertNotExist(oc, "https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts", token, "TestAlert1", 3*uwmLoadTime)

			g.By("get alertmanager pod names")
			PodNames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-ojsonpath={.items[*].metadata.name}", "-l", "app.kubernetes.io/name=alertmanager", "-n", "openshift-user-workload-monitoring").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check alertmanager pod resources limits and requests")
			for _, pod := range strings.Fields(PodNames) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, `-ojsonpath={.spec.containers[?(@.name=="alertmanager")].resources.limits}`, "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring(`"cpu":"100m","memory":"250Mi"`))
				o.Expect(err).NotTo(o.HaveOccurred())
				output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, `-ojsonpath={.spec.containers[?(@.name=="alertmanager")].resources.requests}`, "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring(`"cpu":"40m","memory":"200Mi"`))
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("check alertmanager pod are bound pvcs")
			for _, pod := range strings.Fields(PodNames) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", pod, "-ojsonpath={.spec.volumes[]}", "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(output).To(o.ContainSubstring("uwm-alertmanager"))
				o.Expect(err).NotTo(o.HaveOccurred())
			}

			g.By("check AlertmanagerConfigs are take effect")
			for _, pod := range strings.Fields(PodNames) {
				checkAlertmangerConfig(oc, "openshift-user-workload-monitoring", pod, "api_url: http://wechatserver:8080/", true)
			}

			g.By("check logLevel is correctly set")
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("alertmanager/user-workload", "-ojsonpath={.spec.logLevel}", "-n", "openshift-user-workload-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("debug"))
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check logLevel is take effect")
			for _, pod := range strings.Fields(PodNames) {
				output, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args("-c", "alertmanager", pod, "-n", "openshift-user-workload-monitoring").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(output, "level=debug") {
					e2e.Failf("logLevel is wrong or not take effect")
				}
			}

			g.By("disable alertmanager in user-workload-monitoring-config")
			//oc patch cm user-workload-monitoring-config -p '{"data": {"config.yaml": "alertmanager:\n  enabled: false\n"}}' --type=merge -n openshift-user-workload-monitoring
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "user-workload-monitoring-config", "-p", `{"data": {"config.yaml": "alertmanager:\n  enabled: false\n"}}`, "--type=merge", "-n", "openshift-user-workload-monitoring").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("should found user project alerts in platform alertmanager")
			checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v2/alerts`, token, "TestAlert1", 3*uwmLoadTime)

			g.By("UWM alertmanager pod should disappear") //need time to wait pod fully terminated, put this step after the checkMetric
			checkPodDeleted(oc, "openshift-user-workload-monitoring", "app.kubernetes.io/name=alertmanager", "alertmanager")
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-43286-Allow sending alerts to external Alertmanager for user workload monitoring components - enabled in-cluster alertmanager", func() {
			var (
				testAlertmanager = filepath.Join(monitoringBaseDir, "example-alertmanager.yaml")
				exampleAlert     = filepath.Join(monitoringBaseDir, "example-alert-rule.yaml")
				exampleAlert2    = filepath.Join(monitoringBaseDir, "leaf-prometheus-rule.yaml")
			)
			g.By("create alertmanager and set external alertmanager for prometheus/thanosRuler under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", testAlertmanager)

			g.By("check alertmanager pod is created")
			exutil.AssertPodToBeReady(oc, "alertmanager-test-alertmanager-0", "openshift-user-workload-monitoring")

			g.By("create example PrometheusRule under user namespace")
			oc.SetupProject()
			ns1 := oc.Namespace()
			createResourceFromYaml(oc, ns1, exampleAlert)

			g.By("create another user namespace then create PrometheusRule with leaf-prometheus label")
			oc.SetupProject()
			ns2 := oc.Namespace()
			createResourceFromYaml(oc, ns2, exampleAlert2)

			g.By("Get token of SA prometheus-k8s")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

			g.By("check the user alerts TestAlert1 and TestAlert2 are shown in \"in-cluster alertmanager\" API")
			checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts?filter={alertname="TestAlert1"}`, token, "TestAlert1", 3*uwmLoadTime)
			checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts?filter={alertname="TestAlert1"}`, token, `"generatorURL":"https://thanos-querier-openshift-monitoring`, 3*uwmLoadTime)
			checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts?filter={alertname="TestAlert2"}`, token, "TestAlert2", 3*uwmLoadTime)
			checkMetric(oc, `https://alertmanager-main.openshift-monitoring.svc:9094/api/v1/alerts?filter={alertname="TestAlert2"}`, token, `"generatorURL":"http://prometheus-user-workload-`, 3*uwmLoadTime)

			g.By("check the alerts are also sent to external alertmanager")
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="TestAlert1"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", "TestAlert1", 3*uwmLoadTime)
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="TestAlert1"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", `"generatorURL":"https://thanos-querier-openshift-monitoring`, 3*uwmLoadTime)
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="TestAlert2"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", "TestAlert2", 3*uwmLoadTime)
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="TestAlert2"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", `"generatorURL":"http://prometheus-user-workload-`, 3*uwmLoadTime)
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-43311-Allow sending alerts to external Alertmanager for user workload monitoring components - disabled in-cluster alertmanager [Serial]", func() {
			var (
				InClusterMonitoringCM = filepath.Join(monitoringBaseDir, "disLocalAlert-setExternalAlert-prometheus.yaml")
				testAlertmanager      = filepath.Join(monitoringBaseDir, "example-alertmanager.yaml")
				exampleAlert          = filepath.Join(monitoringBaseDir, "example-alert-rule.yaml")
			)
			g.By("Restore cluster monitoring stack default configuration")
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("alertmanager", "test-alertmanager", "-n", "openshift-user-workload-monitoring", "--ignore-not-found").Execute()
			defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
			defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

			g.By("disable local alertmanager and set external manager for prometheus")
			createResourceFromYaml(oc, "openshift-monitoring", InClusterMonitoringCM)

			g.By("create alertmanager and set external alertmanager for prometheus/thanosRuler under openshift-user-workload-monitoring")
			createResourceFromYaml(oc, "openshift-user-workload-monitoring", testAlertmanager)

			g.By("check alertmanager pod is created")
			exutil.AssertPodToBeReady(oc, "alertmanager-test-alertmanager-0", "openshift-user-workload-monitoring")

			g.By("create example PrometheusRule under user namespace")
			oc.SetupProject()
			ns1 := oc.Namespace()
			createResourceFromYaml(oc, ns1, exampleAlert)

			g.By("Get token of SA prometheus-k8s")
			token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

			g.By("check the user alerts TestAlert1 and in-cluster Watchdog alerts are shown in \"thanos-querier\" API")
			checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="TestAlert1"}'`, token, `TestAlert1`, 3*platformLoadTime)
			checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="Watchdog"}'`, token, `Watchdog`, 3*platformLoadTime)

			g.By("check the alerts are also sent to external alertmanager, include the in-cluster and user project alerts")
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="TestAlert1"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", "TestAlert1", 3*uwmLoadTime)
			queryFromPod(oc, `http://alertmanager-operated.openshift-user-workload-monitoring.svc:9093/api/v1/alerts?filter={alertname="Watchdog"}`, token, "openshift-user-workload-monitoring", "thanos-ruler-user-workload-0", "thanos-ruler", "Watchdog", 3*uwmLoadTime)
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-ConnectedOnly-Medium-44815-Configure containers to honor the global tlsSecurityProfile", func() {
			g.By("get global tlsSecurityProfile")
			// % oc get kubeapiservers.operator.openshift.io cluster -o jsonpath='{.spec.observedConfig.servingInfo.cipherSuites}'
			cipherSuites, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiservers.operator.openshift.io", "cluster", "-ojsonpath={.spec.observedConfig.servingInfo.cipherSuites}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			cipherSuitesFormat := strings.ReplaceAll(cipherSuites, "\"", "")
			cipherSuitesFormat = strings.ReplaceAll(cipherSuitesFormat, "[", "")
			cipherSuitesFormat = strings.ReplaceAll(cipherSuitesFormat, "]", "")
			e2e.Logf("cipherSuites: %s", cipherSuitesFormat)
			// % oc get kubeapiservers.operator.openshift.io cluster -o jsonpath='{.spec.observedConfig.servingInfo.minTLSVersion}'
			minTLSVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("kubeapiservers.operator.openshift.io", "cluster", "-ojsonpath={.spec.observedConfig.servingInfo.minTLSVersion}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check tls-cipher-suites and tls-min-version for prometheus-adapter under openshift-monitoring")
			// % oc -n openshift-monitoring get deploy prometheus-adapter -ojsonpath='{.spec.template.spec.containers[?(@tls-cipher-suites=)].args}'
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploy", "prometheus-adapter", "-ojsonpath={.spec.template.spec.containers[?(@tls-cipher-suites=)].args}", "-n", "openshift-monitoring").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, cipherSuitesFormat) {
				e2e.Failf("tls-cipher-suites is different from global setting! %s", output)
			}
			if !strings.Contains(output, minTLSVersion) {
				e2e.Failf("tls-min-version is different from global setting! %s", output)
			}

			g.By("check tls-cipher-suites and tls-min-version for all pods which use kube-rbac-proxy container under openshift-monitoring/openshift-user-workload-monitoring")
			//oc get pod -l app.kubernetes.io/name=alertmanager -n openshift-monitoring
			alertmanagerPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=alertmanager")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range alertmanagerPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-metric\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=node-exporter -n openshift-monitoring
			nePodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=node-exporter")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range nePodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=kube-state-metrics -n openshift-monitoring
			ksmPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=kube-state-metrics")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range ksmPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-main\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-self\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=openshift-state-metrics -n openshift-monitoring
			osmPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=openshift-state-metrics")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range osmPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-main\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-self\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=prometheus -n openshift-monitoring
			pk8sPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range pk8sPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-thanos\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=prometheus-operator -n openshift-monitoring
			poPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=prometheus-operator")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range poPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=telemeter-client -n openshift-monitoring
			tcPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=telemeter-client")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range tcPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=thanos-query -n openshift-monitoring
			tqPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/name=thanos-query")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range tqPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-rules\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
				cmd = "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy-metrics\")].args}"
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/name=prometheus-operator -n openshift-user-workload-monitoring
			UWMpoPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-user-workload-monitoring", "app.kubernetes.io/name=prometheus-operator")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range UWMpoPodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"kube-rbac-proxy\")].args}"
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
			//oc get pod -l app.kubernetes.io/instance=user-workload -n openshift-user-workload-monitoring
			UWMPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-user-workload-monitoring", "app.kubernetes.io/instance=user-workload")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range UWMPodNames {
				// Multiple container: kube-rbac-**** under this label, use fuzzy query
				cmd := "-ojsonpath={.spec.containers[?(@tls-cipher-suites)].args}"
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, cipherSuitesFormat, true)
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, minTLSVersion, true)
			}
		})

		// author: tagao@redhat.com
		g.It("Author:tagao-Medium-68237-Add the trusted CA bundle in the Prometheus user workload monitoring pods", func() {
			g.By("confirm UWM pod is ready")
			exutil.AssertPodToBeReady(oc, "prometheus-user-workload-0", "openshift-user-workload-monitoring")

			g.By("check configmap under namespace: openshift-user-workload-monitoring")
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-user-workload-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("prometheus-user-workload-trusted-ca-bundle"))
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("check the trusted CA bundle is applied to the pod")
			PodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-user-workload-monitoring", "app.kubernetes.io/name=prometheus")
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, pod := range PodNames {
				cmd := "-ojsonpath={.spec.containers[?(@.name==\"prometheus\")].volumeMounts}"
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, "prometheus-user-workload-trusted-ca-bundle", true)
				cmd = "-ojsonpath={.spec.volumes[?(@.name==\"prometheus-user-workload-trusted-ca-bundle\")]}"
				checkYamlconfig(oc, "openshift-user-workload-monitoring", "pod", pod, cmd, "prometheus-user-workload-trusted-ca-bundle", true)
			}
		})
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Low-30088-User can not deploy ThanosRuler CRs in user namespaces [Serial]", func() {
		var (
			ns                string
			output            string
			deployThanosRuler = filepath.Join(monitoringBaseDir, "deployThanosRuler.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("deploy ThanosRuler under namespace as a common user (non-admin)")
		oc.SetupProject()
		ns = oc.Namespace()
		oc.Run("apply").Args("-n", ns, "-f", deployThanosRuler).Execute()

		g.By("check ThanosRuler is not created")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("ThanosRuler", "user-workload", "-n", ns).Output()
		o.Expect(output).To(o.ContainSubstring("thanosrulers.monitoring.coreos.com \"user-workload\" not found"))

	})

	//author: tagao@redhat.com
	g.It("Author:tagao-NonPreRelease-Longduration-Medium-49191-Enforce body_size_limit [Serial]", func() {
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("set `enforcedBodySizeLimit` to 0, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "0", "0")

		g.By("set `enforcedBodySizeLimit` to a invalid value, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "20MiBPS", "")

		g.By("set `enforcedBodySizeLimit` to 1MB to trigger PrometheusScrapeBodySizeLimitHit alert, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "1MB", "1MB")

		g.By("check PrometheusScrapeBodySizeLimitHit alert is triggered")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, "PrometheusScrapeBodySizeLimitHit", 5*uwmLoadTime)

		g.By("set `enforcedBodySizeLimit` to 40MB, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "40MB", "40MB")

		g.By("check from alert, should not have enforcedBodySizeLimit")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, `"result":[]`, 5*uwmLoadTime)

		g.By("set `enforcedBodySizeLimit` to automatic, and check from the k8s pod")
		patchAndCheckBodySizeLimit(oc, "automatic", "body_size_limit")

		g.By("check from alert, should not have enforcedBodySizeLimit")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertname="PrometheusScrapeBodySizeLimitHit"}'`, token, `"result":[]`, 5*uwmLoadTime)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-60485-check On/Off switch of netdev Collector in Node Exporter [Serial]", func() {
		var (
			disableNetdev = filepath.Join(monitoringBaseDir, "disableNetdev.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check netdev Collector is enabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netdev"))

		g.By("check netdev metrics in prometheus k8s pod")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netdev"}'`, token, `"collector":"netdev"`, uwmLoadTime)

		g.By("disable netdev in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", disableNetdev)

		g.By("check netdev metrics in prometheus k8s pod again, should not have related metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netdev"}'`, token, `"result":[]`, 3*uwmLoadTime)

		g.By("check netdev in daemonset")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--no-collector.netdev"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-59521-check On/Off switch of cpufreq Collector in Node Exporter [Serial]", func() {
		var (
			enableCpufreq = filepath.Join(monitoringBaseDir, "enableCpufreq.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check cpufreq Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.cpufreq"))

		g.By("check cpufreq metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="cpufreq"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable cpufreq in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableCpufreq)

		g.By("check cpufreq metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="cpufreq"}'`, token, `"collector":"cpufreq"`, 3*uwmLoadTime)

		g.By("check cpufreq in daemonset")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--collector.cpufreq"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-60480-check On/Off switch of tcpstat Collector in Node Exporter [Serial]", func() {
		var (
			enableTcpstat = filepath.Join(monitoringBaseDir, "enableTcpstat.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check tcpstat Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.tcpstat"))

		g.By("check tcpstat metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="tcpstat"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable tcpstat in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableTcpstat)

		g.By("check tcpstat metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="tcpstat"}'`, token, `"collector":"tcpstat"`, 3*uwmLoadTime)

		g.By("check tcpstat in daemonset")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--collector.tcpstat"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-60582-check On/Off switch of buddyinfo Collector in Node Exporter [Serial]", func() {
		var (
			enableBuddyinfo = filepath.Join(monitoringBaseDir, "enableBuddyinfo.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check buddyinfo Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.buddyinfo"))

		g.By("check buddyinfo metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="buddyinfo"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable buddyinfo in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableBuddyinfo)

		g.By("check buddyinfo metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="buddyinfo"}'`, token, `"collector":"buddyinfo"`, 3*uwmLoadTime)

		g.By("check buddyinfo in daemonset")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--collector.buddyinfo"))
	})

	//author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-59986-Allow to configure secrets in alertmanager component [Serial]", func() {
		var (
			alertmanagerSecret      = filepath.Join(monitoringBaseDir, "alertmanager-secret.yaml")
			alertmanagerSecretCM    = filepath.Join(monitoringBaseDir, "alertmanager-secret-cm.yaml")
			alertmanagerSecretUwmCM = filepath.Join(monitoringBaseDir, "alertmanager-secret-uwm-cm.yaml")
		)
		g.By("delete secrets/user-workload-monitoring-config/cluster-monitoring-config configmap at the end of a serial case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "test-secret", "-n", "openshift-monitoring").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "slack-api-token", "-n", "openshift-monitoring").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "test-secret", "-n", "openshift-user-workload-monitoring").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("secret", "slack-api-token", "-n", "openshift-user-workload-monitoring").Execute()
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("create alertmanager secret in openshift-monitoring")
		createResourceFromYaml(oc, "openshift-monitoring", alertmanagerSecret)

		g.By("enabled UWM and configure alertmanager secret setting in cluster-monitoring-config configmap")
		createResourceFromYaml(oc, "openshift-monitoring", alertmanagerSecretCM)

		g.By("check if the secrets are mounted to alertmanager pod")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		checkConfigInPod(oc, "openshift-monitoring", "alertmanager-main-0", "alertmanager", "ls /etc/alertmanager/secrets/", "test-secret")
		checkConfigInPod(oc, "openshift-monitoring", "alertmanager-main-0", "alertmanager", "ls /etc/alertmanager/secrets/", "slack-api-token")

		g.By("create the same alertmanager secret in openshift-user-workload-monitoring")
		createResourceFromYaml(oc, "openshift-user-workload-monitoring", alertmanagerSecret)

		g.By("configure alertmanager secret setting in user-workload-monitoring-config configmap")
		createResourceFromYaml(oc, "openshift-user-workload-monitoring", alertmanagerSecretUwmCM)

		g.By("check if the sercrets are mounted to UWM alertmanager pod")
		exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")
		checkConfigInPod(oc, "openshift-user-workload-monitoring", "alertmanager-user-workload-0", "alertmanager", "ls /etc/alertmanager/secrets/", "test-secret")
		checkConfigInPod(oc, "openshift-user-workload-monitoring", "alertmanager-user-workload-0", "alertmanager", "ls /etc/alertmanager/secrets/", "slack-api-token")
	})

	//author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-60532-TechPreview feature is not enabled and collectionProfile is set to valid value [Serial]", func() {
		var (
			collectionProfileminimal = filepath.Join(monitoringBaseDir, "collectionProfile_minimal.yaml")
		)
		g.By("delete user-workload-monitoring-config/cluster-monitoring-config configmap at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("skip the case in TechPreview feature enabled cluster")
		featureSet, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("FeatureGate/cluster", "-ojsonpath={.spec}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("featureSet is: %s", featureSet)
		if featureSet != "{}" && strings.Contains(featureSet, "TechPreviewNoUpgrade") {
			g.Skip("This case is not suitable for TechPreview enabled cluster!")
		}

		g.By("set collectionProfile to minimal in cluster-monitoring-config configmap")
		createResourceFromYaml(oc, "openshift-monitoring", collectionProfileminimal)

		g.By("should see error in CMO logs which indicate collectionProfiles is a TechPreview feature")
		CMOPodName, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-monitoring", "-l", "app.kubernetes.io/name=cluster-monitoring-operator", "-ojsonpath={.items[].metadata.name}").Output()
		checkLogsInContainer(oc, "openshift-monitoring", CMOPodName, "cluster-monitoring-operator", "collectionProfiles is a TechPreview feature")
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Low-60534-check gomaxprocs setting of Node Exporter in CMO [Serial]", func() {
		var (
			setGomaxprocsTo1 = filepath.Join(monitoringBaseDir, "setGomaxprocsTo1.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check default gomaxprocs value is 0")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--runtime.gomaxprocs=0"))

		g.By("set gomaxprocs value to 1")
		createResourceFromYaml(oc, "openshift-monitoring", setGomaxprocsTo1)

		g.By("check gomaxprocs value in daemonset")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output2).To(o.ContainSubstring("--runtime.gomaxprocs=1"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-60486-check On/Off switch of netclass Collector and netlink backend in Node Exporter [Serial]", func() {
		var (
			disableNetclass = filepath.Join(monitoringBaseDir, "disableNetclass.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check netclass Collector is enabled by default, so as netlink")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		//oc -n openshift-monitoring get daemonset.apps/node-exporter -ojsonpath='{.spec.template.spec.containers[?(@.name=="node-exporter")].args}'
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netclass"))
		o.Expect(output).To(o.ContainSubstring("--collector.netclass.netlink"))

		g.By("check netclass metrics in prometheus k8s pod")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netclass"}'`, token, `"collector":"netclass"`, uwmLoadTime)

		g.By("disable netclass in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", disableNetclass)

		g.By("check netclass metrics in prometheus k8s pod again, should not have related metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="netclass"}'`, token, `"result":[]`, 3*uwmLoadTime)

		g.By("check netclass/netlink in daemonset")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.netclass"))
		o.Expect(output).NotTo(o.ContainSubstring("--collector.netclass.netlink"))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-63659-check On/Off switch of ksmd Collector in Node Exporter [Serial]", func() {
		var (
			enableKsmd = filepath.Join(monitoringBaseDir, "enableKsmd.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check ksmd Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.ksmd"))

		g.By("check ksmd metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="ksmd"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable ksmd in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableKsmd)

		g.By("check ksmd metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="ksmd"}'`, token, `"collector":"ksmd"`, 3*uwmLoadTime)

		g.By("check ksmd in daemonset")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.ksmd"))
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-64537-CMO deploys monitoring console-plugin [Serial]", func() {
		var (
			monitoringPluginConfig = filepath.Join(monitoringBaseDir, "monitoringPlugin-config.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("skip the case if console CO is absent")
		checkCO, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(checkCO, "console") {
			g.Skip("This case is not executable when console CO is absent")
		}

		g.By("apply monitoringPlugin config and wait for all pods ready")
		createResourceFromYaml(oc, "openshift-monitoring", monitoringPluginConfig)
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

		g.By("check monitoring-plugin ConfigMap/ConsolePlugin/PodDisruptionBudget/ServiceAccount/Service/deployment are exist")
		resourceNames := []string{"ConfigMap", "ConsolePlugin", "PodDisruptionBudget", "ServiceAccount", "Service", "deployment"}
		for _, resource := range resourceNames {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "monitoring-plugin", "-n", "openshift-monitoring").Output()
			o.Expect(output).To(o.ContainSubstring("monitoring-plugin"))
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		// this step is aim to give time to monitoring-plugin pods loading
		g.By("check monitoring-plugin container usage")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=sum(rate(container_cpu_usage_seconds_total{container="monitoring-plugin",namespace="openshift-monitoring"}[5m]))'`, token, `"value"`, uwmLoadTime)

		g.By("check monitoring-plugin pod config")
		monitoringPluginPodNames, err := exutil.GetAllPodsWithLabel(oc, "openshift-monitoring", "app.kubernetes.io/component=monitoring-plugin")
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, pod := range monitoringPluginPodNames {
			e2e.Logf("If the pod is not ready or not found, it means the pod:{%s} was restarted during the test, pod name changed", pod)
			exutil.AssertPodToBeReady(oc, pod, "openshift-monitoring")
			cmd := "-ojsonpath={.spec.nodeSelector}"
			checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, `{"node-role.kubernetes.io/worker":""}`, true)
			cmd = "-ojsonpath={.spec.topologySpreadConstraints}"
			checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, `{"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"DoNotSchedule"}`, true)
			cmd = "-ojsonpath={.spec.tolerations}"
			checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, `{"operator":"Exists"}`, true)
			cmd = "-ojsonpath={.spec.containers[].resources}"
			checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, `"requests":{"cpu":"15m","memory":"60Mi"}`, true)
			checkYamlconfig(oc, "openshift-monitoring", "pod", pod, cmd, `"limits":{"cpu":"30m","memory":"120Mi"}`, true)
		}
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-63657-check On/Off switch of systemd Collector in Node Exporter [Serial]", func() {
		var (
			enableSystemdUnits = filepath.Join(monitoringBaseDir, "enableSystemdUnits.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check systemd Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.systemd"))

		g.By("check systemd metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="systemd"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable systemd and units in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableSystemdUnits)

		g.By("check systemd related metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="systemd"}'`, token, `"collector":"systemd"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_systemd_system_running'`, token, `"node_systemd_system_running"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_systemd_timer_last_trigger_seconds'`, token, `"node_systemd_timer_last_trigger_seconds"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_systemd_units'`, token, `"node_systemd_units"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_systemd_version'`, token, `"node_systemd_version"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_systemd_unit_state'`, token, `"node_systemd_unit_state"`, 3*uwmLoadTime)

		g.By("check systemd in daemonset")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.systemd"))
		o.Expect(output).To(o.ContainSubstring("--collector.systemd.unit-include=^(network.+|nss.+|logrotate.timer)$"))
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-High-63658-check On/Off switch of mountstats Collector in Node Exporter [Serial]", func() {
		var (
			enableMountstats    = filepath.Join(monitoringBaseDir, "enableMountstats.yaml")
			enableMountstatsNFS = filepath.Join(monitoringBaseDir, "enableMountstats_nfs.yaml")
		)
		g.By("delete uwm-config/cm-config and pvcs at the end of the case")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "-l", "app.kubernetes.io/name=prometheus", "-n", "openshift-monitoring").Execute()
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check mountstats collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.mountstats"))

		g.By("check mountstats metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="mountstats"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable mountstats in CMO")
		createResourceFromYaml(oc, "openshift-monitoring", enableMountstats)

		g.By("check mountstats metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="mountstats"}'`, token, `"collector":"mountstats"`, 3*uwmLoadTime)

		g.By("check mountstats in daemonset")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.mountstats"))

		g.By("check nfs metrics if need")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("sc").Output()
		if strings.Contains(output, "nfs") {
			createResourceFromYaml(oc, "openshift-monitoring", enableMountstatsNFS)
			exutil.AssertPodToBeReady(oc, "prometheus-k8s-0", "openshift-monitoring")
			checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_mountstats_nfs_read_bytes_total'`, token, `"__name__":"node_mountstats_nfs_read_bytes_total"`, 3*uwmLoadTime)
			checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_mountstats_nfs_write_bytes_total'`, token, `"__name__":"node_mountstats_nfs_write_bytes_total"`, 3*uwmLoadTime)
			checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_mountstats_nfs_operations_requests_total'`, token, `"__name__":"node_mountstats_nfs_operations_requests_total"`, 3*uwmLoadTime)
		} else {
			e2e.Logf("no need to check nfs metrics for this env")
		}
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-64868-netclass/netdev device configuration [Serial]", func() {
		var (
			ignoredNetworkDevices = filepath.Join(monitoringBaseDir, "ignoredNetworkDevices-lo.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check netclass/netdev device configuration")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netclass.ignored-devices=^(veth.*|[a-f0-9]{15}|enP.*|ovn-k8s-mp[0-9]*|br-ex|br-int|br-ext|br[0-9]*|tun[0-9]*|cali[a-f0-9]*)$"))
		o.Expect(output).To(o.ContainSubstring("--collector.netdev.device-exclude=^(veth.*|[a-f0-9]{15}|enP.*|ovn-k8s-mp[0-9]*|br-ex|br-int|br-ext|br[0-9]*|tun[0-9]*|cali[a-f0-9]*)$"))

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check lo devices exist, and able to see related metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=group by(device) (node_network_info)'`, token, `"device":"lo"`, uwmLoadTime)

		g.By("modify cm to ignore lo devices")
		createResourceFromYaml(oc, "openshift-monitoring", ignoredNetworkDevices)
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

		g.By("check metrics again, should not see lo device metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_network_info{device="lo"}'`, token, `"result":[]`, 3*uwmLoadTime)

		g.By("check netclass/netdev device configuration, no lo devices")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netclass.ignored-devices=^(lo)$"))
		o.Expect(output).To(o.ContainSubstring("--collector.netdev.device-exclude=^(lo)$"))

		g.By("modify cm to ignore all devices")
		// % oc -n openshift-monitoring patch cm cluster-monitoring-config -p '{"data": {"config.yaml": "nodeExporter:\n ignoredNetworkDevices: [.*]"}}' --type=merge
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cm", "cluster-monitoring-config", "-p", `{"data": {"config.yaml": "nodeExporter:\n ignoredNetworkDevices: [.*]"}}`, "--type=merge", "-n", "openshift-monitoring").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check metrics again, should not see all device metrics")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=group by(device) (node_network_info)'`, token, `"result":[]`, 3*uwmLoadTime)

		g.By("check netclass/netdev device configuration again")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.netclass.ignored-devices=^(.*)$"))
		o.Expect(output).To(o.ContainSubstring("--collector.netdev.device-exclude=^(.*)$"))
	})

	// author: tagao@redhat.com
	g.It("Author:tagao-Medium-64296-disable CORS headers on Thanos querier [Serial]", func() {
		var (
			enableCORS = filepath.Join(monitoringBaseDir, "enableCORS.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check the default enableCORS value is false")
		// oc -n openshift-monitoring get deployments.apps thanos-querier -o jsonpath='{.spec.template.spec.containers[?(@.name=="thanos-query")].args}' |jq
		thanosQueryArgs, getArgsErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployments/thanos-querier", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"thanos-query\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(getArgsErr).NotTo(o.HaveOccurred(), "Failed to get thanos-query container args definition")
		o.Expect(thanosQueryArgs).To(o.ContainSubstring("--web.disable-cors"))

		g.By("set enableCORS as true")
		createResourceFromYaml(oc, "openshift-monitoring", enableCORS)
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

		g.By("check the config again")
		cmd := "-ojsonpath={.spec.template.spec.containers[?(@.name==\"thanos-query\")].args}"
		checkYamlconfig(oc, "openshift-monitoring", "deployments", "thanos-querier", cmd, `--web.disable-cors`, false)
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-Medium-43106-disable Alertmanager deployment[Serial]", func() {
		var (
			disableAlertmanager = filepath.Join(monitoringBaseDir, "disableAlertmanager.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("disable alertmanager in CMO config")
		createResourceFromYaml(oc, "openshift-monitoring", disableAlertmanager)
		exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")

		// this step is aim to give time let CMO removing alertmanager resources
		g.By("confirm alertmanager is down")
		checkPodDeleted(oc, "openshift-monitoring", "alertmanager=main", "alertmanager")

		g.By("check alertmanager resources are removed")
		resourceNames := []string{"route", "servicemonitor", "serviceaccounts", "statefulset", "services", "endpoints", "alertmanagers", "prometheusrules", "clusterrolebindings", "clusterroles", "roles"}
		for _, resource := range resourceNames {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(resource, "-n", "openshift-monitoring").Output()
			o.Expect(strings.Contains(output, "alertmanager")).NotTo(o.BeTrue())
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("check on configmaps")
		checkCM, _ := exec.Command("bash", "-c", `oc -n openshift-monitoring get cm -l app.kubernetes.io/managed-by=cluster-monitoring-operator | grep alertmanager`).Output()
		e2e.Logf("check result is: %v", checkCM)
		o.Expect(checkCM).NotTo(o.ContainSubstring("alertmanager-trusted-ca-bundle"))
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("configmaps", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("alertmanager-trusted-ca-bundle-"))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check on rolebindings")
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("rolebindings", "-n", "openshift-monitoring").Output()
		o.Expect(output).NotTo(o.ContainSubstring("alertmanager-prometheusk8s"))
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get token of SA prometheus-k8s")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")

		g.By("check Watchdog alert exist")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=ALERTS{alertstate="firing",alertname="Watchdog"}'`, token, `"alertname":"Watchdog"`, uwmLoadTime)
	})

	// author: juzhao@redhat.com
	g.It("Author:juzhao-Medium-66736-add option to specify resource requests and limits for components [Serial]", func() {
		var (
			clusterResources = filepath.Join(monitoringBaseDir, "cluster_resources.yaml")
			uwmResources     = filepath.Join(monitoringBaseDir, "uwm_resources.yaml")
		)
		g.By("delete user-workload-monitoring-config/cluster-monitoring-config configmap at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		createResourceFromYaml(oc, "openshift-monitoring", clusterResources)
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")

		g.By("by default there is not resources.limits setting for the components, check the result for kube_pod_container_resource_limits of node-exporter pod to see if the setting loaded to components, same for other components")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="node-exporter",namespace="openshift-monitoring"}'`, token, `"pod":"node-exporter-`, 3*uwmLoadTime)

		g.By("check the resources.requests and resources.limits setting loaded to node-exporter daemonset")
		// oc -n openshift-monitoring get daemonset node-exporter -o jsonpath='{.spec.template.spec.containers[?(@.name=="node-exporter")].resources.requests}'
		result, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get node-exporter container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"40Mi"`))

		// oc -n openshift-monitoring get daemonset node-exporter -o jsonpath='{.spec.template.spec.containers[?(@.name=="node-exporter")].resources.limits}'
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get node-exporter container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"20m","memory":"100Mi"`))

		g.By("check the resources.requests and resources.limits take effect for kube-state-metrics")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="kube-state-metrics",namespace="openshift-monitoring"}'`, token, `"pod":"kube-state-metrics-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/kube-state-metrics", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"kube-state-metrics\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get kube-state-metrics container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"3m","memory":"100Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/kube-state-metrics", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"kube-state-metrics\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get kube-state-metrics container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"200Mi"`))

		g.By("check the resources.requests and resources.limits take effect for openshift-state-metrics")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="openshift-state-metrics",namespace="openshift-monitoring"}'`, token, `"pod":"openshift-state-metrics-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/openshift-state-metrics", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"openshift-state-metrics\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get openshift-state-metrics container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"2m","memory":"40Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/openshift-state-metrics", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"openshift-state-metrics\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get openshift-state-metrics container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"20m","memory":"100Mi"`))

		g.By("check the resources.requests and resources.limits take effect for prometheus-adapter")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="prometheus-adapter",namespace="openshift-monitoring"}'`, token, `"pod":"prometheus-adapter-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-adapter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-adapter\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-adapter container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"2m","memory":"80Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-adapter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-adapter\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-adapter container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"100Mi"`))

		g.By("check the resources.requests and resources.limits take effect for prometheus-operator")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="prometheus-operator",namespace="openshift-monitoring"}'`, token, `"pod":"prometheus-operator-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-operator container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"200Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-operator container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"20m","memory":"300Mi"`))

		g.By("check the resources.requests and resources.limits take effect for prometheus-operator-admission-webhook")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="prometheus-operator-admission-webhook",namespace="openshift-monitoring"}'`, token, `"pod":"prometheus-operator-admission-webhook-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator-admission-webhook", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator-admission-webhook\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-operator-admission-webhook container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"50Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator-admission-webhook", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator-admission-webhook\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get prometheus-operator-admission-webhook container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"20m","memory":"100Mi"`))

		g.By("check the resources.requests and resources.limits take effect for telemeter-client")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="telemeter-client",namespace="openshift-monitoring"}'`, token, `"pod":"telemeter-client-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/telemeter-client", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"telemeter-client\")].resources.requests}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get telemeter-client container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"2m","memory":"50Mi"`))

		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/telemeter-client", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"telemeter-client\")].resources.limits}", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get telemeter-client container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"100Mi"`))

		createResourceFromYaml(oc, "openshift-user-workload-monitoring", uwmResources)
		exutil.AssertAllPodsToBeReady(oc, "openshift-user-workload-monitoring")

		g.By("check the resources.requests and resources.limits for uwm prometheus-operator")
		checkMetric(oc, `https://thanos-querier.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=kube_pod_container_resource_limits{container="prometheus-operator",namespace="openshift-user-workload-monitoring"}'`, token, `"pod":"prometheus-operator-`, 3*uwmLoadTime)
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator\")].resources.requests}", "-n", "openshift-user-workload-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get UWM prometheus-operator container resources.requests setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"2m","memory":"20Mi"`))
		result, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment/prometheus-operator", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"prometheus-operator\")].resources.limits}", "-n", "openshift-user-workload-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred(), "Failed to get UWM prometheus-operator container resources.limits setting")
		o.Expect(result).To(o.ContainSubstring(`"cpu":"10m","memory":"100Mi"`))
	})

	//author: tagao@redhat.com
	g.It("Author:tagao-High-67503-check On/Off switch of processes Collector in Node Exporter [Serial]", func() {
		var (
			enableProcesses = filepath.Join(monitoringBaseDir, "enableProcesses.yaml")
		)
		g.By("delete uwm-config/cm-config at the end of a serial case")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)

		g.By("check processes Collector is disabled by default")
		exutil.AssertAllPodsToBeReady(oc, "openshift-monitoring")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--no-collector.processes"))

		g.By("check processes metrics in prometheus k8s pod, should not have related metrics")
		token := getSAToken(oc, "prometheus-k8s", "openshift-monitoring")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="processes"}'`, token, `"result":[]`, uwmLoadTime)

		g.By("enable processes in CMO config")
		createResourceFromYaml(oc, "openshift-monitoring", enableProcesses)

		g.By("check processes metrics in prometheus k8s pod again")
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_scrape_collector_success{collector="processes"}'`, token, `"collector":"processes"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_processes_max_processes'`, token, `"__name__":"node_processes_max_processes"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_processes_pids'`, token, `"__name__":"node_processes_pids"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_processes_state'`, token, `"__name__":"node_processes_state"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_processes_threads'`, token, `"__name__":"node_processes_threads"`, 3*uwmLoadTime)
		checkMetric(oc, `https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query --data-urlencode 'query=node_processes_threads_state'`, token, `"__name__":"node_processes_threads_state"`, 3*uwmLoadTime)

		g.By("check processes in daemonset")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("get").Args("daemonset.apps/node-exporter", "-ojsonpath={.spec.template.spec.containers[?(@.name==\"node-exporter\")].args}", "-n", "openshift-monitoring").Output()
		o.Expect(output).To(o.ContainSubstring("--collector.processes"))
	})

	// author: hongyli@redhat.com
	g.It("Author:hongyli-Critical-44032-Restore cluster monitoring stack default configuration [Serial]", func() {
		defer deleteConfig(oc, monitoringCM.name, monitoringCM.namespace)
		g.By("Delete config map user-workload--monitoring-config")
		defer deleteConfig(oc, "user-workload-monitoring-config", "openshift-user-workload-monitoring")
		g.By("Delete config map cluster-monitoring-config")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("alertmanager", "test-alertmanager", "-n", "openshift-user-workload-monitoring", "--ignore-not-found").Execute()
		g.By("Delete alertmanager under openshift-user-workload-monitoring")
	})
})
