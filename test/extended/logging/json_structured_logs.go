package logging

import (
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-openshift-logging] Logging", func() {
	defer g.GinkgoRecover()

	var (
		oc             = exutil.NewCLI("logging-json-log", exutil.KubeConfigPath())
		eo             = "elasticsearch-operator"
		clo            = "cluster-logging-operator"
		cloPackageName = "cluster-logging"
		eoPackageName  = "elasticsearch-operator"
	)

	g.Context("JSON structured logs -- outputDefaults testing", func() {
		var (
			subTemplate       = exutil.FixturePath("testdata", "logging", "subscription", "sub-template.yaml")
			SingleNamespaceOG = exutil.FixturePath("testdata", "logging", "subscription", "singlenamespace-og.yaml")
			AllNamespaceOG    = exutil.FixturePath("testdata", "logging", "subscription", "allnamespace-og.yaml")
			jsonLogFile       = exutil.FixturePath("testdata", "logging", "generatelog", "container_json_log_template.json")
			nonJsonLogFile    = exutil.FixturePath("testdata", "logging", "generatelog", "container_non_json_log_template.json")
		)
		cloNS := "openshift-logging"
		eoNS := "openshift-operators-redhat"
		CLO := SubscriptionObjects{clo, cloNS, SingleNamespaceOG, subTemplate, cloPackageName, CatalogSourceObjects{}}
		EO := SubscriptionObjects{eo, eoNS, AllNamespaceOG, subTemplate, eoPackageName, CatalogSourceObjects{}}
		g.BeforeEach(func() {
			//deploy CLO and EO
			//CLO is deployed to `openshift-logging` namespace by default
			//and EO is deployed to `openshift-operators-redhat` namespace
			g.By("deploy CLO and EO")
			CLO.SubscribeLoggingOperators(oc)
			EO.SubscribeLoggingOperators(oc)
			oc.SetupProject()
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-41847-High-41848-structured index by kubernetes.labels.test/openshift.labels.team [Serial][Slow]", func() {
			// create a project, then create a pod in the project to generate some json logs
			g.By("create some json logs")
			app_proj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", app_proj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "42475.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj, "-p", "STRUCTURED_TYPE_KEY=kubernetes.labels.test")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			g.By("deploy EFK pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace)
			g.By("waiting for the EFK pods to be ready...")
			WaitForEFKPodsToBeReady(oc, cloNS)

			// check data in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-centos-logtest", "")

			//check if the JSON logs are parsed
			check_log := "es_util --query=app-centos-logtest*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj + "\"}}}'"
			logs := searchInES(oc, cloNS, podList.Items[0].Name, check_log)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			//create clusterlogforwarder instance
			e2e.Logf("start testing OCP-41848")
			g.By("change clusterlogforwarder/instance")
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj, "-p", "STRUCTURED_TYPE_KEY=openshift.labels.team")
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")
			// check data in ES
			g.By("check indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-qa-openshift-label", "")
			//check if the JSON logs are parsed
			check_log_2 := "es_util --query=app-qa-openshift-label*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj + "\"}}}'"
			logs_2 := searchInES(oc, cloNS, podList.Items[0].Name, check_log_2)
			o.Expect(logs_2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42475-High-42386-structured index by kubernetes.container_name/kubernetes.namespace_name [Serial][Slow]", func() {
			// create a project, then create a pod in the project to generate some json logs
			g.By("create some json logs")
			app_proj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", app_proj, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "42475.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj, "-p", "STRUCTURED_TYPE_KEY=kubernetes.container_name")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			g.By("deploy EFK pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace)
			g.By("waiting for the EFK pods to be ready...")
			WaitForEFKPodsToBeReady(oc, cloNS)

			// check data in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-logging-centos-logtest", "")
			//check if the JSON logs are parsed
			check_log := "es_util --query=app-logging-centos-logtest*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj + "\"}}}'"
			logs := searchInES(oc, cloNS, podList.Items[0].Name, check_log)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			e2e.Logf("start testing OCP-42386")
			g.By("updating clusterlogforwarder")
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj, "-p", "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")
			o.Expect(err).NotTo(o.HaveOccurred())
			WaitForDaemonsetPodsToBeReady(oc, cloNS, "collector")
			// check data in ES
			g.By("check indices in ES pod")
			podList, err = oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-"+app_proj, "")
			//check if the JSON logs are parsed
			check_log_2 := "es_util --query=app-" + app_proj + "*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj + "\"}}}'"
			logs_2 := searchInES(oc, cloNS, podList.Items[0].Name, check_log_2)
			o.Expect(logs_2.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))
		})

		// author: qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42363-structured and default index[Serial]", func() {
			//create 2 projects and generate json logs in each project
			g.By("create some json logs")
			app_proj_1 := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", app_proj_1, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			oc.SetupProject()
			app_proj_2 := oc.Namespace()
			err = oc.WithoutNamespace().Run("new-app").Args("-n", app_proj_2, "-f", jsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "42363.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj_1, "-p", "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			g.By("deploy EFK pods")
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "APP_LOG_MAX_AGE=10m")
			g.By("waiting for the EFK pods to be ready...")
			WaitForEFKPodsToBeReady(oc, cloNS)

			// check indices name in ES
			g.By("check indices in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			for _, indexName := range []string{"app-" + app_proj_1, "app-00", "infra-00", "audit-00"} {
				waitForIndexAppear(oc, cloNS, podList.Items[0].Name, indexName, "")
			}

			// check log in ES
			// logs in proj_1 should be stored in index "app-${app_proj_1}" and json logs should be parsed
			// logs in proj_2,proj_1 should be stored in index "app-000xxx", no json structured logs
			check_log_1 := "es_util --query=app-" + app_proj_1 + "*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj_1 + "\"}}}'"
			logs_1 := searchInES(oc, cloNS, podList.Items[0].Name, check_log_1)
			o.Expect(logs_1.Hits.DataHits[0].Source.Structured.Message).Should(o.Equal("MERGE_JSON_LOG=true"))

			for _, proj := range []string{app_proj_1, app_proj_2} {
				check_log_2 := "es_util --query=app-00*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + proj + "\"}}}'"
				logs_2 := searchInES(oc, cloNS, podList.Items[0].Name, check_log_2)
				o.Expect(logs_2.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())
			}

			// check if the retention policy works with the new indices
			// set managementState to Unmanaged in es/elasticsearch
			err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("es/elasticsearch", "-n", cloNS, "-p", "{\"spec\": {\"managementState\": \"Unmanaged\"}}", "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			indices1, _ := getESIndicesByName(oc, cloNS, podList.Items[0].Name, "app-"+app_proj_1)
			indexNames1 := make([]string, 0, len(indices1))
			for _, index := range indices1 {
				indexNames1 = append(indexNames1, index.Index)
			}
			e2e.Logf("indexNames1: %v\n\n", indexNames1)
			// change the schedule of cj/elasticsearch-im-xxx, make it run in every 2 minute
			for _, cj := range []string{"elasticsearch-im-app", "elasticsearch-im-infra", "elasticsearch-im-audit"} {
				err = oc.AsAdmin().WithoutNamespace().Run("patch").Args("cronjob/"+cj, "-n", cloNS, "-p", "{\"spec\": {\"schedule\": \"*/2 * * * *\"}}").Execute()
				o.Expect(err).NotTo(o.HaveOccurred())
			}
			// remove all the jobs
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("job", "-n", cloNS, "--all").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIMJobsToComplete(oc, cloNS, 180*time.Second)
			indices2, _ := getESIndicesByName(oc, cloNS, podList.Items[0].Name, "app-"+app_proj_1)
			indexNames2 := make([]string, 0, len(indices2))
			for _, index := range indices2 {
				indexNames2 = append(indexNames2, index.Index)
			}
			e2e.Logf("indexNames2: %v\n\n", indexNames2)
			o.Expect(indexNames1).NotTo(o.Equal(indexNames2))
		})

		// author qitang@redhat.com
		g.It("CPaasrunOnly-Author:qitang-High-42419-Fall into app-00* index if message is not json[Serial]", func() {
			// create a project, then create a pod in the project to generate some non-json logs
			g.By("create some non-json logs")
			app_proj := oc.Namespace()
			err := oc.WithoutNamespace().Run("new-app").Args("-n", app_proj, "-f", nonJsonLogFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			//create clusterlogforwarder instance
			g.By("create clusterlogforwarder/instance")
			clfTemplate := exutil.FixturePath("testdata", "logging", "clusterlogforwarder", "42475.yaml")
			clf := resource{"clusterlogforwarder", "instance", cloNS}
			defer clf.clear(oc)
			err = clf.applyFromTemplate(oc, "-n", clf.namespace, "-f", clfTemplate, "-p", "DATA_PROJECT="+app_proj, "-p", "STRUCTURED_TYPE_KEY=kubernetes.namespace_name")
			o.Expect(err).NotTo(o.HaveOccurred())

			// create clusterlogging instance
			g.By("deploy EFK pods")
			sc, err := getStorageClassName(oc)
			o.Expect(err).NotTo(o.HaveOccurred())
			instance := exutil.FixturePath("testdata", "logging", "clusterlogging", "cl-storage-template.yaml")
			cl := resource{"clusterlogging", "instance", cloNS}
			defer cl.deleteClusterLogging(oc)
			cl.createClusterLogging(oc, "-n", cl.namespace, "-f", instance, "-p", "NAMESPACE="+cl.namespace, "-p", "STORAGE_CLASS="+sc)
			g.By("waiting for the EFK pods to be ready...")
			WaitForEFKPodsToBeReady(oc, cloNS)

			g.By("check logs in ES pod")
			podList, err := oc.AdminKubeClient().CoreV1().Pods(cloNS).List(metav1.ListOptions{LabelSelector: "es-node-master=true"})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForIndexAppear(oc, cloNS, podList.Items[0].Name, "app-00", "")
			waitForProjectLogsAppear(oc, cloNS, podList.Items[0].Name, app_proj, "app-00")
			check_log := "es_util --query=app-00*/_search?format=JSON -d '{\"size\": 1, \"sort\": [{\"@timestamp\": {\"order\":\"desc\"}}], \"query\": {\"match\": {\"kubernetes.namespace_name\": \"" + app_proj + "\"}}}'"
			logs := searchInES(oc, cloNS, podList.Items[0].Name, check_log)
			o.Expect(logs.Hits.DataHits[0].Source.Structured.Message).Should(o.BeEmpty())

		})
	})

})
