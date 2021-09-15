package operators

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"

	"github.com/google/go-github/github"
	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	opm "github.com/openshift/openshift-tests-private/test/extended/opm"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	container "github.com/openshift/openshift-tests-private/test/extended/util/container"
	db "github.com/openshift/openshift-tests-private/test/extended/util/db"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-operators] OLM should", func() {
	defer g.GinkgoRecover()

	var oc = exutil.NewCLI("default-"+getRandomString(), exutil.KubeConfigPath())

	// author: jiazha@redhat.com
	g.It("Author:jiazha-High-43487-3rd party Operator Catalog references change during an OCP Upgrade", func() {
		g.By("1) get the Kubernetes version")
		version, err := oc.AsAdmin().WithoutNamespace().Run("version").Args("--short").Output()
		if err != nil {
			e2e.Failf("Fail to get the Kubernetes version")
		}
		reg := regexp.MustCompile(`.*v((\d+).(\d+).(\d+))-`)
		if reg == nil {
			e2e.Failf("version regexp err!")
		}
		result := reg.FindAllStringSubmatch(version, -1)
		// fullVersion := result[0][1]
		majorVersion := result[0][2]
		minorVersion := result[0][3]
		patchVersion := result[0][4]

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		imageTemplates := map[string]string{
			"quay.io/kube-release-v{kube_major_version}/catalog:v{kube_major_version}":                                       majorVersion,
			"quay.io/kube-release-v{kube_major_version}/catalog:v{kube_major_version}.{kube_minor_version}":                  fmt.Sprintf("%s.%s", majorVersion, minorVersion),
			"quay.io/olmqe-v{kube_major_version}/etcd-index:v{kube_major_version}.{kube_minor_version}.{kube_patch_version}": fmt.Sprintf("%s.%s.%s", majorVersion, minorVersion, patchVersion),
		}

		oc.SetupProject()
		for k, fullV := range imageTemplates {
			g.By(fmt.Sprintf("create a CatalogSource with imageTemplate:%s", k))
			buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
			csImageTemplate := filepath.Join(buildPruningBaseDir, "cs-image-template.yaml")
			cs := catalogSourceDescription{
				name:          fmt.Sprintf("cs-43487-%s", fullV),
				namespace:     oc.Namespace(),
				displayName:   "OLM QE Operators",
				publisher:     "Jian",
				sourceType:    "grpc",
				address:       "quay.io/olmqe-v1/etcd-index:v1.21",
				imageTemplate: k,
				template:      csImageTemplate,
			}

			defer cs.delete(itName, dr)
			cs.create(oc, itName, dr)
			// It will fail due to "ImagePullBackOff" since no this CatalogSource image in fact, so remove the status checking
			// newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", oc.Namespace(), "-o=jsonpath={.status..lastObservedState}"}).check(oc)

			g.By("3) get the real CatalogSource image version")
			err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
				// oc get catalogsource cs-43487 -o=jsonpath={.spec.image}
				image, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", cs.name, "-n", oc.Namespace(), "-o=jsonpath={.spec.image}").Output()
				if err != nil {
					e2e.Failf("Fail to get the CatalogSource(%s)'s image, error: %v", cs.name, err)
				}
				if image == "" {
					return false, nil
				}

				reg1 := regexp.MustCompile(`.*-v(\d+).*:v(\d+(.\d+)?(.\d+)?)`)
				if reg1 == nil {
					e2e.Failf("image regexp err!")
				}
				result1 := reg1.FindAllStringSubmatch(image, -1)
				imageMajorVersion := result1[0][1]
				imageFullVersion := result1[0][2]
				e2e.Logf("fullVersion:%s, majorVersion:%s, imageFullVersion:%s, imageMajorVersion:%s", fullV, majorVersion, imageFullVersion, imageMajorVersion)
				if imageMajorVersion != majorVersion || imageFullVersion != fullV {
					e2e.Failf("This CatalogSource(%s) image version(%s) doesn't follow the image template(%s)!", cs.name, image, k)
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("catsrc %s image version not expected", cs.name))
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-43191-Medium-43271-Bundle Content Compression", func() {
		g.By("1) Subscribe to etcdoperator v0.9.4 in a random project")
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-43191",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		defer og.delete(itName, dr)
		og.createwithCheck(oc, itName, dr)

		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-43191",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		defer sub.deleteCSV(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("2) check if the extract job uses the zip flag")
		// ["opm","alpha","bundle","extract","-m","/bundle/","-n","openshift-marketplace","-c","9b59f03f8e8ea2f818061847881908aae51cf41836e4a3b822dcc6d3a01481c","-z"]
		extractCommand, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("job", "-n", "openshift-marketplace", "-o=jsonpath={.items[0].spec.template.spec.containers[0].command}").Output()
		if err != nil {
			e2e.Failf("Fail to get the jobs in the openshift-marketplace project: %v", err)
		}
		if !strings.Contains(extractCommand, "-z") {
			e2e.Failf("This bundle extract job doesn't use the opm compression feature!")
		}

		g.By("3) check if the compression content is empty")
		bData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", "openshift-marketplace", "-o=jsonpath={.items[0].binaryData}").Output()
		if err != nil {
			e2e.Failf("Fail to get ConfigMap's binaryData: %v", err)
		}
		if bData == "" {
			e2e.Failf("The compression content is empty!")
		}
	})

	// author: jiazha@redhat.com
	g.It("ConnectedOnly-Author:jiazha-High-43101-OLM blocks minor OpenShift upgrades when incompatible optional operators are installed", func() {
		// consumes this index imaage: quay.io/olmqe/etcd-index:upgrade-auto, it contains the etcdoperator v0.9.2, v0.9.4, v0.9.5
		g.By("1) Create a CatalogSource in the openshift-marketplace project")
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		cs := catalogSourceDescription{
			name:        "cs-43101",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE Operators",
			publisher:   "Jian",
			sourceType:  "grpc",
			address:     "quay.io/olmqe/etcd-index:upgrade-auto",
			template:    csImageTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		defer cs.delete(itName, dr)
		cs.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", "openshift-marketplace", "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("2) Install the OperatorGroup in a random project")
		oc.SetupProject()
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-43101",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		defer og.delete(itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("3) Install the etcdoperator v0.9.2 with Manual approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-43101",
			namespace:              oc.Namespace(),
			catalogSourceName:      "cs-43101",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Manual",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.2",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer sub.update(oc, itName, dr)
		sub.create(oc, itName, dr)

		g.By("4) Apprrove this etcdoperator.v0.9.2, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.2", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		// olm.properties: '[{"type": "olm.maxOpenShiftVersion", "value": " "}]'
		g.By("5) This operator's olm.maxOpenShiftVersion is empty, so it should block the upgrade")
		CheckUpgradeStatus(oc, "False")

		g.By("6) Apprrove this etcdoperator.v0.9.4, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.4", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
		// olm.properties: '[{"type": "olm.maxOpenShiftVersion", "value": "4.9"}]'
		g.By("7) 4.9.0-xxx upgraded to 4.10.0-xxx < 4.10.0, or 4.9.1 upgraded to 4.9.x < 4.10.0, so it should NOT block the 4.9 upgrade.")
		CheckUpgradeStatus(oc, "True")

		g.By("8) Apprrove this etcdoperator.v0.9.5, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.5", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.5", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
		// olm.properties: '[{"type": "olm.maxOpenShiftVersion", "value": "4.10.0"}]'
		g.By("9) 4.9.0-xxx upgraded to 4.10.0-xxx < 4.10.0, or 4.9.1 upgraded to 4.9.x < 4.11.0, so it should NOT block the 4.9 upgrade.")
		CheckUpgradeStatus(oc, "True")
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-43977-OPENSHIFT_VERSIONS in assisted operator subscription does not propagate", func() {
		// this operator must be installed in the default project since the env variable: MY_POD_NAMESPACE = default
		g.By("1) create the OperatorGroup in the default project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-43977",
			namespace: "default",
			template:  ogSingleTemplate,
		}
		defer og.delete(itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("2) subscribe to the etcdoperator v0.9.4 with ENV variables")
		subTemplate := filepath.Join(buildPruningBaseDir, "env-subscription.yaml")

		sub := subscriptionDescription{
			subName:                "sub-43977",
			namespace:              "default",
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		defer sub.deleteCSV(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", "default", "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3) check those env variables")
		envVars := map[string]string{
			"MY_POD_NAMESPACE":        "default",
			"OPERATOR_CONDITION_NAME": "etcdoperator.v0.9.4",
			"OPENSHIFT_VERSIONS":      "4.8",
		}
		// oc get deployment etcd-operator -o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"MY_POD_NAMESPACE\")].value}
		// oc get deployment etcd-operator -o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"OPERATOR_CONDITION_NAME\")].value}
		// oc get deployment etcd-operator -o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"OPENSHIFT_VERSIONS\")].value}
		for k, v := range envVars {
			jsonpath := fmt.Sprintf("-o=jsonpath={.spec.template.spec.containers[0].env[?(@.name==\"%s\")].value}", k)
			envVar, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "etcd-operator", "-n", "default", jsonpath).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(envVar, v) {
				e2e.Failf("The value of the %s should be %s, but get %s!", k, v, envVar)
			}
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-43978-Catalog pods don't report termination logs to catalog-operator", func() {
		catalogs, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "-n", "openshift-marketplace").Output()
		if err != nil {
			e2e.Failf("Fail to get the CatalogSource in openshift-marketplace project")
		}
		defaultCatalogs := []string{"certified-operators", "community-operators", "redhat-marketplace", "redhat-operators"}
		for i, catalog := range defaultCatalogs {
			g.By(fmt.Sprintf("%d) check CatalogSource: %s", i+1, catalog))
			if strings.Contains(catalogs, catalog) {
				policy, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", fmt.Sprintf("olm.catalogSource=%s", catalog), "-n", "openshift-marketplace", "-o=jsonpath={.items[0].spec.containers[0].terminationMessagePolicy}").Output()
				if err != nil {
					e2e.Failf("Fail to get the policy of the CatalogSource's pod")
				}
				if policy != "FallbackToLogsOnError" {
					e2e.Failf("CatalogSource:%s uses the %s policy, not the FallbackToLogsOnError!", catalog, policy)
				}
			} else {
				e2e.Logf("CatalogSource:%s doesn't install on this cluster", catalog)
			}
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-43803-Only one of multiple subscriptions to the same package is honored", func() {
		g.By("1) create the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-43803",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) subscribe to the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-43803",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		defer sub.deleteCSV(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3) re-subscribe to this etcd operator with another subscription name")
		sub2 := subscriptionDescription{
			subName:                "sub2-43803",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub2.delete(itName, dr)
		sub2.createWithoutCheck(oc, itName, dr)

		g.By("4) Check OLM logs")
		err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			logs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args("deploy/catalog-operator", "-n", "openshift-operator-lifecycle-manager").Output()
			if err != nil {
				e2e.Failf("Fail to get the OLM logs")
			}
			res, _ := regexp.MatchString(".*constraints not satisfiable.*subscription sub2-43803", logs)
			if res {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "subscription sub2-43803 constraints satisfiable")

	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-High-43135-PackageServer respects single-node configuration [Disruptive]", func() {
		g.By("1) get the cluster infrastructure")
		infra, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructures", "cluster", "-o=jsonpath={.status.infrastructureTopology}").Output()
		if err != nil {
			e2e.Failf("Fail to get the cluster infra")
		}
		num, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-operator-lifecycle-manager", "deployment", "packageserver", "-o=jsonpath={.status.replicas}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if infra == "SingleReplica" {
			e2e.Logf("This is a SNO cluster")
			g.By("2) check if only have one packageserver pod")
			if num != "1" {
				e2e.Failf("!!!Fail, should only have 1 packageserver pod, but get %s!", num)
			}
			// make sure the CVO recover if any error in the follow steps
			defer func() {
				_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas", "1", "deployment/cluster-version-operator", "-n", "openshift-cluster-version").Output()
				if err != nil {
					e2e.Failf("Defer: fail to enable CVO")
				}
			}()
			g.By("3) stop CVO")
			_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas", "0", "deployment/cluster-version-operator", "-n", "openshift-cluster-version").Output()
			if err != nil {
				e2e.Failf("Fail to stop CVO")
			}
			g.By("4) stop the PSM")
			_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas", "0", "deployment/package-server-manager", "-n", "openshift-operator-lifecycle-manager").Output()
			if err != nil {
				e2e.Failf("Fail to stop the PSM")
			}
			g.By("5) patch the replica to 3")
			// oc get csv packageserver -o=jsonpath={.spec.install.spec.deployments[?(@.name==\"packageserver\")].spec.replicas}
			// oc patch csv/packageserver -p '{"spec":{"install":{"spec":{"deployments":[{"name":"packageserver", "spec":{"replicas":3, "template":{}, "selector":{"matchLabels":{"app":"packageserver"}}}}]}}}}' --type=merge
			// oc patch deploy/packageserver -p '{"spec":{"replicas":3}}' --type=merge
			patchResource(oc, asAdmin, withoutNamespace, "-n", "openshift-operator-lifecycle-manager", "deployment", "packageserver", "-p", "{\"spec\":{\"replicas\":3}}", "--type=merge")
			err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
				num, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.status.availableReplicas}").Output()
				if num != "3" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "packageserver replicas is 3")
			g.By("6) enable CVO")
			_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("--replicas", "1", "deployment/cluster-version-operator", "-n", "openshift-cluster-version").Output()
			if err != nil {
				e2e.Failf("Fail to enable CVO")
			}
			g.By("7) check if the PSM back")
			err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
				num, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "package-server-manager", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.status.replicas}").Output()
				if num != "1" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "package-server-manager replicas is 1")
			g.By("8) check if the packageserver pods number back to 1")
			err = wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
				num, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.status.availableReplicas}").Output()
				if num != "1" {
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "packageserver replicas is 1")
		} else {
			// HighlyAvailable
			e2e.Logf("This is HA cluster, not SNO")
			g.By("2) check if only have two packageserver pods")
			if num != "2" {
				e2e.Failf("!!!Fail, should only have 2 packageserver pods, but get %s!", num)
			}
		}
	})

	// author: jiazha@redhat.com
	// add `Serial` label since this etcd-operator are subscribed for cluster-scoped,
	// that means may leads to other etcd-opertor subscription fail if in Parallel
	g.It("ConnectedOnly-Author:jiazha-High-37826-use an PullSecret for the private Catalog Source image [Serial]", func() {
		g.By("1) Create a pull secert for CatalogSource")
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		dockerConfig := filepath.Join(buildPruningBaseDir, "dockerconfig.json")
		_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("-n", "openshift-marketplace", "secret", "generic", "secret-37826", fmt.Sprintf("--from-file=.dockerconfigjson=%s", dockerConfig), "--type=kubernetes.io/dockerconfigjson").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-marketplace", "secret", "secret-37826").Execute()

		g.By("2) Install this private CatalogSource in the openshift-marketplace project")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		cs := catalogSourceDescription{
			name:        "cs-37826",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE Operators",
			publisher:   "Jian",
			sourceType:  "grpc",
			address:     "quay.io/olmqe/cs:private",
			template:    csImageTemplate,
			secret:      "secret-37826",
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		cs.create(oc, itName, dr)
		defer cs.delete(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", "openshift-marketplace", "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("4) Install the etcdoperator v0.9.4 from this private image")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-37826",
			namespace:              "openshift-operators",
			catalogSourceName:      "cs-37826",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "clusterwide-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4-clusterwide",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		defer sub.deleteCSV(itName, dr)

		// get the InstallPlan name
		ipName := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installplan.name}")
		if strings.Contains(ipName, "NotFound") {
			e2e.Failf("!!!Fail to get the InstallPlan of sub: %s/%s", sub.namespace, sub.subName)
		}
		// get the unpack job name
		manifest := getResource(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace, ipName, "-o=jsonpath={.status.plan[0].resource.manifest}")
		valid := regexp.MustCompile(`name":"(\S+)","namespace"`)
		job := valid.FindStringSubmatch(manifest)
		g.By("5) Only check if the job pod works well")
		// in this test case, we don't need to care about if the operator pods works well.
		// more details: https://bugzilla.redhat.com/show_bug.cgi?id=1909992#c5
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"-n", "openshift-marketplace", "pods", "-l", fmt.Sprintf("job-name=%s", string(job[1])), "-o=jsonpath={.items[0].status.phase}"}).check(oc)

	})

	// author: chuo@redhat.com
	g.It("Author:jiazha-High-24028-need to set priorityClassName as system-cluster-critical", func() {
		var deploymentResource = [3]string{"catalog-operator", "olm-operator", "packageserver"}
		for _, v := range deploymentResource {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-operator-lifecycle-manager", "deployment", v, "-o=jsonpath={.spec.template.spec.priorityClassName}").Output()
			e2e.Logf("%s.priorityClassName:%s", v, msg)
			if err != nil {
				e2e.Failf("Unable to get %s, error:%v", msg, err)
			}
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.Equal("system-cluster-critical"))
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-High-21548-aggregates CR roles to standard admin/view/edit", func() {
		oc.SetupProject()
		msg, err := oc.Run("whoami").Args("").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("oc whoami: %s", msg)
		o.Expect(msg).NotTo(o.Equal("system:admin"))

		authorizations := []struct {
			resource string
			action   []string
			result   bool
		}{
			{
				resource: "subscriptions",
				action:   []string{"create", "update", "patch", "delete", "get", "list", "watch"},
				result:   true,
			},
			{
				resource: "installplans",
				action:   []string{"create", "update", "patch"},
				result:   false,
			},
			{
				resource: "installplans",
				action:   []string{"get", "list", "watch", "delete"},
				result:   true,
			},
			{
				resource: "catalogsources",
				action:   []string{"get", "list", "watch", "delete"},
				result:   true,
			},
			{
				resource: "catalogsources",
				action:   []string{"create", "update", "patch"},
				result:   false,
			},
			{
				resource: "clusterserviceversions",
				action:   []string{"get", "list", "watch", "delete"},
				result:   true,
			},
			{
				resource: "clusterserviceversions",
				action:   []string{"create", "update", "patch"},
				result:   false,
			},
			{
				resource: "operatorgroups",
				action:   []string{"get", "list", "watch"},
				result:   true,
			},
			{
				resource: "operatorgroups",
				action:   []string{"create", "update", "patch", "delete"},
				result:   false,
			},
			{
				resource: "packagemanifests",
				action:   []string{"get", "list", "watch"},
				result:   true,
			},
			// Based on https://github.com/openshift/operator-framework-olm/blob/master/staging/operator-lifecycle-manager/deploy/chart/templates/0000_50_olm_09-aggregated.clusterrole.yaml#L30
			// But, it returns '*', I will reseach it later.
			// $ oc get clusterrole admin -o yaml |grep packagemanifests -A5
			// - packagemanifests
			// verbs:
			// - '*'
			// {
			// 	resource: "packagemanifests",
			// 	action:   []string{"create", "update", "patch", "delete"},
			// 	result:   false,
			// },
		}

		for _, v := range authorizations {
			for _, act := range v.action {
				res, err := oc.Run("auth").Args("can-i", act, v.resource).Output()
				e2e.Logf(fmt.Sprintf("oc auth can-i %s %s", act, v.resource))
				if res != "no" && err != nil {
					o.Expect(err).NotTo(o.HaveOccurred())
				}
				if v.result {
					o.Expect(res).To(o.Equal("yes"))
				} else {
					o.Expect(res).To(o.Equal("no"))
				}
			}
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-High-37442-create a Conditions CR for each Operator it installs", func() {
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-37442",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-37442",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3) Check if OperatorCondition generated well")
		newCheck("expect", asAdmin, withoutNamespace, compare, "etcd-operator", ok, []string{"operatorcondition", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.spec.deployments[0]}"}).check(oc)
		// three containers: etcd-operator etcd-backup-operator etcd-restore-operator
		newCheck("expect", asAdmin, withoutNamespace, compare, "etcdoperator.v0.9.4 etcdoperator.v0.9.4 etcdoperator.v0.9.4", ok, []string{"deployment", "etcd-operator", "-n", oc.Namespace(), "-o=jsonpath={.spec.template.spec.containers[*].env[?(@.name==\"OPERATOR_CONDITION_NAME\")].value}"}).check(oc)
		// this etcdoperator.v0.9.4 role should be owned by OperatorCondition
		newCheck("expect", asAdmin, withoutNamespace, compare, "OperatorCondition", ok, []string{"role", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.metadata.ownerReferences[0].kind}"}).check(oc)
		// this etcdoperator.v0.9.4 role should be added to etcd-operator SA
		newCheck("expect", asAdmin, withoutNamespace, compare, "etcd-operator", ok, []string{"rolebinding", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.subjects[0].name}"}).check(oc)

		g.By("4) delete the operator so that can check the related resource in next step")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)

		g.By("5) Check if the related resources are removed successfully")
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"operatorcondition", "etcdoperator.v0.9.4", "-n", oc.Namespace()}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"role", "etcdoperator.v0.9.4", "-n", oc.Namespace()}).check(oc)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"rolebinding", "etcdoperator.v0.9.4", "-n", oc.Namespace()}).check(oc)

	})

	// author: jiazha@redhat.com
	// update at June 16, 2021 due to https://bugzilla.redhat.com/show_bug.cgi?id=1927340
	// details: https://hackmd.io/9wG20hu5TU-y1HrkhvcsZQ?view
	g.It("ConnectedOnly-Author:jiazha-Medium-37710-supports the Upgradeable Supported Condition", func() {
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-37710",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.2 with Manual approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-37710",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Manual",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.2",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer sub.update(oc, itName, dr)
		sub.create(oc, itName, dr)

		g.By("3) Apprrove this etcdoperator.v0.9.2, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.2", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		// The conditions array will be added to OperatorCondition’s spec and operator is now expected to only update the conditions in the spec to reflect its condition
		// and no longer push changes to OperatorCondition’s status.
		// $oc patch operatorcondition etcdoperator.v0.9.2 -p '{"spec":{"conditions":[{"type":"Upgradeable", "observedCondition":1,"status":"False","reason":"bug","message":"not ready","lastUpdateTime":"2021-06-16T16:56:44Z","lastTransitionTime":"2021-06-16T16:56:44Z"}]}}' --type=merge
		g.By("4) Patch the spec.conditions[0].Upgradeable to False")
		patchResource(oc, asAdmin, withoutNamespace, "-n", oc.Namespace(), "operatorcondition", "etcdoperator.v0.9.2", "-p", "{\"spec\": {\"conditions\": [{\"type\": \"Upgradeable\", \"status\": \"False\", \"reason\": \"upgradeIsNotSafe\", \"message\": \"Disbale the upgrade\", \"observedCondition\":1, \"lastUpdateTime\":\"2021-06-16T16:56:44Z\",\"lastTransitionTime\":\"2021-06-16T16:56:44Z\"}]}}", "--type=merge")

		newCheck("expect", asAdmin, withoutNamespace, compare, "Upgradeable", ok, []string{"operatorcondition", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[0].type}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "False", ok, []string{"operatorcondition", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.conditions[0].status}"}).check(oc)

		g.By("5) Apprrove this etcdoperator.v0.9.4, the corresponding CSV should be in Pending state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.4", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Pending", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("6) Check the CSV message, the operator is not upgradeable")
		err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", oc.Namespace(), "csv", "etcdoperator.v0.9.4", "-o=jsonpath={.status.message}").Output()
			if !strings.Contains(msg, "operator is not upgradeable") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "etcdoperator.v0.9.4 operator is upgradeable")

		g.By("7) Patch the spec.conditions[0].Upgradeable to True")
		// $oc patch operatorcondition etcdoperator.v0.9.2 -p '{"spec":{"conditions":[{"type":"Upgradeable", "observedCondition":1,"status":"True","reason":"bug","message":"ready","lastUpdateTime":"2021-06-16T16:56:44Z","lastTransitionTime":"2021-06-16T16:56:44Z"}]}}' --type=merge
		patchResource(oc, asAdmin, withoutNamespace, "-n", oc.Namespace(), "operatorcondition", "etcdoperator.v0.9.2", "-p", "{\"spec\": {\"conditions\": [{\"type\": \"Upgradeable\", \"status\": \"True\", \"reason\": \"ready\", \"message\": \"enable the upgrade\", \"observedCondition\":1, \"lastUpdateTime\":\"2021-06-16T17:56:44Z\",\"lastTransitionTime\":\"2021-06-16T17:56:44Z\"}]}}", "--type=merge")
		g.By("8) the etcdoperator.v0.9.2 can be upgraded to etcdoperator.v0.9.4 successfully")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-37631-Allow cluster admin to overwrite the OperatorCondition", func() {
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-37631",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.2 with Manual approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-37631",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Manual",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.2",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer sub.update(oc, itName, dr)
		sub.create(oc, itName, dr)

		g.By("3) Apprrove this etcdoperator.v0.9.2, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.2", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("4) Patch the OperatorCondition to set the Upgradeable to False")
		patchResource(oc, asAdmin, withoutNamespace, "-n", oc.Namespace(), "operatorcondition", "etcdoperator.v0.9.2", "-p", "{\"spec\": {\"overrides\": [{\"type\": \"Upgradeable\", \"status\": \"False\", \"reason\": \"upgradeIsNotSafe\", \"message\": \"Disbale the upgrade\"}]}}", "--type=merge")

		g.By("5) Apprrove this etcdoperator.v0.9.4, the corresponding CSV should be in Pending state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.4", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Pending", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("6) Check the CSV message, the operator is not upgradeable")
		err := wait.Poll(3*time.Second, 60*time.Second, func() (bool, error) {
			msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", oc.Namespace(), "csv", "etcdoperator.v0.9.4", "-o=jsonpath={.status.message}").Output()
			if !strings.Contains(msg, "operator is not upgradeable") {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "etcdoperator.v0.9.4 operator is upgradeable")

		g.By("7) Change the Upgradeable of the OperatorCondition to True")
		patchResource(oc, asAdmin, withoutNamespace, "-n", oc.Namespace(), "operatorcondition", "etcdoperator.v0.9.2", "-p", "{\"spec\": {\"overrides\": [{\"type\": \"Upgradeable\", \"status\": \"True\", \"reason\": \"upgradeIsNotSafe\", \"message\": \"Disbale the upgrade\"}]}}", "--type=merge")

		g.By("8) the etcdoperator.v0.9.2 should be upgraded to etcdoperator.v0.9.4 successfully")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// author: jiazha@redhat.com
	g.It("ConnectedOnly-Author:jiazha-Medium-33450-Operator upgrades can delete existing CSV before completion", func() {
		g.By("1) Install a customization CatalogSource CR")
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		cs := catalogSourceDescription{
			name:        "cs-33450",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE Operators",
			publisher:   "Jian",
			sourceType:  "grpc",
			// use the digest in case wrong updates. quay.io/openshifttest/etcd-index:0.9.4-sa
			address:  "quay.io/openshifttest/etcd-index@sha256:ba18c1d454c45ae470ed1e21b92b979ce85af845e95a0bf4390ee03017fb5768",
			template: csImageTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		cs.create(oc, itName, dr)
		defer cs.delete(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", "openshift-marketplace", "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("2) Subscribe to the etcd operator with Manual approval")
		oc.SetupProject()
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

		og := operatorGroupDescription{
			name:      "og-33450",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-33450",
			namespace:              oc.Namespace(),
			catalogSourceName:      "cs-33450",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "alpha",
			ipApproval:             "Manual",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.2",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)
		g.By("3) Apprrove the etcdoperator.v0.9.2, it should be in Complete state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.2", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("4) Apprrove the etcdoperator.v0.9.4, it should be in Failed state")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.4", "Failed")

		g.By("5) The etcdoperator.v0.9.4 CSV should be in Pending status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Pending", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("6) The SA should be owned by the etcdoperator.v0.9.2")
		err := wait.Poll(3*time.Second, 10*time.Second, func() (bool, error) {
			saOwner := getResource(oc, asAdmin, withoutNamespace, "sa", "etcd-operator", "-n", sub.namespace, "-o=jsonpath={.metadata.ownerReferences[0].name}")
			if strings.Compare(saOwner, "etcdoperator.v0.9.2") != 0 {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "sa etcd-operator owner is not etcdoperator.v0.9.2")

	})

	// author: jiazha@redhat.com
	g.It("ConnectedOnly-Author:jiazha-High-37260-should allow to create the default CatalogSource [Disruptive]", func() {
		g.By("1) Disable the default OperatorHub")
		patchResource(oc, asAdmin, withoutNamespace, "operatorhub", "cluster", "-p", "{\"spec\": {\"disableAllDefaultSources\": true}}", "--type=merge")
		defer patchResource(oc, asAdmin, withoutNamespace, "operatorhub", "cluster", "-p", "{\"spec\": {\"disableAllDefaultSources\": false}}", "--type=merge")
		g.By("1-1) Check if the default CatalogSource resource are removed")
		err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "redhat-operators", "-n", "openshift-marketplace").Output()
			if strings.Contains(res, "not found") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "redhat-operators found")

		g.By("2) Create a CatalogSource with a default CatalogSource name")
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		cs := catalogSourceDescription{
			name:        "redhat-operators",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE",
			publisher:   "OLM QE",
			sourceType:  "grpc",
			address:     "quay.io/openshift-qe-optional-operators/ocp4-index:latest",
			template:    csImageTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		cs.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)
		g.By("2-1) Check if this custom CatalogSource resource works well")
		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest").Output()
			if strings.Contains(res, "OLM QE") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "packagemanifest does not contain OLM QE")

		g.By("3) Delete the Marketplace pods and check if the custome CatalogSource still works well")
		g.By("3-1) get the marketplace-operator pod's name")
		podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "name=marketplace-operator", "-o=jsonpath={.items..metadata.name}", "-n", "openshift-marketplace").Output()
		if err != nil {
			e2e.Failf("Failed to get the marketplace pods")
		}
		g.By("3-2) delete/recreate the marketplace-operator pod")
		_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pods", podName, "-n", "openshift-marketplace").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// time.Sleep(30 * time.Second)
		// waiting for the new marketplace pod ready
		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l", "name=marketplace-operator", "-o=jsonpath={.items..status.phase}", "-n", "openshift-marketplace").Output()
			if strings.Contains(res, "Running") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "marketplace-operator pod is not running")
		g.By("3-3) check if the custom CatalogSource still there")
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest").Output()
			if strings.Contains(res, "OLM QE") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "packagemanifest does not contain OLM QE")

		g.By("4) Enable the default OperatorHub")
		patchResource(oc, true, true, "operatorhub", "cluster", "-p", "{\"spec\": {\"disableAllDefaultSources\": false}}", "--type=merge")
		g.By("4-1) Check if the default CatalogSource resource are back")
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", "redhat-operators", "-n", "openshift-marketplace", "-o=jsonpath={.status..lastObservedState}"}).check(oc)
		g.By("4-2) Check if the default CatalogSource works and the custom one are removed")
		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest").Output()
			if strings.Contains(res, "Red Hat Operators") && !strings.Contains(res, "OLM QE") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "packagemanifest does contain OLM QE or has no Red Hat Operators")
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-25922-Support spec.config.volumes and volumemount in Subscription", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		oc.SetupProject()
		og := operatorGroupDescription{
			name:      "test-og-25922",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		g.By(fmt.Sprintf("1) create the OperatorGroup in project: %s", oc.Namespace()))
		og.createwithCheck(oc, itName, dr)

		g.By("2) install etcd operator")
		sub := subscriptionDescription{
			subName:                "etcd-sub",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer sub.update(oc, itName, dr)
		sub.create(oc, itName, dr)

		g.By("3) create a ConfigMap")
		cmTemplate := filepath.Join(buildPruningBaseDir, "cm-template.yaml")

		cm := configMapDescription{
			name:      "special-config",
			namespace: oc.Namespace(),
			template:  cmTemplate,
		}
		cm.create(oc, itName, dr)

		g.By("4) Patch this ConfigMap a volume")
		sub.patch(oc, "{\"spec\": {\"channel\":\"singlenamespace-alpha\",\"config\":{\"volumeMounts\":[{\"mountPath\":\"/test\",\"name\":\"config-volume\"}],\"volumes\":[{\"configMap\":{\"name\":\"special-config\"},\"name\":\"config-volume\"}]},\"name\":\"etcd\",\"source\":\"community-operators\",\"sourceNamespace\":\"openshift-marketplace\"}}")
		err := wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=etcd-operator-alm-owned", "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("4-1) Get etcd operator pod name:%s", podName)
			result, err := oc.AsAdmin().Run("exec").Args(podName, "--", "cat", "/test/special.how").Output()
			e2e.Logf("4-2) Check if the ConfigMap mount well")
			if strings.Contains(result, "very") {
				e2e.Logf("4-3) The ConfigMap: special-config mount well")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of etcd-operator-alm-owned special-config not mount well")
		g.By("5) Patch a non-exist volume")
		sub.patch(oc, "{\"spec\":{\"channel\":\"singlenamespace-alpha\",\"config\":{\"volumeMounts\":[{\"mountPath\":\"/test\",\"name\":\"volume1\"}],\"volumes\":[{\"persistentVolumeClaim\":{\"claimName\":\"claim1\"},\"name\":\"volume1\"}]},\"name\":\"etcd\",\"source\":\"community-operators\",\"sourceNamespace\":\"openshift-marketplace\"}}")
		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			for i := 0; i < 2; i++ {
				g.By("5-1) Check the pods status")
				podStatus, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=etcd-operator-alm-owned", fmt.Sprintf("-o=jsonpath={.items[%d].status.phase}", i)).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if podStatus == "Pending" {
					g.By("5-2) The pod status is Pending as expected")
					return true, nil
				}
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of etcd-operator-alm-owned status is not Pending")
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-35631-Remove OperatorSource API", func() {
		g.By("1) Check the operatorsource resource")
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("operatorsource").Output()
		e2e.Logf("Get the expected error: %s", msg)
		o.Expect(msg).To(o.ContainSubstring("the server doesn't have a resource type"))

		// for current disconnected env, only have the default community CatalogSource CRs
		g.By("2) Check the default Community CatalogSource CRs")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "-n", "openshift-marketplace").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Get the installed CatalogSource CRs:\n %s", msg)
		// o.Expect(msg).To(o.ContainSubstring("certified-operators"))
		o.Expect(msg).To(o.ContainSubstring("community-operators"))
		// o.Expect(msg).To(o.ContainSubstring("redhat-marketplace"))
		// o.Expect(msg).To(o.ContainSubstring("redhat-operators"))
		g.By("3) Check the Packagemanifest")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "-n", "openshift-marketplace").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No resources found"))
	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Medium-31693-Check CSV information on the PackageManifest", func() {
		g.By("1) The relatedImages should exist")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "prometheus", "-o=jsonpath={.status.channels[?(.name=='beta')].currentCSVDesc.relatedImages}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("2) The minKubeVersion should exist")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "prometheus", "-o=jsonpath={.status.channels[?(.name=='beta')].currentCSVDesc.minKubeVersion}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.BeEmpty())

		g.By("3) In this case, nativeAPI is optional, and prometheus does not have any nativeAPIs, which is ok.")
		oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "prometheus", "-o=jsonpath={.status.channels[?(.name=='beta')].currentCSVDesc.nativeAPIs}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-24850-Allow users to edit the deployment of an active CSV", func() {

		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-24850",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-24850",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3) Get pod name")
		podName, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=etcd-operator-alm-owned", "-n", oc.Namespace(), "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4) Patch the deploy object by adding an environment variable")
		_, err = oc.AsAdmin().WithoutNamespace().Run("set").Args("env", "deploy/etcd-operator", "A=B", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("5) Get restarted pod name")
		podNameAfterPatch, err := oc.AsAdmin().Run("get").Args("pods", "-l", "name=etcd-operator-alm-owned", "-n", oc.Namespace(), "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podName).NotTo(o.Equal(podNameAfterPatch))

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-High-24387-Any CRD upgrade is allowed if there is only one owner in a cluster [Disruptive]", func() {
		var (
			catName             = "cs-24387"
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			csImageTemplate     = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		)

		oc.SetupProject()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		var (
			cs = catalogSourceDescription{
				name:        catName,
				namespace:   "openshift-marketplace",
				displayName: "OLM QE Operators",
				publisher:   "bandrade",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/etcd-index-24387:5.0",
				template:    csImageTemplate,
			}

			og = operatorGroupDescription{
				name:      "test-og-24387",
				namespace: oc.Namespace(),
				template:  ogSingleTemplate,
			}

			sub = subscriptionDescription{
				subName:                "etcd",
				namespace:              oc.Namespace(),
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				channel:                "singlenamespace-alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "etcd",
				singleNamespace:        true,
				template:               subFile,
				startingCSV:            "etcdoperator.v0.9.4",
			}

			subModified = subscriptionDescription{
				subName:                "etcd",
				namespace:              oc.Namespace(),
				catalogSourceName:      catName,
				catalogSourceNamespace: "openshift-marketplace",
				ipApproval:             "Automatic",
				template:               subFile,
				channel:                "alpha",
				operatorPackage:        "etcd",
				startingCSV:            "etcdoperator.v0.9.4",
				singleNamespace:        true,
			}
		)

		g.By("1) Create catalog source")
		defer cs.delete(itName, dr)
		cs.create(oc, itName, dr)

		g.By("2) Create the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		g.By("3) Start to subscribe to the Etcd operator")
		sub.create(oc, itName, dr)

		g.By("4) Delete Etcd subscription and csv")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)

		g.By("5) Start to subscribe to the Etcd operator with the modifier crd")
		subModified.create(oc, itName, dr)

		g.By("6) Get property propertyIncludedTest in etcdclusters.etcd.database.coreos.com")
		crdYamlOutput, err := oc.AsAdmin().Run("get").Args("crd", "etcdclusters.etcd.database.coreos.com", "-o=yaml").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(crdYamlOutput).To(o.ContainSubstring("propertyIncludedTest"))

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-42970-OperatorGroup status indicates cardinality conflicts - SingleNamespace", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		)

		oc.SetupProject()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		ns := oc.Namespace()
		dr.addIr(itName)

		var (
			og = operatorGroupDescription{
				name:      "og-42970",
				namespace: ns,
				template:  ogSingleTemplate,
			}
			og1 = operatorGroupDescription{
				name:      "og-42970-1",
				namespace: ns,
				template:  ogSingleTemplate,
			}
		)

		g.By("1) Create first OperatorGroup")
		og.create(oc, itName, dr)

		g.By("2) Create second OperatorGroup")
		og1.create(oc, itName, dr)

		g.By("3) Check OperatorGroup Status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "MultipleOperatorGroupsFound", ok, []string{"og", og.name, "-n", ns, "-o=jsonpath={.status.conditions..reason}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "MultipleOperatorGroupsFound", ok, []string{"og", og1.name, "-n", ns, "-o=jsonpath={.status.conditions..reason}"}).check(oc)

		g.By("4) Delete second OperatorGroup")
		og1.delete(itName, dr)

		g.By("5) Check OperatorGroup status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"og", og.name, "-n", ns, "-o=jsonpath={.status.conditions..reason}"}).check(oc)

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-42972-OperatorGroup status should indicate if the SA named in spec not found", func() {
		var (
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSAtemplate        = filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
			sa                  = "scoped-42972"
		)

		oc.SetupProject()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		ns := oc.Namespace()
		dr.addIr(itName)

		var (
			og = operatorGroupDescription{
				name:               "og-42972",
				namespace:          ns,
				template:           ogSAtemplate,
				serviceAccountName: sa,
			}
		)

		g.By("1) Create first OperatorGroup")
		og.create(oc, itName, dr)

		g.By("2) Check OperatorGroup Status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "ServiceAccountNotFound", ok, []string{"og", og.name, "-n", ns, "-o=jsonpath={.status.conditions..reason}"}).check(oc)

		g.By("3) Check Service Account")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4) Check OperatorGroup status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"og", og.name, "-n", ns, "-o=jsonpath={.status.conditions..reason}"}).check(oc)

	})
	// author: jiazha@redhat.com
	g.It("Author:jiazha-ConnectedOnly-Medium-33902-Catalog Weighting", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")

		// the priority ranking is bucket-test1 > bucket-test2 > community-operators(-400 default)
		csObjects := []struct {
			name     string
			address  string
			priority int
		}{
			{"ocs-cs", "quay.io/olmqe/ocs-index:4.3.0", 0},
			{"bucket-test1", "quay.io/olmqe/bucket-index:1.0.0", 20},
			{"bucket-test2", "quay.io/olmqe/bucket-index:1.0.0", -1},
		}

		// create the openshift-storage project
		project := projectDescription{
			name: "openshift-storage",
		}

		// create the OperatorGroup resource
		og := operatorGroupDescription{
			name:      "test-og-33902",
			namespace: "openshift-storage",
			template:  ogSingleTemplate,
		}

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		for i, v := range csObjects {
			g.By(fmt.Sprintf("%d) start to create the %s CatalogSource", i+1, v.name))
			cs := catalogSourceDescription{
				name:        v.name,
				namespace:   "openshift-marketplace",
				displayName: "Priority Test",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     v.address,
				template:    csImageTemplate,
				priority:    v.priority,
			}
			cs.create(oc, itName, dr)
			newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status.connectionState.lastObservedState}"}).check(oc)
		}

		g.By("4) create the openshift-storage project")
		project.createwithCheck(oc, itName, dr)

		g.By("5) create the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		g.By("6) start to subscribe to the OCS operator")
		sub := subscriptionDescription{
			subName:                "ocs-sub",
			namespace:              "openshift-storage",
			catalogSourceName:      "ocs-cs",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "4.3.0",
			ipApproval:             "Automatic",
			operatorPackage:        "ocs-operator",
			singleNamespace:        true,
			template:               subTemplate,
		}
		sub.create(oc, itName, dr)

		g.By("7) check the dependce operator's subscription")
		depSub := subscriptionDescription{
			subName:                "lib-bucket-provisioner-4.3.0-bucket-test1-openshift-marketplace",
			namespace:              "openshift-storage",
			catalogSourceName:      "bucket-test1",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "4.3.0",
			ipApproval:             "Automatic",
			operatorPackage:        "lib-bucket-provisioner",
			singleNamespace:        true,
			template:               subTemplate,
		}
		// The dependence is lib-bucket-provisioner-4.3.0, it should from the bucket-test1 CatalogSource since its priority is the highest.
		dr.getIr(itName).add(newResource(oc, "sub", depSub.subName, requireNS, depSub.namespace))
		depSub.findInstalledCSV(oc, itName, dr)

		g.By(fmt.Sprintf("8) Remove subscription:%s, %s", sub.subName, depSub.subName))
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		depSub.delete(itName, dr)
		depSub.getCSV().delete(itName, dr)

		for _, v := range csObjects {
			g.By(fmt.Sprintf("9) Remove the %s CatalogSource", v.name))
			cs := catalogSourceDescription{
				name:        v.name,
				namespace:   "openshift-marketplace",
				displayName: "Priority Test",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     v.address,
				template:    csImageTemplate,
				priority:    v.priority,
			}
			cs.delete(itName, dr)
		}

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-24771-OLM should support for user defined ServiceAccount for OperatorGroup", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		saRoles := filepath.Join(buildPruningBaseDir, "scoped-sa-roles.yaml")
		oc.SetupProject()
		namespace := oc.Namespace()
		ogSAtemplate := filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		csv := "etcdoperator.v0.9.4"
		sa := "scoped-24771"

		// create the namespace
		project := projectDescription{
			name: namespace,
		}

		// create the OperatorGroup resource
		og := operatorGroupDescription{
			name:               "test-og-24771",
			namespace:          namespace,
			serviceAccountName: sa,
			template:           ogSAtemplate,
		}

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		g.By("1) Create the namespace")
		project.createwithCheck(oc, itName, dr)

		g.By("2) Create the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		g.By("3) Create the service account")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4) Create a Subscription")
		sub := subscriptionDescription{
			subName:                "etcd",
			namespace:              namespace,
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        true,
			template:               subTemplate,
			startingCSV:            csv,
		}
		sub.createWithoutCheck(oc, itName, dr)

		g.By("5) The install plan is Failed")
		installPlan := getResourceNoEmpty(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace, "-o=jsonpath={.items..metadata.name}")
		newCheck("expect", asAdmin, withoutNamespace, compare, "InstallComponentFailed", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions..reason}"}).check(oc)

		g.By("6) Grant the proper permissions to the service account")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", saRoles, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("7) Recreate the Subscription")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)

		g.By("8) Checking the state of CSV")
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", csv, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-24772-OLM should support for user defined ServiceAccount for OperatorGroup with fine grained permission", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		saRoles := filepath.Join(buildPruningBaseDir, "scoped-sa-fine-grained-roles.yaml")
		oc.SetupProject()
		namespace := oc.Namespace()
		ogSAtemplate := filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		csv := "etcdoperator.v0.9.4"
		sa := "scoped-24772"

		// create the namespace
		project := projectDescription{
			name: namespace,
		}

		// create the OperatorGroup resource
		og := operatorGroupDescription{
			name:               "test-og-24772",
			namespace:          namespace,
			serviceAccountName: sa,
			template:           ogSAtemplate,
		}

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		g.By("1) Create the namespace")
		project.createwithCheck(oc, itName, dr)

		g.By("2) Create the OperatorGroup")
		og.createwithCheck(oc, itName, dr)

		g.By("3) Create the service account")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4) Create a Subscription")
		sub := subscriptionDescription{
			subName:                "etcd",
			namespace:              namespace,
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        true,
			template:               subTemplate,
			startingCSV:            csv,
		}
		sub.createWithoutCheck(oc, itName, dr)

		g.By("5) The install plan is Failed")
		installPlan := getResourceNoEmpty(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace, "-o=jsonpath={.items..metadata.name}")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Failed", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("6) Grant the proper permissions to the service account")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", saRoles, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("7) Recreate the Subscription")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)

		g.By("8) Checking the state of CSV")
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", csv, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-24886-OLM should support for user defined ServiceAccount permission changes", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		saRoles := filepath.Join(buildPruningBaseDir, "scoped-sa-etcd.yaml")
		oc.SetupProject()
		namespace := oc.Namespace()
		ogTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		ogSAtemplate := filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		csv := "etcdoperator.v0.9.4"
		sa := "scoped-24886"

		// create the namespace
		project := projectDescription{
			name: namespace,
		}

		// create the OperatorGroup resource
		og := operatorGroupDescription{
			name:      "test-og-24886",
			namespace: namespace,
			template:  ogTemplate,
		}

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		g.By("1) Create the namespace")
		project.createwithCheck(oc, itName, dr)

		g.By("2) Create the OperatorGroup without service account")
		og.createwithCheck(oc, itName, dr)

		g.By("3) Create a Subscription")
		sub := subscriptionDescription{
			subName:                "etcd",
			namespace:              namespace,
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        true,
			template:               subTemplate,
			startingCSV:            csv,
		}
		sub.create(oc, itName, dr)

		g.By("4) Checking the state of CSV")
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", csv, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("5) Delete the Operator Group")
		og.delete(itName, dr)

		// create the OperatorGroup resource
		ogSA := operatorGroupDescription{
			name:               "test-og-24886",
			namespace:          namespace,
			serviceAccountName: sa,
			template:           ogSAtemplate,
		}
		g.By("6) Create the OperatorGroup with service account")
		ogSA.createwithCheck(oc, itName, dr)

		g.By("7) Create the service account")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("9) Grant the proper permissions to the service account")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", saRoles, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("10) Recreate the Subscription")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		g.By("11) Checking the state of CSV")
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", csv, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

	})
	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Medium-30765-Operator-version based dependencies metadata", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")

		oc.SetupProject()
		g.By("1) Start to create the CatalogSource CR")
		cs := catalogSourceDescription{
			name:        "prometheus-dependency-cs",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE",
			publisher:   "OLM QE",
			sourceType:  "grpc",
			address:     "quay.io/olmqe/etcd-prometheus-dependency-index:11.0",
			template:    csImageTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		defer cs.delete(itName, dr)
		cs.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("2) Install the OperatorGroup in a random project")

		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-30765",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("3) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-30765",
			namespace:              oc.Namespace(),
			catalogSourceName:      "prometheus-dependency-cs",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd-service-monitor",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("4) Assert that prometheus dependency is resolved")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("prometheus"))

	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Medium-27680-OLM Bundle support for Prometheus Types", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")

		oc.SetupProject()
		g.By("Start to create the CatalogSource CR")
		cs := catalogSourceDescription{
			name:        "prometheus-dependency1-cs",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE",
			publisher:   "OLM QE",
			sourceType:  "grpc",
			address:     "quay.io/olmqe/etcd-prometheus-dependency-index:11.0",
			template:    csImageTemplate,
		}
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		defer cs.delete(itName, dr)
		cs.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("Start to subscribe the Etcd operator")

		g.By("1) Install the OperatorGroup in a random project")

		oc.SetupProject()
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-27680",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-27680",
			namespace:              oc.Namespace(),
			catalogSourceName:      "prometheus-dependency1-cs",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd-service-monitor",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Assert that prometheus dependency is resolved")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("prometheus"))

		g.By("Assert that ServiceMonitor is created")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("ServiceMonitor", "my-servicemonitor", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("my-servicemonitor"))

		g.By("Assert that PrometheusRule is created")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("PrometheusRule", "my-prometheusrule", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("my-prometheusrule"))

	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Medium-24916-Operators in AllNamespaces should be granted namespace list", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		g.By("Start to subscribe the Camel-k operator")
		sub := subscriptionDescription{
			subName:                "camel-k",
			namespace:              "openshift-operators",
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "stable-1.5",
			ipApproval:             "Automatic",
			operatorPackage:        "camel-k",
			singleNamespace:        false,
			startingCSV:            "camel-k-operator.v1.5.0",
			template:               subTemplate,
		}

		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		g.By("check if camel-k is already installed")
		csvList := getResource(oc, asAdmin, withNamespace, "csv", "-o=jsonpath={.items[*].metadata.name}")
		e2e.Logf("CSV list %s ", csvList)
		if !strings.Contains("camel-k", csvList) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "list", "namespaces").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(msg).To(o.ContainSubstring("system:serviceaccount:openshift-operators:camel-k-operator"))
		} else {
			e2e.Failf("Not able to install Camel-K Operator")
		}
	})
	// author: jiazha@redhat.com
	g.It("Author:jiazha-High-32559-catalog operator crashed", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csImageTemplate := filepath.Join(buildPruningBaseDir, "cs-without-image.yaml")
		csTypes := []struct {
			name        string
			csType      string
			expectedMSG string
		}{
			{"cs-noimage", "grpc", "image and address unset"},
			{"cs-noimage-cm", "configmap", "configmap name unset"},
		}
		for _, t := range csTypes {
			g.By(fmt.Sprintf("test the %s type CatalogSource", t.csType))
			cs := catalogSourceDescription{
				name:        t.name,
				namespace:   "openshift-marketplace",
				displayName: "OLM QE",
				publisher:   "OLM QE",
				sourceType:  t.csType,
				template:    csImageTemplate,
			}
			dr := make(describerResrouce)
			itName := g.CurrentGinkgoTestDescription().TestText
			dr.addIr(itName)
			cs.create(oc, itName, dr)

			err := wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-marketplace", "catalogsource", cs.name, "-o=jsonpath={.status.message}").Output()
				if err != nil {
					e2e.Logf("Fail to get CatalogSource: %s, error: %s and try again", cs.name, err)
					return false, nil
				}
				if strings.Contains(output, t.expectedMSG) {
					e2e.Logf("Get expected message: %s", t.expectedMSG)
					return true, nil
				}
				return false, nil
			})

			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("catsrc of openshift-marketplace does not contain %v", t.expectedMSG))

			status, err := oc.AsAdmin().Run("get").Args("-n", "openshift-operator-lifecycle-manager", "pods", "-l", "app=catalog-operator", "-o=jsonpath={.items[0].status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if status != "Running" {
				e2e.Failf("The status of the CatalogSource: %s pod is: %s", cs.name, status)
			}
		}

		//destroy the two CatalogSource CRs
		for _, t := range csTypes {
			_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", "openshift-marketplace", "catalogsource", t.name).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Critical-22070-support grpc sourcetype [Serial]", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		csTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		oc.SetupProject()

		g.By("Start to create the CatalogSource CR")
		cs := catalogSourceDescription{
			name:        "cs-22070",
			namespace:   "openshift-marketplace",
			displayName: "OLM QE",
			publisher:   "OLM QE",
			sourceType:  "grpc",
			// use the quay.io/openshifttest/etcd-index:auto as index image
			address:  "quay.io/openshifttest/etcd-index@sha256:026e78179e1d790b465569f3630677d5698457131b1dfbba110b3d2a912f537c",
			template: csTemplate,
		}

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
		defer cs.delete(itName, dr)
		cs.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", cs.name, "-n", cs.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("Start to subscribe this etcd operator")
		sub := subscriptionDescription{
			subName:                "sub-22070",
			namespace:              "openshift-operators",
			catalogSourceName:      "cs-22070",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "clusterwide-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        false,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Assert that etcd dependency is resolved")
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("0.9.4-clusterwide"))
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Medium-21130-Fetching non-existent `PackageManifest` should return 404", func() {
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "--all-namespaces", "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		packageserverLines := strings.Split(msg, "\n")
		if len(packageserverLines) > 0 {
			raw, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "a_package_that_not_exists", "-o yaml", "--loglevel=8").Output()
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(raw).To(o.ContainSubstring("\"code\": 404"))
		} else {
			e2e.Failf("No packages to evaluate if 404 works when a PackageManifest does not exists")
		}
	})

	// author: bandrade@redhat.com
	g.It("Author:bandrade-Low-24057-Have terminationMessagePolicy defined as FallbackToLogsOnError", func() {
		labels := [...]string{"app=packageserver", "app=catalog-operator", "app=olm-operator"}
		for _, l := range labels {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o=jsonpath={range .items[*].spec}{.containers[*].name}{\"\t\"}", "-n", "openshift-operator-lifecycle-manager", "-l", l).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			amountOfContainers := len(strings.Split(msg, "\t"))
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-o=jsonpath={range .items[*].spec}{.containers[*].terminationMessagePolicy}{\"\t\"}", "-n", "openshift-operator-lifecycle-manager", "-l", l).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			regexp := regexp.MustCompile("FallbackToLogsOnError")
			amountOfContainersWithFallbackToLogsOnError := len(regexp.FindAllStringIndex(msg, -1))
			o.Expect(amountOfContainers).To(o.Equal(amountOfContainersWithFallbackToLogsOnError))
			if amountOfContainers != amountOfContainersWithFallbackToLogsOnError {
				e2e.Failf("OLM does not have all containers definied with FallbackToLogsOnError terminationMessagePolicy")
			}
		}
	})

	g.It("ConnectedOnly-Author:bandrade-High-40317-Check CatalogSources index images", func() {
		clusterVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "version", "-o=jsonpath={.status.history..version}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		cs := [...]string{"certified-operators", "community-operators", "redhat-operators"}

		for _, v := range cs {
			msgCertified, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", v, "-o=jsonpath={.spec.image}", "-n", "openshift-marketplace").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			splittedCertifiedVersion := strings.Split(msgCertified, ":")[1]
			certifiedVersion := splittedCertifiedVersion[1:]
			o.Expect(clusterVersion).To(o.ContainSubstring(certifiedVersion))

		}
	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-High-32613-Operators won't install if the CSV dependency is already installed", func() {

		namespace := "kogito"
		infinispanPackage := CreateSubscriptionSpecificNamespace("infinispan", oc, true, true, namespace, INSTALLPLAN_AUTOMATIC_MODE)
		CheckDeployment(infinispanPackage, oc)
		keycloakPackage := CreateSubscriptionSpecificNamespace("keycloak-operator", oc, false, false, namespace, INSTALLPLAN_AUTOMATIC_MODE)
		CheckDeployment(keycloakPackage, oc)
		kogitoPackage := CreateSubscriptionSpecificNamespace("kogito-operator", oc, false, false, namespace, INSTALLPLAN_AUTOMATIC_MODE)
		CheckDeployment(kogitoPackage, oc)
		RemoveOperatorDependencies(kogitoPackage, oc, false)
		RemoveNamespace(namespace, oc)

	})

	// author: bandrade@redhat.com
	g.It("ConnectedOnly-Author:bandrade-Low-24055-Check for defaultChannel mandatory field when having multiple channels", func() {
		olmBaseDir := exutil.FixturePath("testdata", "olm")
		cmMapWithoutDefaultChannel := filepath.Join(olmBaseDir, "configmap-without-defaultchannel.yaml")
		cmMapWithDefaultChannel := filepath.Join(olmBaseDir, "configmap-with-defaultchannel.yaml")
		csNamespaced := filepath.Join(olmBaseDir, "catalogsource-namespace.yaml")

		namespace := "scenario3"
		defer RemoveNamespace(namespace, oc)
		g.By("1) Creating a namespace")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("ns", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2) Creating a ConfigMap without a default channel")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", cmMapWithoutDefaultChannel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("3) Creating a CatalogSource")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("-f", csNamespaced).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("4) Checking CatalogSource error statement due to the absense of a default channel")
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l olm.catalogSource=scenario3", "-n", "scenario3").Output()
			if err != nil {
				return false, nil
			}
			if strings.Contains(output, "CrashLoopBackOff") {

				return true, nil
			}
			return false, nil
		})

		exutil.AssertWaitPollNoErr(err, "pod of olm.catalogSource=scenario3 is not CrashLoopBackOff")

		g.By("5) Changing the CatalogSource to include default channel for each package")
		_, err = oc.WithoutNamespace().AsAdmin().Run("apply").Args("-f", cmMapWithDefaultChannel).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("6) Checking the state of CatalogSource(Running)")
		err = wait.Poll(30*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-l olm.catalogSource=scenario3", "-n", "scenario3").Output()
			if err != nil {
				return false, nil
			}
			if strings.Contains(output, "Running") {

				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "pod of olm.catalogSource=scenario3 is not running")
	})

	// author: jiazha@redhat.com
	g.It("Author:jiazha-Medium-20981-contain the source commit id", func() {
		sameCommit := ""
		subPods := []string{"catalog-operator", "olm-operator", "packageserver"}

		for _, v := range subPods {
			podName, err := oc.AsAdmin().Run("get").Args("-n", "openshift-operator-lifecycle-manager", "pods", "-l", fmt.Sprintf("app=%s", v), "-o=jsonpath={.items[0].metadata.name}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("get pod name:%s", podName)

			g.By(fmt.Sprintf("get olm version from the %s pod", v))
			commands := []string{"-n", "openshift-operator-lifecycle-manager", "exec", podName, "--", "olm", "--version"}
			olmVersion, err := oc.AsAdmin().Run(commands...).Args().Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			idSlice := strings.Split(olmVersion, ":")
			gitCommitID := strings.TrimSpace(idSlice[len(idSlice)-1])
			e2e.Logf("olm source git commit ID:%s", gitCommitID)
			if len(gitCommitID) != 40 {
				e2e.Failf(fmt.Sprintf("the length of the git commit id is %d, != 40", len(gitCommitID)))
			}

			if sameCommit == "" {
				sameCommit = gitCommitID
				g.By("checking this commitID in https://github.com/openshift/operator-framework-olm repo")
				ctx, tc := githubClient()
				client := github.NewClient(tc)
				// OLM downstream repo has been changed to: https://github.com/openshift/operator-framework-olm
				_, _, err := client.Git.GetCommit(ctx, "openshift", "operator-framework-olm", gitCommitID)
				if err != nil {
					e2e.Failf("Git.GetCommit returned error: %v", err)
				}
				o.Expect(err).NotTo(o.HaveOccurred())

			} else if gitCommitID != sameCommit {
				e2e.Failf("These commitIDs inconformity!!!")
			}
		}
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-30312-can allow admission webhook definitions in CSV", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		validatingCsv := filepath.Join(buildPruningBaseDir, "validatingwebhook-csv.yaml")

		g.By("create new namespace")
		newNamespace := "test-operators-30312"
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", newNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", newNamespace).Execute()

		g.By("create operatorGroup")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=test-operator", fmt.Sprintf("NAMESPACE=%s", newNamespace)).OutputToFile("config-30312.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create csv")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", validatingCsv, "-p", fmt.Sprintf("NAMESPACE=%s", newNamespace), "OPERATION=CREATE").OutputToFile("config-30312.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("validatingwebhookconfiguration", "-l", "olm.owner.namespace=test-operators-30312").Execute()
			if err != nil {
				e2e.Logf("The validatingwebhookconfiguration is not created:%v", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("validatingwebhookconfiguration which owner ns %s is not created", "test-operators-30312"))

		g.By("update csv")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", validatingCsv, "-p", fmt.Sprintf("NAMESPACE=%s", newNamespace), "OPERATION=DELETE").OutputToFile("config-30312.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		validatingwebhookName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("validatingwebhookconfiguration", "-l", "olm.owner.namespace=test-operators-30312", "-o=jsonpath={.items[0].metadata.name}").Output()
		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("validatingwebhookconfiguration", validatingwebhookName, "-o=jsonpath={..operations}").Output()
			e2e.Logf(output)
			if err != nil {
				e2e.Logf("DELETE operations cannot be found:%v", err)
				return false, nil
			}
			if strings.Contains(output, "DELETE") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("validatingwebhookconfiguration %s has no DELETE operation", validatingwebhookName))
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-30317-can allow mutating admission webhook definitions in CSV", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		mutatingCsv := filepath.Join(buildPruningBaseDir, "mutatingwebhook-csv.yaml")

		g.By("create new namespace")
		newNamespace := "test-operators-30317"
		err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", newNamespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", newNamespace).Execute()

		g.By("create operatorGroup")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=test-operator", fmt.Sprintf("NAMESPACE=%s", newNamespace)).OutputToFile("config-30317.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create csv")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", mutatingCsv, "-p", fmt.Sprintf("NAMESPACE=%s", newNamespace), "OPERATION=CREATE").OutputToFile("config-30317.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", "-l", "olm.owner.namespace=test-operators-30317").Execute()
			if err != nil {
				e2e.Logf("The mutatingwebhookconfiguration is not created:%v", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("mutatingwebhookconfiguration which owner ns %s is not created", "test-operators-30317"))

		g.By("Start to test 30374")
		g.By("update csv")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", mutatingCsv, "-p", fmt.Sprintf("NAMESPACE=%s", newNamespace), "OPERATION=DELETE").OutputToFile("config-30317.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		mutatingwebhookName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", "-l", "olm.owner.namespace=test-operators-30317", "-o=jsonpath={.items[0].metadata.name}").Output()
		err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("mutatingwebhookconfiguration", mutatingwebhookName, "-o=jsonpath={..operations}").Output()
			e2e.Logf(output)
			if err != nil {
				e2e.Logf("DELETE operations cannot be found:%v", err)
				return false, nil
			}
			if strings.Contains(output, "DELETE") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("mutatingwebhookconfiguration %s has no DELETE operation", mutatingwebhookName))
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-30319-Admission Webhook Configuration names should be unique", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		operatorGroup := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		validatingCsv := filepath.Join(buildPruningBaseDir, "validatingwebhook-csv.yaml")

		var validatingwebhookName1, validatingwebhookName2 string
		for i := 1; i < 3; i++ {
			istr := strconv.Itoa(i)
			g.By("create new namespace")
			newNamespace := "test-operators-30319-"
			newNamespace += istr
			err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", newNamespace).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", newNamespace).Execute()

			g.By("create operatorGroup")
			configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=test-operator", fmt.Sprintf("NAMESPACE=%s", newNamespace)).OutputToFile("config-30319.json")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("create csv")
			configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", validatingCsv, "-p", fmt.Sprintf("NAMESPACE=%s", newNamespace), "OPERATION=CREATE").OutputToFile("config-30319.json")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = wait.Poll(20*time.Second, 180*time.Second, func() (bool, error) {
				err := oc.AsAdmin().WithoutNamespace().Run("get").Args("validatingwebhookconfiguration", "-l", fmt.Sprintf("olm.owner.namespace=%s", newNamespace)).Execute()
				if err != nil {
					e2e.Logf("The validatingwebhookconfiguration is not created:%v", err)
					return false, nil
				}
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, fmt.Sprintf("validatingwebhookconfiguration which owner namespace %s is not created", newNamespace))

			validatingwebhookName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("validatingwebhookconfiguration", "-l", fmt.Sprintf("olm.owner.namespace=%s", newNamespace), "-o=jsonpath={.items[0].metadata.name}").Output()
			if i == 1 {
				validatingwebhookName1 = validatingwebhookName
			}
			if i == 2 {
				validatingwebhookName2 = validatingwebhookName
			}
		}
		if validatingwebhookName1 != validatingwebhookName2 {
			e2e.Logf("The test case pass")
		} else {
			err := "The test case fail"
			o.Expect(err).NotTo(o.HaveOccurred())
		}
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-24738-CRD should update if previously defined schemas do not change [Disruptive]", func() {
		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var cfgMap = filepath.Join(buildPruningBaseDir, "configmap-etcd.yaml")
		var patchCfgMap = filepath.Join(buildPruningBaseDir, "configmap-ectd-alpha-beta.yaml")
		var catSource = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
		var og = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var etcdCluster = filepath.Join(buildPruningBaseDir, "etcd-cluster.yaml")
		var operatorWait = 150 * time.Second

		g.By("check precondition and prepare env")
		if isPresentResource(oc, asAdmin, withoutNamespace, present, "crd", "etcdclusters.etcd.database.coreos.com") && isPresentResource(oc, asAdmin, withoutNamespace, present, "EtcdCluster", "-A") {
			e2e.Logf("It is distruptive case and the resources exists, do not destory it. exit")
			return
		}
		oc.AsAdmin().Run("delete").Args("crd", "etcdclusters.etcd.database.coreos.com").Output()
		oc.AsAdmin().Run("delete").Args("crd", "etcdbackups.etcd.database.coreos.com").Output()
		oc.AsAdmin().Run("delete").Args("crd", "etcdrestores.etcd.database.coreos.com").Output()

		defer oc.AsAdmin().Run("delete").Args("ns", "test-automation-24738").Execute()
		defer oc.AsAdmin().Run("delete").Args("ns", "test-automation-24738-1").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", "installed-community-24738-global-operators", "-n", "openshift-marketplace").Execute()
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("catalogsource", "installed-community-24738-global-operators", "-n", "openshift-marketplace").Execute()

		g.By("create new namespace")
		var err = oc.AsAdmin().Run("create").Args("ns", "test-automation-24738").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create ConfigMap")
		createCfgMap, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", cfgMap, "-p", "NAME=installed-community-24738-global-operators", "NAMESPACE=openshift-marketplace").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("TEST1********** %v", createCfgMap)

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createCfgMap, "-n", "openshift-marketplace").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create CatalogSource")
		createCatSource, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", catSource, "-p", "NAME=installed-community-24738-global-operators", "NAMESPACE=openshift-marketplace", "ADDRESS=installed-community-24738-global-operators", "DISPLAYNAME=Community Operators", "PUBLISHER=Community", "SOURCETYPE=internal").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("TEST2oc get ********** %v", createCatSource)

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createCatSource, "-n", "openshift-marketplace").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createOg, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", og, "-p", "NAME=test-operators-og", "NAMESPACE=test-automation-24738").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(60*time.Second, operatorWait, func() (bool, error) {
			checkCatSource, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", "installed-community-24738-global-operators", "-n", "openshift-marketplace", "-o", "jsonpath={.status.connectionState.lastObservedState}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			//o.Expect(checkCatSource).To(o.Equal("READY"))
			if checkCatSource == "READY" {
				e2e.Logf("Installed catalogsource")
				return true, nil
			} else {
				e2e.Logf("FAIL - Installed catalogsource ")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "installed-community-24738-global-operators is not READY")

		createImgSub, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", Sub, "-p", "SUBNAME=etcd-etcdoperator.v0.9.2", "SUBNAMESPACE=test-automation-24738", "CHANNEL=alpha", "APPROVAL=Automatic", "OPERATORNAME=etcd-update", "SOURCENAME=installed-community-24738-global-operators", "SOURCENAMESPACE=openshift-marketplace", "STARTINGCSV=etcdoperator.v0.9.2").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			oc.AsAdmin().Run("delete").Args("crd", "etcdclusters.etcd.database.coreos.com").Output()
			oc.AsAdmin().Run("delete").Args("crd", "etcdbackups.etcd.database.coreos.com").Output()
			oc.AsAdmin().Run("delete").Args("crd", "etcdrestores.etcd.database.coreos.com").Output()
		}()
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createImgSub).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(60*time.Second, operatorWait, func() (bool, error) {
			checknameCsv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "etcdoperator.v0.9.2", "-n", "test-automation-24738", "-o", "jsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checknameCsv)
			if checknameCsv == "Succeeded" {
				e2e.Logf("CSV Installed ")
				return true, nil
			} else {
				e2e.Logf("CSV not installed  ")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "etcdoperator.v0.9.2 is not installed")

		createEtcdCluster, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", etcdCluster, "-p", "NAME=example", "NAMESPACE=test-automation-24738").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createEtcdCluster).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, operatorWait, func() (bool, error) {
			checkCreateEtcdCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "test-automation-24738").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checkCreateEtcdCluster)

			lines := strings.Split(checkCreateEtcdCluster, "\n")
			var count = 0
			for _, line := range lines {
				e2e.Logf(line)
				if strings.Contains(line, "example") {
					count++
				}
				e2e.Logf(line)
			}
			if count == 3 {
				e2e.Logf("EtcCluster Create Installed ")
				return true, nil
			} else {
				e2e.Logf("EtcCluster Not Installed ")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "test-automation-24738 EtcCluster is not installed")

		g.By("create new namespace")
		err = oc.AsAdmin().Run("create").Args("ns", "test-automation-24738-1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createOg1, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", og, "-p", "NAME=test-operators-og", "NAMESPACE=test-automation-24738-1").OutputToFile("cconfig-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createImgSub1, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", Sub, "-p", "SUBNAME=etcd-etcdoperator.v0.9.2", "SUBNAMESPACE=test-automation-24738-1", "CHANNEL=alpha", "APPROVAL=Automatic", "OPERATORNAME=etcd-update", "SOURCENAME=installed-community-24738-global-operators", "SOURCENAMESPACE=openshift-marketplace", "STARTINGCSV=etcdoperator.v0.9.2").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createImgSub1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createEtcdCluster1, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", etcdCluster, "-p", "NAME=example", "NAMESPACE=test-automation-24738-1").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createEtcdCluster1).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(30*time.Second, operatorWait, func() (bool, error) {
			checkCreateEtcdCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "test-automation-24738-1").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checkCreateEtcdCluster)

			lines := strings.Split(checkCreateEtcdCluster, "\n")
			var count = 0
			for _, line := range lines {
				e2e.Logf(line)
				if strings.Contains(line, "example") {
					count++
				}
				e2e.Logf(line)
			}
			if count == 3 {
				e2e.Logf("EtcCluster Create Installed ")
				return true, nil
			} else {
				e2e.Logf("EtcCluster Not Installed ")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "test-automation-24738-1 EtcCluster is not installed")

		g.By("update ConfigMap")
		createPatchCfgMap, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", patchCfgMap, "-p", "NAME=installed-community-24738-global-operators", "NAMESPACE=openshift-marketplace").OutputToFile("config-24738.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("TEST1********** %v", createPatchCfgMap)

		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", createPatchCfgMap, "-n", "openshift-marketplace").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		patchIP, err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("sub", "etcd-etcdoperator.v0.9.2", "-n", "test-automation-24738-1", "--type=json", "-p", "[{\"op\": \"replace\" , \"path\" : \"/spec/channel\", \"value\":beta}]").Output()
		e2e.Logf(patchIP)
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(patchIP).To(o.ContainSubstring("patched"))

		err = wait.Poll(30*time.Second, operatorWait, func() (bool, error) {
			checkCreateEtcdCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", "-n", "test-automation-24738-1").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checkCreateEtcdCluster)

			lines := strings.Split(checkCreateEtcdCluster, "\n")
			var count = 0
			for _, line := range lines {
				e2e.Logf(line)
				if strings.Contains(line, "install") {
					count++
				}
				e2e.Logf(line)
			}
			if count == 2 {
				e2e.Logf("Ip Channel created")
				return true, nil
			} else {
				e2e.Logf("Ip Channel NOT created")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "test-automation-24738-1 Ip Channel NOT created")

		err = wait.Poll(60*time.Second, operatorWait, func() (bool, error) {
			checknameCsv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "etcdoperator.v0.9.4", "-n", "test-automation-24738-1", "-o", "jsonpath={.status.phase}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checknameCsv)
			if checknameCsv == "Succeeded" {
				e2e.Logf("CSV Installed ")
				return true, nil
			} else {
				e2e.Logf("CSV not installed  ")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "test-automation-24738-1 CSV not installed")

	})

	// author: scolange@redhat.com
	// only community operator ready for the disconnected env now
	g.It("ConnectedOnly-Author:scolange-Medium-32862-Pods found with invalid container images not present in release payload", func() {

		g.By("Verify the version of marketplace_operator")
		pods, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-marketplace", "--no-headers").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		lines := strings.Split(pods, "\n")
		for _, line := range lines {
			e2e.Logf("line: %v", line)
			if strings.Contains(line, "certified-operators") || strings.Contains(line, "community-operators") || strings.Contains(line, "marketplace-operator") || strings.Contains(line, "redhat-marketplace") || strings.Contains(line, "redhat-operators") && strings.Contains(line, "1/1") {
				name := strings.Split(line, " ")
				checkRel, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(name[0], "-n", "openshift-marketplace", "--", "cat", "/etc/redhat-release").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(checkRel).To(o.ContainSubstring("Red Hat"))
			}
		}

	})

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-42041-Available=False despite unavailableReplicas <= maxUnavailable", func() {
		_, err1 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("csv", "packageserver", "-n", "openshift-operator-lifecycle-manager",
			"--type=json", "--patch", "[{\"op\": \"add\",\"path\": \"/spec/install/spec/deployments/0/spec/template/metadata/annotations\", \"value\": { \"custom.csv\": \"custom csv value\"} }]").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		maxUnavailable, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.spec.strategy.rollingUpdate.maxUnavailable}").Output()
		e2e.Logf(maxUnavailable)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(maxUnavailable).To(o.Equal("1"))

		maxSurge, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.spec.strategy.rollingUpdate.maxSurge}").Output()
		e2e.Logf(maxSurge)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(maxSurge).To(o.Equal("1"))
	})

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-42068-Available condition set to false on any Deployment spec change", func() {
		available, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator", "operator-lifecycle-manager-packageserver", "-o=jsonpath={.status.conditions[1].type}").Output()
		e2e.Logf(available)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(available).To(o.Equal("Available"))

		statusAvailable, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusteroperator", "operator-lifecycle-manager-packageserver", "-o=jsonpath={.status.conditions[1].status}").Output()
		e2e.Logf(statusAvailable)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(statusAvailable).To(o.Equal("True"))
	})

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-42069-component not found log should be debug level", func() {

		var since = "--since=60s"
		var snooze time.Duration = 90
		var tail = "--tail=10"

		oc.SetupProject()
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-42069",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-42069",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}

		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)

		nameIP := sub.getIP(oc)
		deteleIP, err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("installplan", nameIP, "-n", oc.Namespace()).Output()
		e2e.Logf(deteleIP)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(deteleIP).To(o.ContainSubstring("deleted"))

		catPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=olm-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catPodname).NotTo(o.BeEmpty())

		waitErr := wait.Poll(3*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", tail, since).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "component not found") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "compoent found in log of pod of openshift-operator-lifecycle-manager")

	})

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-42073-deployment sets neither CPU or memory request on the packageserver container", func() {
		cpu, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={..containers..resources.requests.cpu}").Output()
		e2e.Logf(cpu)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(cpu).NotTo(o.Equal(""))

		memory, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "packageserver", "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={..containers..resources.requests.memory}").Output()
		e2e.Logf(memory)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(memory).NotTo(o.Equal(""))

		catPodnames, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=packageserver", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catPodnames).NotTo(o.BeEmpty())

		lines := strings.Split(catPodnames, " ")
		for _, line := range lines {
			e2e.Logf("line: %v", line)

			pkg1Cpu, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", line, "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.spec..resources.requests.cpu}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(pkg1Cpu).To(o.Equal(cpu))

			pkg1Memory, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", line, "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.spec..resources.requests.memory}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(pkg1Memory).To(o.Equal(memory))
		}
	})

	// Author: tbuskey@redhat.com, scolange@redhat.com
	g.It("Author:tbuskey-Medium-23673-Installplan can be created while Install and uninstall operators via Marketplace for 5 times [Slow]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			finalCSV            = ""
			err                 error
			exists              bool
			i                   int
			msgCsv              string
			msgSub              string
			waitErr             error
		)

		oc.SetupProject()

		var (
			og = operatorGroupDescription{
				name:      "23673",
				namespace: oc.Namespace(),
				template:  ogTemplate,
			}
			sub = subscriptionDescription{
				subName:                "prometheus-23673",
				namespace:              oc.Namespace(),
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				ipApproval:             "Automatic",
				channel:                "beta",
				operatorPackage:        "prometheus",
				startingCSV:            finalCSV,
				singleNamespace:        true,
				template:               subFile,
			}
		)

		dr := make(describerResrouce)
		dr.addIr(itName)

		g.By("1, check if this operator ready for installing")
		e2e.Logf("Check if %v exists in the %v catalog", sub.operatorPackage, sub.catalogSourceName)
		exists, err = clusterPackageExists(oc, sub)
		if !exists {
			e2e.Failf("FAIL:PackageMissing %v does not exist in catalog %v", sub.operatorPackage, sub.catalogSourceName)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("2, Create og")
		og.create(oc, itName, dr)

		g.By("3, Subscribe to operator prometheus")
		sub.create(oc, itName, dr)
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)

		// grab the installedCSV and use as startingCSV
		finalCSV, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace(), sub.subName, "-o", "jsonpath={.status.installedCSV}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(finalCSV).NotTo(o.BeEmpty())
		sub.startingCSV = finalCSV

		g.By("4 Unsubscribe to operator prometheus")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		msgSub, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace()).Output()
		msgCsv, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace()).Output()
		if !strings.Contains(msgSub, "No resources found") && (!strings.Contains(msgCsv, "No resources found") || strings.Contains(msgCsv, finalCSV)) {
			e2e.Failf("Cycle #1 subscribe/unsubscribe failed %v:\n%v \n%v \n", err, msgSub, msgCsv)
		}

		g.By("5, subscribe/unsubscribe to operator prometheus 4 more times")
		for i = 2; i < 6; i++ {
			e2e.Logf("Cycle #%v starts", i)

			// g.By("subscribe")
			sub.create(oc, itName, dr)
			newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", finalCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

			// g.By("unsubscribe")
			msgCsv, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("csv", "-n", oc.Namespace(), sub.installedCSV).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			// sub.deleteCSV(itName, dr) // this doesn't seem to work for multiple cycles
			sub.delete(itName, dr)
			// Need to ensure its deleted before proceeding
			waitErr = wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
				msgSub, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace()).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				msgCsv, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace()).Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if strings.Contains(msgSub, "No resources found") && (strings.Contains(msgCsv, "No resources found") || !strings.Contains(msgCsv, finalCSV)) {
					return true, nil
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(waitErr, "sub or csv found")
		}

		g.By("6 FINISH")
		i--
		e2e.Logf("Finished %v subscribe & unsubscribe cycles\n\n", i)
	})

	// author: scolange@redhat.com
	g.It("Author:scolange-Medium-24586-Prevent Operator Conflicts in OperatorHub", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var catSource = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var og = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("catalogsource", "test-prometer-24586", "-n", "openshift-marketplace").Execute()
		defer oc.AsAdmin().Run("delete").Args("ns", "test24586").Execute()

		g.By("create new namespace")
		var err = oc.AsAdmin().Run("create").Args("ns", "test24586").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create new catalogsource")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", catSource, "-p", "NAME=test-prometer-24586",
			"NAMESPACE=openshift-marketplace", "ADDRESS=quay.io/olmqe/mktplc-367", "DISPLAYNAME=prometer-24586", "PUBLISHER=QE", "SOURCETYPE=grpc").OutputToFile("config-24586.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createOg, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", og, "-p", "NAME=test-operators-og", "NAMESPACE=test24586").OutputToFile("config-24586.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createImgSub, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", Sub, "-p", "SUBNAME=prometheus", "SUBNAMESPACE=test24586",
			"CHANNEL=alpha", "APPROVAL=Automatic", "OPERATORNAME=prometheus", "SOURCENAME=community-operators", "SOURCENAMESPACE=openshift-marketplace").OutputToFile("config-24586.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", createImgSub).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		createImgSub2, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", Sub, "-p", "SUBNAME=prometheus2", "SUBNAMESPACE=test24586",
			"CHANNEL=alpha", "APPROVAL=Automatic", "OPERATORNAME=prometheus", "SOURCENAME=test-prometer-24586", "SOURCENAMESPACE=openshift-marketplace").OutputToFile("config-24586.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", createImgSub2).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

	})

	// author: scolange@redhat.com OCP-40316
	g.It("ConnectedOnly-Author:scolange-Medium-40316-OLM enters infinite loop if Pending CSV replaces itself [Serial]", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var operatorGroup = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		var pkgServer = filepath.Join(buildPruningBaseDir, "packageserver.yaml")
		//var operatorWait = 180 * time.Second
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", "test40316").Execute()

		g.By("create new namespace")
		var err = oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", "test40316").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("create new OperatorGroup")
		ogFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", operatorGroup, "-p", "NAME=test-operator", "NAMESPACE=test40316").OutputToFile("config-40316.json")
		o.Expect(err).NotTo(o.HaveOccurred())

		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", ogFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", pkgServer, "-p", "NAME=packageserver", "NAMESPACE=test40316").OutputToFile("config-40316.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		statusCsv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", "test40316").Output()
		e2e.Logf("CSV prometheus %v", statusCsv)
		o.Expect(err).NotTo(o.HaveOccurred())

		pods, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", "openshift-operator-lifecycle-manager").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(pods)

		lines := strings.Split(pods, "\n")
		for _, line := range lines {
			e2e.Logf("line: %v", line)
			if strings.Contains(line, "olm-operator") {
				name := strings.Split(line, " ")
				checkRel, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("top", "pods", name[0], "-n", "openshift-operator-lifecycle-manager").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				linesTop := strings.Split(checkRel, "\n")
				for _, lineTop := range linesTop {
					if strings.Contains(lineTop, name[0]) {
						cpuOutput := strings.Split(strings.TrimSpace(lineTop), " ")[1]
						cpu := strings.Split(cpuOutput, "m")[0]
						if cpu > "98" {
							e2e.Logf("cpu: %v", cpu)
							e2e.Failf("CPU Limit usage is more the 99%: %v", checkRel)
						}
					}

				}

			}
		}
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-24075-The couchbase packagemanifest labels provider value should not be MongoDB Inc ", func() {
		NameCouchBase, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", "couchbase-enterprise-certified", "-o", "jsonpath={.status.provider.name}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(NameCouchBase).To(o.Equal("Couchbase"))
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-41283-Marketplace extract container request CPU or memory", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var og1 = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		var operatorWait = 150 * time.Second

		oc.SetupProject()
		namespace := oc.Namespace()

		/*
			createOg, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", og, "-p", "NAME=test-operators-og", "NAMESPACE=test41283").OutputToFile("config-41283.json")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", createOg).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		*/

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		og := operatorGroupDescription{
			name:      "test-operators-og",
			namespace: namespace,
			template:  og1,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("Verify inside the jobs the value of spec.containers[].resources.requests field are setted")

		/*
			createImgSub, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", Sub, "-p", "SUBNAME=couchbase", "SUBNAMESPACE=test41283",
				"CHANNEL=stable", "APPROVAL=Automatic", "OPERATORNAME=couchbase-enterprise-certified", "SOURCENAME=certified-operators", "SOURCENAMESPACE=openshift-marketplace").OutputToFile("config-41283.json")
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", createImgSub).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		*/

		sub := subscriptionDescription{
			subName:                "couchbase",
			namespace:              namespace,
			catalogSourceName:      "certified-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "stable",
			ipApproval:             "Automatic",
			operatorPackage:        "couchbase-enterprise-certified",
			singleNamespace:        true,
			template:               Sub,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		err := wait.Poll(60*time.Second, operatorWait, func() (bool, error) {
			checknameCsv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("jobs", "-n", "openshift-marketplace", "-o", "jsonpath={.items[*].spec.template.spec.containers[*].resources.requests.cpu}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(checknameCsv)
			if checknameCsv == "" {
				e2e.Logf("jobs KO Limit not setted ")
				return false, nil
			} else {
				e2e.Logf("jobs OK Limit setted ")
				return true, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "jobs KO Limit not setted")

	})

	g.It("ConnectedOnly-Author:scolange-Medium-21534-Check OperatorGroups on console", func() {
		ogNamespace, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "global-operators", "-n", "openshift-operators", "-o", "jsonpath={.status.namespaces}").Output()
		e2e.Logf(ogNamespace)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(ogNamespace).To(o.Equal("[\"\"]"))

	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-24587-Add InstallPlan conditions to Subscription status", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var og1 = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

		oc.SetupProject()
		namespace := oc.Namespace()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		og := operatorGroupDescription{
			name:      "test-operators-og",
			namespace: namespace,
			template:  og1,
		}
		og.createwithCheck(oc, itName, dr)

		sub := subscriptionDescription{
			subName:                "couchbase",
			namespace:              namespace,
			catalogSourceName:      "certified-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "stable",
			ipApproval:             "Manual",
			operatorPackage:        "couchbase-enterprise-certified",
			singleNamespace:        true,
			template:               Sub,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		// the InstallPlan should Manual on sub
		newCheck("expect", asAdmin, withoutNamespace, compare, "Manual", ok, []string{"sub", "-n", namespace, "-o=jsonpath={.items[*].spec.installPlanApproval}"}).check(oc)

		// the InstallPlan should Manual on ip
		newCheck("expect", asAdmin, withoutNamespace, compare, "Manual", ok, []string{"installplan", sub.getIP(oc), "-n", sub.namespace, "-o=jsonpath={.spec.approval}"}).check(oc)

		// the InstallPlan patched
		patchIP, err2 := oc.AsAdmin().WithoutNamespace().Run("patch").Args("installplan", sub.getIP(oc), "-n", namespace, "--type=merge", "-p", "{\"spec\":{\"approved\": true}}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(patchIP).To(o.ContainSubstring("patched"))

		// the InstallPlan should be approved on sub
		newCheck("expect", asAdmin, withoutNamespace, compare, "AtLatestKnown", ok, []string{"sub", "-n", namespace, "-o=jsonpath={.items[*].status.state}"}).check(oc)

		// the delete InstallPlan
		deteleIP, err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("installplan", sub.getIP(oc), "-n", namespace).Output()
		e2e.Logf(deteleIP)
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(deteleIP).To(o.ContainSubstring("deleted"))

		// the InstallPlan should InstallPlanMissing on sub
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlanMissing", ok, []string{"sub", "-n", namespace, "-o=jsonpath={.items[*].status.conditions[*].type}"}).check(oc)

	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-41565-Resolution fails to sort channel if inner entry does not satisfy predicate", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var og1 = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

		oc.SetupProject()
		namespace := oc.Namespace()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		og := operatorGroupDescription{
			name:      "test-operators-og",
			namespace: namespace,
			template:  og1,
		}
		og.createwithCheck(oc, itName, dr)

		sub := subscriptionDescription{
			subName:                "hive-operator",
			namespace:              namespace,
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "hive-operator",
			singleNamespace:        true,
			template:               Sub,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		e2e.Logf("Check 1 operator")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		sub = subscriptionDescription{
			subName:                "assisted-service-operator",
			namespace:              namespace,
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "assisted-service-operator",
			singleNamespace:        true,
			template:               Sub,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		e2e.Logf("Check 2 operator")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		e2e.Logf("Check event in failed")
		eventOutput, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", namespace).Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(eventOutput).NotTo(o.ContainSubstring("Failed"))

	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-25674-restart the marketplace-operator when the cluster is in bad state [Disruptive]", func() {

		var buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
		var Sub = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		var og1 = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")

		oc.SetupProject()
		namespace := oc.Namespace()
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		og := operatorGroupDescription{
			name:      "test-operators-og",
			namespace: namespace,
			template:  og1,
		}
		og.createwithCheck(oc, itName, dr)

		sub := subscriptionDescription{
			subName:                "jagerk",
			namespace:              namespace,
			catalogSourceName:      "kaka",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "",
			singleNamespace:        true,
			template:               Sub,
		}

		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)

		e2e.Logf("Check 1 first")
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.items[*].spec.name}"}).check(oc)

		g.By("get pod of marketplace")
		podName := getResource(oc, asAdmin, withoutNamespace, "pod", "--selector=name=marketplace-operator", "-n", "openshift-marketplace", "-o=jsonpath={...metadata.name}")
		o.Expect(podName).NotTo(o.BeEmpty())

		g.By("delete pod of marketplace")
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "pod", podName, "-n", "openshift-marketplace")
		o.Expect(err).NotTo(o.HaveOccurred())

		exec.Command("bash", "-c", "sleep 10").Output()

		g.By("pod of marketplace restart")
		newCheck("expect", asAdmin, withoutNamespace, compare, "TrueFalseFalse", ok, []string{"clusteroperator", "marketplace",
			"-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"}).check(oc)

	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-High-23172-the copied CSV will exist in new created project", func() {

		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")

		sub := subscriptionDescription{
			subName:                "sub-etcd-23172",
			namespace:              "openshift-operators",
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "clusterwide-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			singleNamespace:        false,
			template:               subTemplate,
		}

		g.By("1, Check if the global operator global-operators support all namesapces")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[]", ok, []string{"og", "global-operators", "-n", "openshift-operators", "-o=jsonpath={.status.namespaces}"})

		g.By("2, Create operator targeted at all namespace")
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		g.By("3, Create new namespace")
		oc.SetupProject()

		e2e.Logf("The test case pass")

		g.By("4, Check the csv within new namespace is copied.")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
		e2e.Logf("The t**************")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Copied", ok, []string{"csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.reason}"})
	})

	// author: scolange@redhat.com
	g.It("ConnectedOnly-Author:scolange-Medium-23395-Deleted catalog registry pods and verify if them are recreated automatically [Disruptive]", func() {

		g.By("get pod of marketplace")
		podName := getResource(oc, asAdmin, withoutNamespace, "pod", "--selector=olm.catalogSource=redhat-operators", "-n", "openshift-marketplace", "-o=jsonpath={...metadata.name}")
		o.Expect(podName).NotTo(o.BeEmpty())

		g.By("delete pod of marketplace")
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "pod", podName, "-n", "openshift-marketplace")
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(10*time.Second, 180*time.Second, func() (bool, error) {
			res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "--selector=olm.catalogSource=redhat-operators", "-o=jsonpath={.items..status.phase}", "-n", "openshift-marketplace").Output()
			if strings.Contains(res, "Running") {
				return true, nil
			}
			return false, nil
		})
	})

	// author: jiazha@redhat.c
	g.It("Author:jiazha-Medium-21126-OLM Subscription status says CSV is installed when it is not", func() {
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-21126",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Manual approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-21126",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Manual",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)
		g.By("3) Check the etcdoperator v0.9.4 related resources")
		// the installedCSV should be NULL
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"sub", "sub-21126", "-n", oc.Namespace(), "-o=jsonpath={.status.installedCSV}"}).check(oc)
		// the state should be UpgradePending
		newCheck("expect", asAdmin, withoutNamespace, compare, "UpgradePending", ok, []string{"sub", "sub-21126", "-n", oc.Namespace(), "-o=jsonpath={.status.state}"}).check(oc)
		// the InstallPlan should not approved
		newCheck("expect", asAdmin, withoutNamespace, compare, "false", ok, []string{"installplan", sub.getIP(oc), "-n", oc.Namespace(), "-o=jsonpath={.spec.approved}"}).check(oc)
		// should no etcdoperator.v0.9.4 CSV found
		msg, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "etcdoperator.v0.9.4", "-n", oc.Namespace()).Output()
		if !strings.Contains(msg, "not found") {
			e2e.Failf("still found the etcdoperator.v0.9.4 in namespace:%s, msg:%v", oc.Namespace(), msg)
		}
	})
})

var _ = g.Describe("[sig-operators] OLM for an end user use", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-23440", exutil.KubeConfigPath())
	)

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-Low-24058-components should have resource limits defined", func() {
		olmUnlimited := 0
		olmNames := []string{""}
		olmNamespace := "openshift-operator-lifecycle-manager"
		olmJpath := "-o=jsonpath={range .items[*]}{@.metadata.name}{','}{@.spec.containers[0].resources.requests.*}{'\\n'}"
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", olmNamespace, olmJpath).Output()
		if err != nil {
			e2e.Failf("Unable to get pod -n %v %v.", olmNamespace, olmJpath)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No resources found"))
		lines := strings.Split(msg, "\n")
		for _, line := range lines {
			name := strings.Split(line, ",")
			// e2e.Logf("Line is %v, len %v, len name %v, name0 %v, name1 %v\n", line, len(line), len(name), name[0], name[1])
			if strings.Contains(line, "packageserver") {
				continue
			} else {
				if len(line) > 1 {
					if len(name) > 1 && len(name[1]) < 1 {
						olmUnlimited++
						olmNames = append(olmNames, name[0])
					}
				}
			}
		}
		if olmUnlimited > 0 && len(olmNames) > 0 {
			e2e.Failf("There are no limits set on %v of %v OLM components: %v", olmUnlimited, len(lines), olmNames)
		}
	})

})

var _ = g.Describe("[sig-operators] OLM for an end user handle common object", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-common-"+getRandomString(), exutil.KubeConfigPath())

		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
	})

	g.AfterEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.getIr(itName).cleanup()
		dr.rmIr(itName)
	})

	// It will cover test case: OCP-22259, author: kuiwang@redhat.com
	g.It("Author:kuiwang-Medium-22259-marketplace operator CR status on a running cluster [Exclusive]", func() {

		g.By("check marketplace status")
		newCheck("expect", asAdmin, withoutNamespace, compare, "TrueFalseFalse", ok, []string{"clusteroperator", "marketplace",
			"-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"}).check(oc)

		g.By("get pod of marketplace")
		podName := getResource(oc, asAdmin, withoutNamespace, "pod", "--selector=name=marketplace-operator", "-n", "openshift-marketplace",
			"-o=jsonpath={...metadata.name}")
		o.Expect(podName).NotTo(o.BeEmpty())

		g.By("delete pod of marketplace")
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "pod", podName, "-n", "openshift-marketplace")
		o.Expect(err).NotTo(o.HaveOccurred())

		exec.Command("bash", "-c", "sleep 10").Output()

		g.By("pod of marketplace restart")
		newCheck("expect", asAdmin, withoutNamespace, compare, "TrueFalseFalse", ok, []string{"clusteroperator", "marketplace",
			"-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}{.status.conditions[?(@.type==\"Progressing\")].status}{.status.conditions[?(@.type==\"Degraded\")].status}"}).check(oc)
	})

	// It will cover test case: OCP-24076, author: kuiwang@redhat.com
	g.It("ProdrunBoth-StagerunBoth-Author:kuiwang-Medium-24076-check the version of olm operator is appropriate in ClusterOperator", func() {
		var (
			olmClusterOperatorName = "operator-lifecycle-manager"
		)

		g.By("get the version of olm operator")
		olmVersion := getResource(oc, asAdmin, withoutNamespace, "clusteroperator", olmClusterOperatorName, "-o=jsonpath={.status.versions[?(@.name==\"operator\")].version}")
		o.Expect(olmVersion).NotTo(o.BeEmpty())

		g.By("Check if it is appropriate in ClusterOperator")
		newCheck("expect", asAdmin, withoutNamespace, compare, olmVersion, ok, []string{"clusteroperator", fmt.Sprintf("-o=jsonpath={.items[?(@.metadata.name==\"%s\")].status.versions[?(@.name==\"operator\")].version}", olmClusterOperatorName)}).check(oc)
	})

	// It will cover test case: OCP-29775 and OCP-29786, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-29775-Medium-29786-as oc user on linux to mirror catalog image", func() {
		var (
			bundleIndex1         = "quay.io/kuiwang/operators-all:v1"
			bundleIndex2         = "quay.io/kuiwang/operators-dockerio:v1"
			operatorAllPath      = "operators-all-manifests-" + getRandomString()
			operatorDockerioPath = "operators-dockerio-manifests-" + getRandomString()
		)
		defer exec.Command("bash", "-c", "rm -fr ./"+operatorAllPath).Output()
		defer exec.Command("bash", "-c", "rm -fr ./"+operatorDockerioPath).Output()

		g.By("mirror to quay.io/kuiwang")
		output, err := oc.AsAdmin().WithoutNamespace().Run("adm", "catalog", "mirror").Args("--manifests-only", "--to-manifests="+operatorAllPath, bundleIndex1, "quay.io/kuiwang").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operators-all-manifests"))

		g.By("check mapping.txt")
		result, err := exec.Command("bash", "-c", "cat ./"+operatorAllPath+"/mapping.txt|grep -E \"atlasmap-atlasmap-operator:0.1.0|quay.io/kuiwang/jmckind-argocd-operator:[a-z0-9][a-z0-9]|redhat-cop-cert-utils-operator:latest\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("atlasmap-atlasmap-operator:0.1.0"))
		o.Expect(result).To(o.ContainSubstring("redhat-cop-cert-utils-operator:latest"))
		o.Expect(result).To(o.ContainSubstring("quay.io/kuiwang/jmckind-argocd-operator"))

		g.By("check icsp yaml")
		result, err = exec.Command("bash", "-c", "cat ./"+operatorAllPath+"/imageContentSourcePolicy.yaml | grep -E \"quay.io/kuiwang/strimzi-operator|docker.io/strimzi/operator$\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("- quay.io/kuiwang/strimzi-operator"))
		o.Expect(result).To(o.ContainSubstring("source: docker.io/strimzi/operator"))

		g.By("mirror to localhost:5000")
		output, err = oc.AsAdmin().WithoutNamespace().Run("adm", "catalog", "mirror").Args("--manifests-only", "--to-manifests="+operatorDockerioPath, bundleIndex2, "localhost:5000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operators-dockerio-manifests"))

		g.By("check mapping.txt to localhost:5000")
		result, err = exec.Command("bash", "-c", "cat ./"+operatorDockerioPath+"/mapping.txt|grep -E \"localhost:5000/atlasmap/atlasmap-operator:0.1.0|localhost:5000/strimzi/operator:[a-z0-9][a-z0-9]\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("localhost:5000/atlasmap/atlasmap-operator:0.1.0"))
		o.Expect(result).To(o.ContainSubstring("localhost:5000/strimzi/operator"))

		g.By("check icsp yaml to localhost:5000")
		result, err = exec.Command("bash", "-c", "cat ./"+operatorDockerioPath+"/imageContentSourcePolicy.yaml | grep -E \"localhost:5000/strimzi/operator|docker.io/strimzi/operator$\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("- localhost:5000/strimzi/operator"))
		o.Expect(result).To(o.ContainSubstring("source: docker.io/strimzi/operator"))
		o.Expect(result).NotTo(o.ContainSubstring("docker.io/atlasmap/atlasmap-operator"))
	})

	// It will cover test case: OCP-33452, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-33452-oc adm catalog mirror does not mirror the index image itself", func() {
		var (
			bundleIndex1 = "quay.io/olmqe/olm-api@sha256:71cfd4deaa493d31cd1d8255b1dce0fb670ae574f4839c778f2cfb1bf1f96995"
			manifestPath = "manifests-olm-api-" + getRandomString()
		)
		defer exec.Command("bash", "-c", "rm -fr ./"+manifestPath).Output()

		g.By("mirror to localhost:5000/test")
		output, err := oc.AsAdmin().WithoutNamespace().Run("adm", "catalog", "mirror").Args("--manifests-only", "--to-manifests="+manifestPath, bundleIndex1, "localhost:5000/test").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("manifests-olm-api"))

		g.By("check mapping.txt to localhost:5000")
		result, err := exec.Command("bash", "-c", "cat ./"+manifestPath+"/mapping.txt|grep -E \"quay.io/olmqe/olm-api\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("quay.io/olmqe/olm-api"))

		g.By("check icsp yaml to localhost:5000")
		result, err = exec.Command("bash", "-c", "cat ./"+manifestPath+"/imageContentSourcePolicy.yaml | grep -E \"quay.io/olmqe/olm-api\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("quay.io/olmqe/olm-api"))
	})

	// It will cover test case: OCP-21825, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-21825-Certs for packageserver can be rotated successfully", func() {
		var (
			packageserverName = "packageserver"
		)

		g.By("Get certsRotateAt and APIService name")
		resources := strings.Fields(getResource(oc, asAdmin, withoutNamespace, "csv", packageserverName, "-n", "openshift-operator-lifecycle-manager", fmt.Sprintf("-o=jsonpath={.status.certsRotateAt}{\" \"}{.status.requirementStatus[?(@.kind==\"%s\")].name}", "APIService")))
		o.Expect(resources).NotTo(o.BeEmpty())
		apiServiceName := resources[1]
		certsRotateAt, err := time.Parse(time.RFC3339, resources[0])
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get caBundle")
		caBundle := getResource(oc, asAdmin, withoutNamespace, "apiservices", apiServiceName, "-o=jsonpath={.spec.caBundle}")
		o.Expect(caBundle).NotTo(o.BeEmpty())

		g.By("Change caBundle")
		patchResource(oc, asAdmin, withoutNamespace, "apiservices", apiServiceName, "-p", fmt.Sprintf("{\"spec\":{\"caBundle\":\"test%s\"}}", caBundle))

		g.By("Check updated certsRotataAt")
		err = wait.Poll(3*time.Second, 150*time.Second, func() (bool, error) {
			updatedCertsRotateAt, err := time.Parse(time.RFC3339, getResource(oc, asAdmin, withoutNamespace, "csv", packageserverName, "-n", "openshift-operator-lifecycle-manager", "-o=jsonpath={.status.certsRotateAt}"))
			if err != nil {
				e2e.Logf("the get error is %v, and try next", err)
				return false, nil
			}
			if !updatedCertsRotateAt.After(certsRotateAt) {
				e2e.Logf("wait update, and try next")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("csv %s cert is not updated", packageserverName))

		newCheck("expect", asAdmin, withoutNamespace, contain, "redhat-operators", ok, []string{"packagemanifest", fmt.Sprintf("--selector=catalog=%s", "redhat-operators"), "-o=jsonpath={.items[*].status.catalogSource}"}).check(oc)

	})

})

var _ = g.Describe("[sig-operators] OLM for an end user handle within a namespace", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-a-"+getRandomString(), exutil.KubeConfigPath())
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
	})

	g.AfterEach(func() {})

	// It will cover test case: OCP-29231 and OCP-29277, author: kuiwang@redhat.com
	g.It("Author:kuiwang-Medium-29231-Medium-29277-label to target namespace of group", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			og1                 = operatorGroupDescription{
				name:      "og1-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			og2 = operatorGroupDescription{
				name:      "og2-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og1.namespace = oc.Namespace()
		og2.namespace = oc.Namespace()

		g.By("Create og1 and check the label of target namespace of og1 is created")
		og1.create(oc, itName, dr)
		og1Uid := getResource(oc, asAdmin, withNamespace, "og", og1.name, "-o=jsonpath={.metadata.uid}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+og1Uid, ok,
			[]string{"ns", og1.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+og1Uid, nok,
			[]string{"ns", "openshift-operators", "-o=jsonpath={.metadata.labels}"}).check(oc)

		g.By("Delete og1 and check the label of target namespace of og1 is removed")
		og1.delete(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+og1Uid, nok,
			[]string{"ns", og1.namespace, "-o=jsonpath={.metadata.labels}"}).check(oc)

		g.By("Create og2 and recreate og1 and check the label")
		og2.create(oc, itName, dr)
		og2Uid := getResource(oc, asAdmin, withNamespace, "og", og2.name, "-o=jsonpath={.metadata.uid}")
		og1.create(oc, itName, dr)
		og1Uid = getResource(oc, asAdmin, withNamespace, "og", og1.name, "-o=jsonpath={.metadata.uid}")
		labelNs := getResource(oc, asAdmin, withoutNamespace, "ns", og1.namespace, "-o=jsonpath={.metadata.labels}")
		o.Expect(labelNs).To(o.ContainSubstring(og2Uid))
		o.Expect(labelNs).To(o.ContainSubstring(og1Uid))

		//OCP-29277
		g.By("Check no label of global operator group ")
		globalOgUID := getResource(oc, asAdmin, withoutNamespace, "og", "global-operators", "-n", "openshift-operators", "-o=jsonpath={.metadata.uid}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+globalOgUID, nok,
			[]string{"ns", "default", "-o=jsonpath={.metadata.labels}"}).check(oc)

	})

	// It will cover test case: OCP-23170, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-23170-API labels should be hash", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create operator")
		sub.create(oc, itName, dr)

		g.By("Check the API labes should be hash")
		apiLabels := getResource(oc, asUser, withNamespace, "csv", sub.installedCSV, "-o=jsonpath={.metadata.labels}")
		o.Expect(len(apiLabels)).NotTo(o.BeZero())

		for _, v := range strings.Split(strings.Trim(apiLabels, "{}"), ",") {
			if strings.Contains(v, "olm.api") {
				hash := strings.Trim(strings.Split(strings.Split(v, ":")[0], ".")[2], "\"")
				match, err := regexp.MatchString(`^[a-fA-F0-9]{16}$|^[a-fA-F0-9]{15}$`, hash)
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(match).To(o.BeTrue())
			}
		}
	})

	// It will cover test case: OCP-20979, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-20979-only one IP is generated", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create operator")
		sub.create(oc, itName, dr)
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check there is only one ip")
		ips := getResource(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace, "--no-headers")
		ipList := strings.Split(ips, "\n")
		for _, ip := range ipList {
			name := strings.Fields(ip)[0]
			getResource(oc, asAdmin, withoutNamespace, "installplan", name, "-n", sub.namespace, "-o=json")
		}
		o.Expect(strings.Count(ips, sub.installedCSV)).To(o.Equal(1))
	})

	// It will cover test case: OCP-25757 and 22656, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-25757-High-22656-manual approval strategy apply to subsequent releases", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("prepare for manual approval")
		sub.ipApproval = "Manual"
		sub.startingCSV = "windup-operator.0.0.4"

		g.By("Create Sub which apply manual approve install plan")
		sub.create(oc, itName, dr)

		g.By("the install plan is RequiresApproval")
		installPlan := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installplan.name}")
		o.Expect(installPlan).NotTo(o.BeEmpty())
		newCheck("expect", asAdmin, withoutNamespace, compare, "RequiresApproval", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("manually approve sub")
		sub.approve(oc, itName, dr)

		g.By("the target CSV is created with upgrade")
		o.Expect(strings.Compare(sub.installedCSV, sub.startingCSV) != 0).To(o.BeTrue())
	})

	g.It("ConnectedOnly-Author:bandrade-Critical-41026-OCS should only one installplan generated when creating subscription", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "ocs-operator",
				namespace:              "",
				ipApproval:             "Automatic",
				operatorPackage:        "ocs-operator",
				catalogSourceName:      "redhat-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create operator")
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Check there is only one ip")
		ips := getResource(oc, asAdmin, withoutNamespace, "installplan", "-n", sub.namespace, "--no-headers")
		ipList := strings.Split(ips, "\n")
		for _, ip := range ipList {
			name := strings.Fields(ip)[0]
			getResource(oc, asAdmin, withoutNamespace, "installplan", name, "-n", sub.namespace, "-o=json")
		}
		o.Expect(strings.Count(ips, sub.installedCSV)).To(o.Equal(1))
	})

	// It will cover test case: OCP-24438, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-24438-check subscription CatalogSource Status", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}

			catsrc = catalogSourceDescription{
				name:        "catsrc-test-operator",
				namespace:   "",
				displayName: "Test Catsrc Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "",
				template:    catsrcImageTemplate,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		catsrc.namespace = oc.Namespace()
		sub.catalogSourceName = catsrc.name
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("create sub with the above catalogsource")
		sub.createWithoutCheck(oc, itName, dr)

		g.By("check its condition is UnhealthyCatalogSourceFound")
		newCheck("expect", asUser, withoutNamespace, contain, "UnhealthyCatalogSourceFound", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions[*].reason}"}).check(oc)

		g.By("create catalogsource")
		imageAddress := getResource(oc, asAdmin, withoutNamespace, "catsrc", "community-operators", "-n", "openshift-marketplace", "-o=jsonpath={.spec.image}")
		o.Expect(imageAddress).NotTo(o.BeEmpty())
		catsrc.address = imageAddress
		catsrc.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "READY", ok, []string{"catsrc", catsrc.name, "-n", catsrc.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)

		g.By("check its condition is AllCatalogSourcesHealthy and csv is created")
		newCheck("expect", asUser, withoutNamespace, contain, "AllCatalogSourcesHealthy", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions[*].reason}"}).check(oc)
		sub.findInstalledCSV(oc, itName, dr)
	})

	// It will cover test case: OCP-24027, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-24027-can create and delete catalogsource and sub repeatedly", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-test-operator",
				namespace:   "",
				displayName: "Test Catsrc Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "",
				template:    catsrcImageTemplate,
			}
			repeatedCount = 2
			og            = ogD
			sub           = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		catsrc.namespace = oc.Namespace()
		sub.catalogSourceName = catsrc.name
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Get address of catalogsource and name")
		imageAddress := getResource(oc, asAdmin, withoutNamespace, "catsrc", "community-operators", "-n", "openshift-marketplace", "-o=jsonpath={.spec.image}")
		o.Expect(imageAddress).NotTo(o.BeEmpty())
		catsrc.address = imageAddress

		for i := 0; i < repeatedCount; i++ {
			g.By("Create Catalogsource")
			catsrc.create(oc, itName, dr)
			newCheck("expect", asUser, withoutNamespace, compare, "READY", ok, []string{"catsrc", catsrc.name, "-n", catsrc.namespace, "-o=jsonpath={.status..lastObservedState}"}).check(oc)

			g.By("Create sub with the above catalogsource")
			sub.create(oc, itName, dr)
			newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}"}).check(oc)

			g.By("Remove catalog and sub")
			sub.delete(itName, dr)
			sub.deleteCSV(itName, dr)
			catsrc.delete(itName, dr)
			if i < repeatedCount-1 {
				time.Sleep(20 * time.Second)
			}
		}
	})

	// It will cover part of test case: OCP-21404, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-21404-csv will be RequirementsNotMet after sa is delete", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create operator")
		sub.create(oc, itName, dr)
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("Get SA of csv")
		sa := newSa(getResource(oc, asUser, withNamespace, "csv", sub.installedCSV, "-o=jsonpath={.status.requirementStatus[?(@.kind==\"ServiceAccount\")].name}"), sub.namespace)

		g.By("Delete sa of csv")
		sa.getDefinition(oc)
		sa.delete(oc)
		newCheck("expect", asUser, withNamespace, compare, "RequirementsNotMet", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.reason}"}).check(oc)

		g.By("Recovery sa of csv")
		sa.reapply(oc)
		newCheck("expect", asUser, withNamespace, compare, "Succeeded+2+Installing", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// It will cover part of test case: OCP-21404, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-21404-csv will be RequirementsNotMet after role rule is delete", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			ogD                 = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			subD = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			og  = ogD
			sub = subD
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create operator")
		sub.create(oc, itName, dr)
		newCheck("expect", asUser, withNamespace, compare, "Succeeded"+"InstallSucceeded", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.status.phase}{.status.reason}"}).check(oc)

		g.By("Get SA of csv")
		sa := newSa(getResource(oc, asUser, withNamespace, "csv", sub.installedCSV, "-o=jsonpath={.status.requirementStatus[?(@.kind==\"ServiceAccount\")].name}"), sub.namespace)
		sa.checkAuth(oc, "yes", "Windup")

		g.By("Get Role of csv")
		role := newRole(getResource(oc, asUser, withNamespace, "role", "-n", sub.namespace, fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-o=jsonpath={.items[0].metadata.name}"), sub.namespace)
		origRules := role.getRules(oc)
		modifiedRules := role.getRulesWithDelete(oc, "windup.jboss.org")

		g.By("Remove rules")
		role.patch(oc, fmt.Sprintf("{\"rules\": %s}", modifiedRules))
		sa.checkAuth(oc, "no", "Windup")

		g.By("Recovery rules")
		role.patch(oc, fmt.Sprintf("{\"rules\": %s}", origRules))
		sa.checkAuth(oc, "yes", "Windup")
	})

	// It will cover test case: OCP-29723, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-29723-As cluster admin find abnormal status condition via components of operator resource", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-29723-operator",
				namespace:   "",
				displayName: "Test Catsrc 29723 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-api:v2",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "stable-5.x",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.4",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("delete catalog source")
		catsrc.delete(itName, dr)
		g.By("delete sa")
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "sa", "default", "-n", sub.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("check abnormal status")
		output := getResource(oc, asAdmin, withoutNamespace, "operator", sub.operatorPackage+"."+sub.namespace, "-o=json")
		o.Expect(output).NotTo(o.BeEmpty())

		output = getResource(oc, asAdmin, withoutNamespace, "operator", sub.operatorPackage+"."+sub.namespace,
			fmt.Sprintf("-o=jsonpath={.status.components.refs[?(@.name==\"%s\")].conditions[*].type}", sub.subName))
		o.Expect(output).To(o.ContainSubstring("CatalogSourcesUnhealthy"))

		newCheck("expect", asAdmin, withoutNamespace, contain, "RequirementsNotMet+2+InstallWaiting", ok, []string{"operator", sub.operatorPackage + "." + sub.namespace,
			fmt.Sprintf("-o=jsonpath={.status.components.refs[?(@.name==\"%s\")].conditions[*].reason}", sub.installedCSV)}).check(oc)
	})

	// It will cover test case: OCP-30762, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-30762-installs bundles with v1 CRDs", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-30762-operator",
				namespace:   "",
				displayName: "Test Catsrc 30762 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-api:v2",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "stable-5.x",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.4",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// It will cover test case: OCP-27683, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-27683-InstallPlans can install from extracted bundles", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-27683-operator",
				namespace:   "",
				displayName: "Test Catsrc 27683 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/mta-index:v0.0.5",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.5",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("get bundle package from ip")
		installPlan := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installplan.name}")
		o.Expect(installPlan).NotTo(o.BeEmpty())
		ipBundle := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.bundleLookups[0].path}")
		o.Expect(ipBundle).NotTo(o.BeEmpty())

		g.By("get bundle package from job")
		jobName := getResource(oc, asAdmin, withoutNamespace, "job", "-n", catsrc.namespace, "-o=jsonpath={.items[0].metadata.name}")
		o.Expect(jobName).NotTo(o.BeEmpty())
		jobBundle := getResource(oc, asAdmin, withoutNamespace, "pod", "-l", "job-name="+jobName, "-n", catsrc.namespace, "-o=jsonpath={.items[0].status.initContainerStatuses[*].image}")
		o.Expect(jobName).NotTo(o.BeEmpty())
		o.Expect(jobBundle).To(o.ContainSubstring(ipBundle))
	})

	// It will cover test case: OCP-24513, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-24513-Operator config support env only", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-24513-operator",
				namespace:   "",
				displayName: "Test Catsrc 24513 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:v1-crdarg",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "buildv2-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "buildv2-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "buildv2-operator.v0.3.0",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			opename = "build-operator"
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("get parameter of deployment")
		newCheck("expect", asAdmin, withoutNamespace, contain, "ARGS1", ok, []string{"deployment", opename, "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[0].command}"}).check(oc)

		g.By("patch env for sub")
		sub.patch(oc, "{\"spec\": {\"config\": {\"env\": [{\"name\": \"EMPTY_ENV\"},{\"name\": \"ARGS1\",\"value\": \"-v=4\"}]}}}")

		g.By("check the empty env")
		newCheck("expect", asAdmin, withoutNamespace, contain, "EMPTY_ENV", ok, []string{"deployment", opename, "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[0].env[*].name}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "-v=4", ok, []string{"deployment", opename, "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[0].env[*].value}"}).check(oc)
	})

	// It will cover test case: OCP-24382, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-24382-Should restrict CRD update if schema changes [Serial]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			etcdCluster         = filepath.Join(buildPruningBaseDir, "etcd-cluster.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-24382-operator",
				namespace:   "",
				displayName: "Test Catsrc 24382 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:vschema-crdv1",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "etcd",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "etcd",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "etcdoperator.v0.9.2",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			etcdCr = customResourceDescription{
				name:      "example-24382",
				namespace: "",
				typename:  "EtcdCluster",
				template:  etcdCluster,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace
		etcdCr.namespace = oc.Namespace()

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("creat cr")
		etcdCr.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Running", ok, []string{etcdCr.typename, etcdCr.name, "-n", etcdCr.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("update operator")
		sub.patch(oc, "{\"spec\": {\"channel\": \"beta\"}}")
		sub.findInstalledCSV(oc, itName, dr)

		g.By("check schema does not work")
		installPlan := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installplan.name}")
		o.Expect(installPlan).NotTo(o.BeEmpty())
		newCheck("expect", asAdmin, withoutNamespace, contain, "error validating existing CRs", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions[*].message}"}).check(oc)
	})

	// It will cover test case: OCP-25760, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-25760-Operator upgrades does not fail after change the channel", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-25760-operator",
				namespace:   "",
				displayName: "Test Catsrc 25760 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:vchannel-crdv1",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.4",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("switch channel")
		sub.patch(oc, "{\"spec\": {\"channel\": \"beta\"}}")
		sub.findInstalledCSV(oc, itName, dr)

		g.By("check csv of new channel")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// It will cover test case: OCP-35895, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-35895-can't install a CSV with duplicate roles", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-35895-operator",
				namespace:   "",
				displayName: "Test Catsrc 35895 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:vmtaduprol",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.5",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("check sa")
		newCheck("expect", asAdmin, withoutNamespace, contain, "windup-operator-haproxy", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={..serviceAccountName}"}).check(oc)
	})

	// It will cover test case: OCP-32863, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-32863-Support resources required for SAP Gardener Control Plane Operator [Disruptive]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			vpaTemplate         = filepath.Join(buildPruningBaseDir, "vpa-crd.yaml")
			crdVpa              = crdDescription{
				name:     "verticalpodautoscalers.autoscaling.k8s.io",
				template: vpaTemplate,
			}
			og = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-32863-operator",
				namespace:   "",
				displayName: "Test Catsrc 32863 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/single-bundle-index:pdb",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "busybox",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "busybox",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "busybox.v2.0.0",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		// defer crdVpa.delete(oc) //it is not needed in case it already exist
		if isPresentResource(oc, asAdmin, withoutNamespace, notPresent, "crd", crdVpa.name) {

			oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
			og.namespace = oc.Namespace()
			catsrc.namespace = oc.Namespace()
			sub.namespace = oc.Namespace()
			sub.catalogSourceNamespace = catsrc.namespace

			g.By("create vpa crd")
			crdVpa.create(oc, itName, dr)
			defer crdVpa.delete(oc)

			g.By("create catalog source")
			catsrc.create(oc, itName, dr)

			g.By("Create og")
			og.create(oc, itName, dr)

			g.By("install perator")
			sub.create(oc, itName, dr)

			g.By("check csv")
			newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

			g.By("check additional resources")
			newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"vpa", "busybox-vpa", "-n", sub.namespace}).check(oc)
			newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"PriorityClass", "super-priority", "-n", sub.namespace}).check(oc)
			newCheck("present", asAdmin, withoutNamespace, present, "", ok, []string{"PodDisruptionBudget", "busybox-pdb", "-n", sub.namespace}).check(oc)
		}
	})

	// It will cover test case: OCP-34472, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-34472-OLM label dependency", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "olm-1933-v8-catalog",
				namespace:   "",
				displayName: "OLM 1933 v8 Operator Catalog",
				publisher:   "QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:v9",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.5",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			dependentOperator = "buildv2-operator.v0.3.0"
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install perator")
		sub.create(oc, itName, dr)

		g.By("check if dependent operator is installed")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", dependentOperator, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// It will cover test case: OCP-37263, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-37263-Subscription stays in UpgradePending but InstallPlan not installing [Slow]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "olm-1860185-catalog",
				namespace:   "",
				displayName: "OLM 1860185 Catalog",
				publisher:   "QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:v1860185-v1",
				template:    catsrcImageTemplate,
			}
			subStrimzi = subscriptionDescription{
				subName:                "strimzi",
				namespace:              "",
				channel:                "strimzi-0.23.x",
				ipApproval:             "Automatic",
				operatorPackage:        "strimzi-kafka-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "strimzi-cluster-operator.v0.23.0",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			subBuildv2 = subscriptionDescription{
				subName:                "buildv2-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "buildv2-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "buildv2-operator.v0.3.0",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			subMta = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.5",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		subStrimzi.namespace = oc.Namespace()
		subStrimzi.catalogSourceNamespace = catsrc.namespace
		subBuildv2.namespace = oc.Namespace()
		subBuildv2.catalogSourceNamespace = catsrc.namespace
		subMta.namespace = oc.Namespace()
		subMta.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install Strimzi")
		subStrimzi.create(oc, itName, dr)

		g.By("check if Strimzi is installed")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", subStrimzi.installedCSV, "-n", subStrimzi.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("install Portworx")
		subMta.create(oc, itName, dr)

		g.By("check if Portworx is installed")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", subMta.installedCSV, "-n", subMta.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("get IP of Portworx")
		mtaIP := subMta.getIP(oc)

		g.By("Delete Portworx sub")
		subMta.delete(itName, dr)

		g.By("check if Portworx sub is Deleted")
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"sub", subMta.subName, "-n", subMta.namespace}).check(oc)

		g.By("Delete Portworx csv")
		csvPortworx := csvDescription{
			name:      subMta.installedCSV,
			namespace: subMta.namespace,
		}
		csvPortworx.delete(itName, dr)

		g.By("check if Portworx csv is Deleted")
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"csv", subMta.installedCSV, "-n", subMta.namespace}).check(oc)

		g.By("install Couchbase")
		subBuildv2.create(oc, itName, dr)

		g.By("get IP of Couchbase")
		couchbaseIP := subBuildv2.getIP(oc)

		g.By("it takes different IP")
		o.Expect(couchbaseIP).NotTo(o.Equal(mtaIP))

	})

	// It will cover test case: OCP-33176, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-33176-Enable generated operator component adoption for operators with single ns mode [Slow]", func() {
		var (
			itName                  = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir     = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate        = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catsrcImageTemplate     = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			apiserviceImageTemplate = filepath.Join(buildPruningBaseDir, "apiservice.yaml")
			apiserviceVersion       = "v33176"
			apiserviceName          = apiserviceVersion + ".foos.bar.com"
			og                      = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-33176-operator",
				namespace:   "",
				displayName: "Test Catsrc 33176 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-api:v3",
				template:    catsrcImageTemplate,
			}
			subEtcd = subscriptionDescription{
				subName:                "etcd33176",
				namespace:              "",
				channel:                "singlenamespace-alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "etcd",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "etcdoperator.v0.9.4", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
			}
			subCockroachdb = subscriptionDescription{
				subName:                "cockroachdb33176",
				namespace:              "",
				channel:                "stable-5.x",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.4", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
			}
		)

		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		subEtcd.namespace = oc.Namespace()
		subEtcd.catalogSourceNamespace = catsrc.namespace
		subCockroachdb.namespace = oc.Namespace()
		subCockroachdb.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install Etcd")
		subEtcd.create(oc, itName, dr)
		defer doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subEtcd.operatorPackage+"."+subEtcd.namespace)

		g.By("Check all resources via operators")
		newCheck("expect", asAdmin, withoutNamespace, contain, "ServiceAccount", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Role", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RoleBinding", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Subscription", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlan", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ClusterServiceVersion", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Deployment", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, subEtcd.namespace, ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].namespace}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

		g.By("delete operator and Operator still exists because of crd")
		subEtcd.delete(itName, dr)
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subEtcd.installedCSV, "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("reinstall etcd and check Operator")
		subEtcd.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

		g.By("delete etcd and the Operator")
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "sub", subEtcd.subName, "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subEtcd.installedCSV, "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subEtcd.operatorPackage+"."+subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("install etcd manually")
		subEtcd.ipApproval = "Manual"
		subEtcd.startingCSV = "etcdoperator.v0.9.4"
		subEtcd.installedCSV = ""
		subEtcd.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlan", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("approve etcd")
		subEtcd.approve(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ClusterServiceVersion", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, subEtcd.namespace, ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].namespace}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

		g.By("unlabel resource and it is relabeled automatically")
		roleName := getResource(oc, asAdmin, withoutNamespace, "operator", subEtcd.operatorPackage+"."+subEtcd.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='Role')].name}")
		o.Expect(roleName).NotTo(o.BeEmpty())
		_, err = doAction(oc, "label", asAdmin, withoutNamespace, "Role", roleName, "operators.coreos.com/"+subEtcd.operatorPackage+"."+subEtcd.namespace+"-", "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "Role", ok, []string{"operator", subEtcd.operatorPackage + "." + subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("delete etcd and the Operator again and Operator should recreated because of crd")
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "sub", subEtcd.subName, "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subEtcd.installedCSV, "-n", subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subEtcd.operatorPackage+"."+subEtcd.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		// here there is issue and take WA
		_, err = doAction(oc, "label", asAdmin, withoutNamespace, "crd", "etcdbackups.etcd.database.coreos.com", "operators.coreos.com/"+subEtcd.operatorPackage+"."+subEtcd.namespace+"-")
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = doAction(oc, "label", asAdmin, withoutNamespace, "crd", "etcdbackups.etcd.database.coreos.com", "operators.coreos.com/"+subEtcd.operatorPackage+"."+subEtcd.namespace+"=")
		o.Expect(err).NotTo(o.HaveOccurred())
		//done for WA
		var componentKind string
		err = wait.Poll(15*time.Second, 240*time.Second, func() (bool, error) {
			componentKind = getResource(oc, asAdmin, withoutNamespace, "operator", subEtcd.operatorPackage+"."+subEtcd.namespace, "-o=jsonpath={.status.components.refs[*].kind}")
			if strings.Contains(componentKind, "CustomResourceDefinition") {
				return true, nil
			}
			e2e.Logf("the got kind is %v", componentKind)
			return false, nil
		})
		if err != nil && strings.Compare(componentKind, "") != 0 {
			e2e.Failf("the operator has wrong component")
			// after the official is supported, will change it again.
		}

		g.By("install Cockroachdb")
		subCockroachdb.create(oc, itName, dr)
		defer doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subCockroachdb.operatorPackage+"."+subCockroachdb.namespace)

		g.By("Check all resources of Cockroachdb via operators")
		newCheck("expect", asAdmin, withoutNamespace, contain, "Role", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "RoleBinding", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Subscription", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlan", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "ClusterServiceVersion", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "Deployment", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, subCockroachdb.namespace, ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].namespace}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

		g.By("create ns test-33176 and label it")
		_, err = doAction(oc, "create", asAdmin, withoutNamespace, "ns", "test-33176")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer doAction(oc, "delete", asAdmin, withoutNamespace, "ns", "test-33176")
		_, err = doAction(oc, "label", asAdmin, withoutNamespace, "ns", "test-33176", "operators.coreos.com/"+subCockroachdb.operatorPackage+"."+subCockroachdb.namespace+"=")
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "Namespace", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("create apiservice and label it")
		err = applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", apiserviceImageTemplate, "-p", "NAME="+apiserviceName, "VERSION="+apiserviceVersion)
		o.Expect(err).NotTo(o.HaveOccurred())
		defer doAction(oc, "delete", asAdmin, withoutNamespace, "apiservice", apiserviceName)
		_, err = doAction(oc, "label", asAdmin, withoutNamespace, "apiservice", apiserviceName, "operators.coreos.com/"+subCockroachdb.operatorPackage+"."+subCockroachdb.namespace+"=")
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "APIService", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

	})

	// It will cover test case: OCP-39897, author: kuiwang@redhat.com
	//Set it as serial because it will delete CRD of teiid. It potential impact other cases if it is in parallel.
	g.It("ConnectedOnly-Author:kuiwang-Medium-39897-operator objects should not be recreated after all other associated resources have been deleted [Serial]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-39897-operator",
				namespace:   "",
				displayName: "Test Catsrc 39897 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/mta-index:v0.0.5",
				template:    catsrcImageTemplate,
			}
			subMta = subscriptionDescription{
				subName:                "mta-operator",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.5", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
			}
			crd = crdDescription{
				name: "windups.windup.jboss.org",
			}
		)

		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		subMta.namespace = oc.Namespace()
		subMta.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("install Teiid")
		subMta.create(oc, itName, dr)
		defer doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subMta.operatorPackage+"."+subMta.namespace)

		g.By("Check the resources via operators")
		newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subMta.operatorPackage + "." + subMta.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("delete operator and Operator still exists because of crd")
		subMta.delete(itName, dr)
		_, err := doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subMta.installedCSV, "-n", subMta.namespace)
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subMta.operatorPackage + "." + subMta.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		g.By("delete crd")
		crd.delete(oc)

		g.By("delete Operator resource to check if it is recreated")
		doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subMta.operatorPackage+"."+subMta.namespace)
		newCheck("present", asAdmin, withoutNamespace, notPresent, "", ok, []string{"operator", subMta.operatorPackage + "." + subMta.namespace}).check(oc)
	})

	// It will cover test case: OCP-24917, author: tbuskey@redhat.com
	g.It("Author:tbuskey-Medium-24917-Operators in SingleNamespace should not be granted namespace list [Disruptive]", func() {
		g.By("1) Install the OperatorGroup in a random project")
		dr := make(describerResrouce)
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)

		oc.SetupProject()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		og := operatorGroupDescription{
			name:      "og-24917",
			namespace: oc.Namespace(),
			template:  ogSingleTemplate,
		}
		og.createwithCheck(oc, itName, dr)

		g.By("2) Install the etcdoperator v0.9.4 with Automatic approval")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		sub := subscriptionDescription{
			subName:                "sub-24917",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			channel:                "singlenamespace-alpha",
			ipApproval:             "Automatic",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
			template:               subTemplate,
		}
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3) check if this operator's SA can list all namespaces")
		expectedSA := fmt.Sprintf("system:serviceaccount:%s:etcd-operator", oc.Namespace())
		msg, err := oc.AsAdmin().WithoutNamespace().Run("policy").Args("who-can", "list", "namespaces").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, expectedSA)).To(o.BeFalse())

		g.By("4) get the token of this operator's SA")
		token, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "etcd-operator", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("5) get the cluster server")
		server, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-server").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("6) get the current context")
		context, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-context").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// make sure switch to the current cluster-admin role after finished
		defer func() {
			g.By("9) Switch to the cluster-admin role")
			_, err := oc.AsAdmin().WithoutNamespace().Run("config").Args("use-context", context).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		g.By("7) login the cluster with this token")
		_, err = oc.AsAdmin().WithoutNamespace().Run("login").Args(server, "--token", token).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		whoami, err := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(whoami, expectedSA)).To(o.BeTrue())

		g.By("8) this SA user should NOT have the permission to list all namespaces")
		ns, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns").Output()
		o.Expect(strings.Contains(ns, "namespaces is forbidden")).To(o.BeTrue())
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-Medium-25782-CatalogSource Status should have information on last observed state", func() {
		var err error
		var (
			catName             = "installed-community-25782-global-operators"
			msg                 = ""
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			// the namespace and catName are hardcoded in the files
			cmTemplate       = filepath.Join(buildPruningBaseDir, "cm-csv-etcd.yaml")
			catsrcCmTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
		)

		oc.SetupProject()
		itName := g.CurrentGinkgoTestDescription().TestText

		var (
			cm = configMapDescription{
				name:      catName,
				namespace: oc.Namespace(),
				template:  cmTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        catName,
				namespace:   oc.Namespace(),
				displayName: "Community bad Operators",
				publisher:   "QE",
				sourceType:  "configmap",
				address:     catName,
				template:    catsrcCmTemplate,
			}
		)

		g.By("Create ConfigMap with bad operator manifest")
		cm.create(oc, itName, dr)

		// Make sure bad configmap was created
		g.By("Check configmap")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(msg, catName)).To(o.BeTrue())

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Wait for pod to fail")
		waitErr := wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", oc.Namespace()).Output()
			e2e.Logf("\n%v", msg)
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "CrashLoopBackOff") {
				e2e.Logf("STEP pod is in  CrashLoopBackOff as expected")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, "the pod is not in CrashLoopBackOff")

		g.By("Check catsrc state for TRANSIENT_FAILURE in lastObservedState")
		waitErr = wait.Poll(3*time.Second, 180*time.Second, func() (bool, error) {
			msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", catName, "-n", oc.Namespace(), "-o=jsonpath={.status}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "TRANSIENT_FAILURE") && strings.Contains(msg, "lastObservedState") {
				msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("catalogsource", catName, "-n", oc.Namespace(), "-o=jsonpath={.status.connectionState.lastObservedState}").Output()
				e2e.Logf("catalogsource had lastObservedState =  %v as expected ", msg)
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("catalogsource %s is not TRANSIENT_FAILURE", catName))
		e2e.Logf("cleaning up")
	})

	// It will cover test case: OCP-25644, author: tbuskey@redhat.com
	g.It("Author:tbuskey-Medium-25644-OLM collect CSV health per version", func() {
		var err error
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			ogAllTemplate       = filepath.Join(buildPruningBaseDir, "og-allns.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			csvName             = "etcdoperator.v0.9.4"
			next                = false
			ogName              = "test-25644-group"
		)

		oc.SetupProject()

		og := operatorGroupDescription{
			name:      ogName,
			namespace: oc.Namespace(),
			template:  ogTemplate,
		}
		ogAll := operatorGroupDescription{
			name:      ogName,
			namespace: oc.Namespace(),
			template:  ogAllTemplate,
		}

		sub := subscriptionDescription{
			subName:                "sub-25644",
			namespace:              oc.Namespace(),
			catalogSourceName:      "community-operators",
			catalogSourceNamespace: "openshift-marketplace",
			ipApproval:             "Automatic",
			template:               subFile,
			channel:                "singlenamespace-alpha",
			operatorPackage:        "etcd",
			startingCSV:            "etcdoperator.v0.9.4",
			singleNamespace:        true,
		}

		g.By("Create cluster-scoped OperatorGroup")
		ogAll.create(oc, itName, dr)
		msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "-n", oc.Namespace()).Output()
		e2e.Logf("og: %v, %v", msg, og.name)

		g.By("Subscribe to etcd operator and wait for the csv to fail")
		// CSV should fail && show fail.  oc describe csv xyz will have error
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)
		// find the CSV so that it can be delete after finished
		sub.findInstalledCSV(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Failed", ok, []string{"csv", csvName, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace(), csvName, "-o=jsonpath={.status.conditions..reason}").Output()
		e2e.Logf("--> get the csv reason: %v ", msg)
		o.Expect(strings.Contains(msg, "UnsupportedOperatorGroup") || strings.Contains(msg, "NoOperatorGroup")).To(o.BeTrue())

		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace(), csvName, "-o=jsonpath={.status.conditions..message}").Output()
		e2e.Logf("--> get the csv message: %v\n", msg)
		o.Expect(strings.Contains(msg, "InstallModeType not supported") || strings.Contains(msg, "csv in namespace with no operatorgroup")).To(o.BeTrue())

		g.By("Get prometheus token")
		olmToken, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmToken).NotTo(o.BeEmpty())

		g.By("get OLM pod name")
		olmPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=olm-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmPodname).NotTo(o.BeEmpty())

		g.By("check metrics")
		metrics, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(olmPodname, "-n", "openshift-operator-lifecycle-manager", "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://localhost:8443/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())

		var metricsVal, metricsVar string
		for _, s := range strings.Fields(metrics) {
			if next {
				metricsVal = s
				break
			}
			if strings.Contains(s, "csv_abnormal{") && strings.Contains(s, csvName) && strings.Contains(s, oc.Namespace()) {
				metricsVar = s
				next = true
			}
		}
		e2e.Logf("\nMetrics\n    %v == %v\n", metricsVar, metricsVal)
		o.Expect(metricsVal).NotTo(o.BeEmpty())

		g.By("reset og to single namespace")
		og.delete(itName, dr)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "-n", oc.Namespace()).Output()
		e2e.Logf("og deleted:%v", msg)

		og.create(oc, itName, dr)
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "-n", oc.Namespace(), "--no-headers").Output()
		e2e.Logf("og created:%v", msg)

		g.By("Wait for csv to recreate and ready")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace(), csvName, "-o=jsonpath={.status.reason}").Output()
		e2e.Logf("--> get the csv reason: %v ", msg)
		o.Expect(strings.Contains(msg, "InstallSucceeded")).To(o.BeTrue())
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("csv", "-n", oc.Namespace(), csvName, "-o=jsonpath={.status.message}").Output()
		e2e.Logf("--> get the csv message: %v\n", msg)
		o.Expect(strings.Contains(msg, "completed with no errors")).To(o.BeTrue())

		g.By("Make sure pods are fully running")
		waitErr := wait.Poll(5*time.Second, 120*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, "etcd-operator") && strings.Contains(msg, "Running") && strings.Contains(msg, "3/3") {
				return true, nil
			}
			return false, nil
		})
		e2e.Logf("\nPods\n%v", msg)
		exutil.AssertWaitPollNoErr(waitErr, "etcd-operator pod is not running as 3")

		g.By("check new metrics")
		next = false
		metricsVar = ""
		metricsVal = ""
		metrics, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args(olmPodname, "-n", "openshift-operator-lifecycle-manager", "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://localhost:8443/metrics").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(metrics).NotTo(o.BeEmpty())
		for _, s := range strings.Fields(metrics) {
			if next {
				metricsVal = s
				break
			}
			if strings.Contains(s, "csv_succeeded{") && strings.Contains(s, csvName) && strings.Contains(s, oc.Namespace()) {
				metricsVar = s
				next = true
			}
		}
		e2e.Logf("\nMetrics\n%v ==  %v\n", metricsVar, metricsVal)
		o.Expect(metricsVar).NotTo(o.BeEmpty())
		o.Expect(metricsVal).NotTo(o.BeEmpty())

		g.By("SUCCESS")

	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-29809-can complete automatical updates based on replaces", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-29809",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-29809",
				namespace:   "",
				displayName: "Test Catsrc 29809 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/cockroachdb-index:29809",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "cockroachdb-operator-29809",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				template:               subTemplate,
				singleNamespace:        true,
				startingCSV:            "cockroachdb.v5.0.3",
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create og")
		og.create(oc, itName, dr)

		g.By("create catalog source")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)

		g.By("check the operator upgrade to cockroachdb.v5.0.4")
		err := wait.Poll(15*time.Second, 480*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sub.namespace, "csv", "cockroachdb.v5.0.4", "-o=jsonpath={.spec.replaces}").Output()
			e2e.Logf(output)
			if err != nil {
				e2e.Logf("The csv is not created, error:%v", err)
				return false, nil
			}
			if strings.Contains(output, "cockroachdb.v5.0.3") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "cockroachdb.v5.0.4 does not replace cockroachdb.v5.0.3")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-30206-Medium-30242-can include secrets and configmaps in the bundle", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-30206",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-30206",
				namespace:   "",
				displayName: "Test Catsrc 30206 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/cockroachdb-index:5.0.4-30206",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "cockroachdb-operator-30206",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				template:               subTemplate,
				singleNamespace:        true,
				startingCSV:            "cockroachdb.v5.0.4",
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create og")
		og.create(oc, itName, dr)

		g.By("create catalog source")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		defer sub.delete(itName, dr)
		sub.create(oc, itName, dr)

		g.By("check secrets")
		errWait := wait.Poll(30*time.Second, 240*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sub.namespace, "secrets", "mysecret").Execute()
			if err != nil {
				e2e.Logf("Failed to create secrets, error:%v", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "mysecret is not created")

		g.By("check configmaps")
		errWait = wait.Poll(30*time.Second, 240*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sub.namespace, "configmaps", "my-config-map").Execute()
			if err != nil {
				e2e.Logf("Failed to create secrets, error:%v", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "my-config-map is not found")

		g.By("start to test OCP-30242")
		g.By("delete csv")
		sub.deleteCSV(itName, dr)

		g.By("check secrets has been deleted")
		errWait = wait.Poll(20*time.Second, 120*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sub.namespace, "secrets", "mysecret").Execute()
			if err != nil {
				e2e.Logf("The secrets has been deleted")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "mysecret is not found")

		g.By("check configmaps has been deleted")
		errWait = wait.Poll(20*time.Second, 120*time.Second, func() (bool, error) {
			err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", sub.namespace, "configmaps", "my-config-map").Execute()
			if err != nil {
				e2e.Logf("The configmaps has been deleted")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(errWait, "my-config-map still exists")
	})

	// Test case: OCP-24566, author:xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-24566-OLM automatically configures operators with global proxy config", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		subTemplateProxy := filepath.Join(buildPruningBaseDir, "olm-proxy-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: oc.Namespace(),
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-planetscale-operator",
				namespace:   oc.Namespace(),
				displayName: "Test planetscale Operators",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/planetscale-index:v1-4.8",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "planetscale-sub",
				namespace:              oc.Namespace(),
				catalogSourceName:      "catsrc-planetscale-operator",
				catalogSourceNamespace: oc.Namespace(),
				channel:                "beta",
				ipApproval:             "Automatic",
				operatorPackage:        "planetscale",
				singleNamespace:        true,
				template:               subTemplate,
			}
			subP = subscriptionDescription{subName: "planetscale-sub",
				namespace:              oc.Namespace(),
				catalogSourceName:      "catsrc-planetscale-operator",
				catalogSourceNamespace: oc.Namespace(),
				channel:                "beta",
				ipApproval:             "Automatic",
				operatorPackage:        "planetscale",
				singleNamespace:        true,
				template:               subTemplateProxy}
			subProxyTest = subscriptionDescriptionProxy{
				subscriptionDescription: subP,
				httpProxy:               "test_http_proxy",
				httpsProxy:              "test_https_proxy",
				noProxy:                 "test_no_proxy",
			}
			subProxyFake = subscriptionDescriptionProxy{
				subscriptionDescription: subP,
				httpProxy:               "fake_http_proxy",
				httpsProxy:              "fake_https_proxy",
				noProxy:                 "fake_no_proxy",
			}
			subProxyEmpty = subscriptionDescriptionProxy{
				subscriptionDescription: subP,
				httpProxy:               "",
				httpsProxy:              "",
				noProxy:                 "",
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText

		//oc get proxy cluster
		g.By(fmt.Sprintf("0) get the cluster proxy configuration"))
		httpProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpProxy}")
		httpsProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.httpsProxy}")
		noProxy := getResource(oc, asAdmin, withoutNamespace, "proxy", "cluster", "-o=jsonpath={.status.noProxy}")

		g.By(fmt.Sprintf("1) create the catsrc and OperatorGroup in project: %s", oc.Namespace()))
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("2) install sub")
		sub.create(oc, itName, dr)
		g.By("install operator SUCCESS")
		deployment := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o", "yaml")
		o.Expect(deployment).To(o.ContainSubstring("planetscale-operator"))

		if httpProxy == "" {
			nodeHTTPProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTP_PROXY\")].value}")
			o.Expect(nodeHTTPProxy).To(o.BeEmpty())
			nodeHTTPSProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTPS_PROXY\")].value}")
			o.Expect(nodeHTTPSProxy).To(o.BeEmpty())
			nodeNoProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"NO_PROXY\")].value}")
			o.Expect(nodeNoProxy).To(o.BeEmpty())
			g.By("CHECK proxy configure SUCCESS")
			sub.delete(itName, dr)
			sub.deleteCSV(itName, dr)
		} else {
			o.Expect(httpProxy).NotTo(o.BeEmpty())
			o.Expect(httpsProxy).NotTo(o.BeEmpty())
			o.Expect(noProxy).NotTo(o.BeEmpty())
			nodeHTTPProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTP_PROXY\")].value}")
			o.Expect(nodeHTTPProxy).To(o.Equal(httpProxy))
			nodeHTTPSProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTPS_PROXY\")].value}")
			o.Expect(nodeHTTPSProxy).To(o.Equal(httpsProxy))
			nodeNoProxy := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"NO_PROXY\")].value}")
			o.Expect(nodeNoProxy).To(o.Equal(noProxy))
			g.By("CHECK proxy configure SUCCESS")
			sub.delete(itName, dr)
			sub.deleteCSV(itName, dr)

			g.By("3) create subscription and set variables ( HTTP_PROXY, HTTPS_PROXY and NO_PROXY ) with non-empty values. ")
			subProxyTest.create(oc, itName, dr)
			nodeHTTPProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTP_PROXY\")].value}")
			o.Expect(nodeHTTPProxy).To(o.Equal("test_http_proxy"))
			nodeHTTPSProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTPS_PROXY\")].value}")
			o.Expect(nodeHTTPSProxy).To(o.Equal("test_https_proxy"))
			nodeNoProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"NO_PROXY\")].value}")
			o.Expect(nodeNoProxy).To(o.Equal("test_no_proxy"))
			subProxyTest.delete(itName, dr)
			subProxyTest.getCSV().delete(itName, dr)

			g.By("4) Create a new subscription and set variables ( HTTP_PROXY, HTTPS_PROXY and NO_PROXY ) with a fake value.")
			subProxyFake.create(oc, itName, dr)
			nodeHTTPProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTP_PROXY\")].value}")
			o.Expect(nodeHTTPProxy).To(o.Equal("fake_http_proxy"))
			nodeHTTPSProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTPS_PROXY\")].value}")
			o.Expect(nodeHTTPSProxy).To(o.Equal("fake_https_proxy"))
			nodeNoProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"NO_PROXY\")].value}")
			o.Expect(nodeNoProxy).To(o.Equal("fake_no_proxy"))
			subProxyFake.delete(itName, dr)
			subProxyFake.getCSV().delete(itName, dr)

			g.By("5) Create a new subscription and set variables ( HTTP_PROXY, HTTPS_PROXY and NO_PROXY ) with an empty value.")
			subProxyEmpty.create(oc, itName, dr)
			nodeHTTPProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=marketplace.operatorSource=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTP_PROXY\")].value}")
			o.Expect(nodeHTTPProxy).To(o.BeEmpty())
			nodeHTTPSProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=marketplace.operatorSource=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"HTTPS_PROXY\")].value}")
			o.Expect(nodeHTTPSProxy).To(o.BeEmpty())
			nodeNoProxy = getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=marketplace.operatorSource=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={.spec.template.spec.containers[?(.name==\"planetscale-operator\")].env[?(.name==\"NO_PROXY\")].value}")
			o.Expect(nodeNoProxy).To(o.BeEmpty())
			subProxyEmpty.delete(itName, dr)
			subProxyEmpty.getCSV().delete(itName, dr)
		}
	})

	// author: tbuskey@redhat.com, test case OCP-21080
	g.It("Author:tbuskey-High-21080-OLM Check OLM metrics [Serial]", func() {

		type metrics struct {
			csvCount              int
			csvUpgradeCount       int
			catalogSourceCount    int
			installPlanCount      int
			subscriptionCount     int
			subscriptionSyncTotal int
		}

		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catPodname          string
			data                PrometheusQueryResult
			err                 error
			exists              bool
			i                   int
			metricsBefore       metrics
			metricsAfter        metrics
			msg                 string
			olmPodname          string
			olmToken            string
			subSync             PrometheusQueryResult
		)

		oc.SetupProject()

		var (
			og = operatorGroupDescription{
				name:      "test-21080-group",
				namespace: oc.Namespace(),
				template:  ogTemplate,
			}
			sub = subscriptionDescription{
				subName:                "etcd-21080",
				namespace:              oc.Namespace(),
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				ipApproval:             "Automatic",
				channel:                "singlenamespace-alpha",
				operatorPackage:        "etcd",
				startingCSV:            "etcdoperator.v0.9.2",
				singleNamespace:        true,
				template:               subFile,
			}
		)

		g.By("1, check if this operator ready for instaalling")
		e2e.Logf("Check if %v exists in the %v catalog", sub.operatorPackage, sub.catalogSourceName)
		exists, err = clusterPackageExists(oc, sub)
		if !exists {
			e2e.Failf("FAIL:PackageMissing %v does not exist in catalog %v", sub.operatorPackage, sub.catalogSourceName)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(exists).To(o.BeTrue())

		g.By("2, Get token & pods so that access the Prometheus")
		og.create(oc, itName, dr)
		olmToken, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmToken).NotTo(o.BeEmpty())

		olmPodname, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=olm-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmPodname).NotTo(o.BeEmpty())

		catPodname, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catPodname).NotTo(o.BeEmpty())

		g.By("3, Collect olm metrics before installing an operator")
		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", olmPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=csv_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsBefore.csvCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", olmPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=csv_upgrade_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsBefore.csvUpgradeCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=catalog_source_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsBefore.catalogSourceCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=install_plan_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsBefore.installPlanCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=subscription_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsBefore.subscriptionCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		metricsBefore.subscriptionSyncTotal = 0

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=subscription_sync_total").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &subSync)
		for i = range subSync.Data.Result {
			if strings.Contains(subSync.Data.Result[i].Metric.SrcName, sub.subName) {
				metricsBefore.subscriptionSyncTotal, err = strconv.Atoi(subSync.Data.Result[i].Value[1].(string))
			}
		}

		e2e.Logf("\nbefore {csv_count, csv_upgrade_count, catalog_source_count, install_plan_count, subscription_count, subscription_sync_total}\n%v", metricsBefore)

		g.By("4, Start to subscribe to etcdoperator.v0.9.2")
		defer sub.delete(itName, dr) // remove the subscription after test
		sub.create(oc, itName, dr)

		defer sub.deleteCSV(itName, dr) // remove the csv after test
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("5, etcdoperator.v0.9.2 upgrade to etcdoperator.v0.9.4, start to collect olm metrics after")
		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", olmPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=csv_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsAfter.csvCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", olmPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=csv_upgrade_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsAfter.csvUpgradeCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=catalog_source_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsAfter.catalogSourceCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=install_plan_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsAfter.installPlanCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=subscription_count").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &data)
		metricsAfter.subscriptionCount, err = strconv.Atoi(data.Data.Result[0].Value[1].(string))

		metricsAfter.subscriptionSyncTotal = 0
		msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-operator-lifecycle-manager", catPodname, "-i", "--", "curl", "-k", "-H", fmt.Sprintf("Authorization: Bearer %v", olmToken), "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=subscription_sync_total").Outputs()
		o.Expect(msg).NotTo(o.BeEmpty())
		json.Unmarshal([]byte(msg), &subSync)
		for i = range subSync.Data.Result {
			if strings.Contains(subSync.Data.Result[i].Metric.SrcName, sub.subName) {
				metricsAfter.subscriptionSyncTotal, err = strconv.Atoi(subSync.Data.Result[i].Value[1].(string))
			}
		}

		g.By("6, Check results")
		e2e.Logf("{csv_count csv_upgrade_count catalog_source_count install_plan_count subscription_count subscription_sync_total}")
		e2e.Logf("%v", metricsBefore)
		e2e.Logf("%v", metricsAfter)

		g.By("Check Results")
		// csv_count can increase since there is a new csv generated
		o.Expect(metricsBefore.csvCount <= metricsAfter.csvCount).To(o.BeTrue())
		e2e.Logf("PASS csv_count is greater")

		/* These are not reliable if other operators are added in parallel
		// csv_upgrade_count should increase since its type is counter, see: https://prometheus.io/docs/concepts/metric_types/
		o.Expect((metricsAfter.csvUpgradeCount - metricsBefore.csvUpgradeCount) == 1).To(o.BeTrue())
		e2e.Logf("PASS csv_upgrade_count is greater")

		// catalog_source_count should be equal since we don't install/uninstall it in this test
		o.Expect(metricsBefore.catalogSourceCount == metricsAfter.catalogSourceCount).To(o.BeTrue())
		e2e.Logf("PASS catalog_source_count is equal")

		// install_plan_count should be greater since we there are 2 new ip generated in this case
		o.Expect(metricsBefore.installPlanCount < metricsAfter.installPlanCount).To(o.BeTrue())
		e2e.Logf("PASS install_plan_count is greater")

		// subscription_count should be greater since we there are 1 new subscription generated in this case
		o.Expect(metricsBefore.subscriptionCount < metricsAfter.subscriptionCount).To(o.BeTrue())
		e2e.Logf("PASS subscription_count is greater")

		// subscription_sync_total should be greater
		o.Expect(metricsBefore.subscriptionSyncTotal < metricsAfter.subscriptionSyncTotal).To(o.BeTrue())
		e2e.Logf("PASS subscription_sync_total is greater")
		*/

		g.By("All PASS\n")
	})

	// author: tbuskey@redhat.com, test case OCP-40972
	g.It("Author:tbuskey-High-40972-Provide more specific text when no candidates for Subscription spec", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catPodname          string
			err                 error
			exists              bool
			failures            = 0
			failureNames        = ""
			since               = "--since=360s"
			msg                 string
			s                   string
			snooze              time.Duration = 360
			step                string
			tail                = "--tail=100"
			waitErr             error
		)

		oc.SetupProject()

		var (
			og = operatorGroupDescription{
				name:      "test-40972-group",
				namespace: oc.Namespace(),
				template:  ogTemplate,
			}
			subOriginal = subscriptionDescription{
				subName:                "etcd-40972",
				namespace:              oc.Namespace(),
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				ipApproval:             "Automatic",
				channel:                "singlenamespace-alpha",
				operatorPackage:        "etcd",
				startingCSV:            "etcdoperator.v0.9.2",
				singleNamespace:        true,
				template:               subFile,
			}
			sub = subOriginal
		)

		g.By("1, check if this operator exists")
		e2e.Logf("Check if %v exists in the %v catalog", sub.operatorPackage, sub.catalogSourceName)
		exists, err = clusterPackageExists(oc, sub)
		if !exists {
			e2e.Failf("FAIL:PackageMissing %v does not exist in catalog %v", sub.operatorPackage, sub.catalogSourceName)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(exists).To(o.BeTrue())

		g.By("2, Get the OLM catalog pod name for log query")
		catPodname, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catPodname).NotTo(o.BeEmpty())

		g.By("3, Create og")
		og.create(oc, itName, dr)
		defer og.delete(itName, dr)

		g.By("1/4 bad package name")
		sub = subOriginal
		sub.operatorPackage = "xyzzy"
		s = fmt.Sprintf("no operators found in package %v in the catalog referenced by subscription %v", sub.operatorPackage, sub.subName)
		step = "1/4"

		sub.createWithoutCheck(oc, itName, dr)
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", tail, since).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, s) {
				return true, nil
			}
			return false, nil
		})
		if !strings.Contains(msg, s) {
			e2e.Logf("STEP after %v, %v FAIL log is missing %v\nSTEP in: %v\n", waitErr, step, s, msg)
			failures++
			failureNames = s + "\n"
		}
		sub.deleteCSV(itName, dr)
		sub.delete(itName, dr)

		g.By("2/4 bad catalog name")
		e2e.Logf("catpodname %v", catPodname)
		sub = subOriginal
		sub.catalogSourceName = "xyzzy"
		s = fmt.Sprintf("no operators found from catalog %v in namespace openshift-marketplace referenced by subscription %v", sub.catalogSourceName, sub.subName)
		step = "2/4"

		sub.createWithoutCheck(oc, itName, dr)
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", tail, since).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, s) {
				return true, nil
			}
			return false, nil
		})
		if !strings.Contains(msg, s) {
			e2e.Logf("STEP after %v, %v FAIL log is missing %v\nSTEP in: %v\n", waitErr, step, s, msg)
			failures++
			failureNames = failureNames + s + "\n"
		}
		sub.deleteCSV(itName, dr)
		sub.delete(itName, dr)

		g.By("3/4 bad channel")
		sub = subOriginal
		sub.channel = "xyzzy"
		s = fmt.Sprintf("no operators found in channel %v of package etcd in the catalog referenced by subscription %v", sub.channel, sub.subName)
		step = "3/4"

		sub.createWithoutCheck(oc, itName, dr)
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", tail, since).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, s) {
				return true, nil
			}
			return false, nil
		})
		if !strings.Contains(msg, s) {
			e2e.Logf("STEP after %v, %v FAIL log is missing %v\nSTEP in: %v\n", waitErr, step, s, msg)
			failures++
			failureNames = failureNames + s + "\n"
		}
		sub.deleteCSV(itName, dr)
		sub.delete(itName, dr)

		g.By("4/4 bad CSV")
		sub = subOriginal
		sub.startingCSV = "xyzzy.v0.9.2"
		s = fmt.Sprintf("no operators found with name %v in channel singlenamespace-alpha of package etcd in the catalog referenced by subscription %v", sub.startingCSV, sub.subName)
		step = "4/4"

		sub.createWithoutCheck(oc, itName, dr)
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", tail, since).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, s) {
				return true, nil
			}
			return false, nil
		})
		if !strings.Contains(msg, s) {
			e2e.Logf("STEP after %v, %v FAIL log is missing %v\nSTEP in: %v\n", waitErr, step, s, msg)
			failures++
			failureNames = failureNames + s + "\n"
		}
		sub.deleteCSV(itName, dr)
		sub.delete(itName, dr)

		g.By("FINISH\n")
		if failures != 0 {
			e2e.Failf("FAILED: %v times for %v", failures, failureNames)
		}
	})

	// author: xzha@redhat.com, test case OCP-40529
	g.It("ConnectedOnly-Author:xzha-Medium-40529-OPERATOR_CONDITION_NAME should have correct value", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			catsrc = catalogSourceDescription{
				name:        "catsrc-etcd-40529",
				namespace:   namespaceName,
				displayName: "Test Catsrc etcd Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/etcd-index:v1-4.8",
				template:    catsrcImageTemplate,
			}
			og = operatorGroupDescription{
				name:      "og-40529",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "sub-40529",
				namespace:              namespaceName,
				catalogSourceName:      "catsrc-etcd-40529",
				catalogSourceNamespace: namespaceName,
				channel:                "singlenamespace-alpha",
				ipApproval:             "Manual",
				operatorPackage:        "etcd",
				singleNamespace:        true,
				template:               subTemplate,
				startingCSV:            "etcdoperator.v0.9.2",
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("1: create the catalog source and OperatorGroup ")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("2: create sub")
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		// to get the latest installedCSV for manual subscription so that its csv can be deleted successfully
		defer sub.update(oc, itName, dr)

		sub.create(oc, itName, dr)
		e2e.Logf("approve the install plan")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.2", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.2", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("3: check OPERATOR_CONDITION_NAME")
		// there are 3 containers in this pod
		newCheck("expect", asAdmin, withoutNamespace, compare, "etcdoperator.v0.9.2 etcdoperator.v0.9.2 etcdoperator.v0.9.2", ok, []string{"deployment", "etcd-operator", "-n", namespaceName, "-o=jsonpath={.spec.template.spec.containers[*].env[?(@.name==\"OPERATOR_CONDITION_NAME\")].value}"}).check(oc)

		g.By("4: approve the install plan")
		sub.approveSpecificIP(oc, itName, dr, "etcdoperator.v0.9.4", "Complete")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", "etcdoperator.v0.9.4", "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("5: check OPERATOR_CONDITION_NAME")
		// there are 3 containers in this pod
		newCheck("expect", asAdmin, withoutNamespace, compare, "etcdoperator.v0.9.4 etcdoperator.v0.9.4 etcdoperator.v0.9.4", ok, []string{"deployment", "etcd-operator", "-n", namespaceName, "-o=jsonpath={.spec.template.spec.containers[*].env[?(@.name==\"OPERATOR_CONDITION_NAME\")].value}"}).check(oc)
	})

	// author: xzha@redhat.com, test case OCP-40534
	g.It("ConnectedOnly-Author:xzha-Medium-40534-the deployment should not lost the resources section", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "hive-40534-operator",
				namespace:              namespaceName,
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "hive-operator",
				singleNamespace:        true,
				template:               subTemplate,
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("STEP 1: create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("STEP 2: create sub")
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		sub.create(oc, itName, dr)

		g.By("STEP 3: check OPERATOR_CONDITION_NAME")
		cpuCSV := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={..containers[?(@.name==\"hive-operator\")].resources.requests.cpu}")
		o.Expect(cpuCSV).NotTo(o.BeEmpty())
		memoryCSV := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={..containers[?(@.name==\"hive-operator\")].resources.requests.memory}")
		o.Expect(memoryCSV).NotTo(o.BeEmpty())
		cpuDeployment := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..containers[?(@.name==\"hive-operator\")].resources.requests.cpu}")
		o.Expect(cpuDeployment).To(o.Equal(cpuDeployment))
		memoryDeployment := getResource(oc, asAdmin, withoutNamespace, "deployment", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..containers[?(@.name==\"hive-operator\")].resources.requests.memory}")
		o.Expect(memoryDeployment).To(o.Equal(memoryCSV))

	})

	// author: xzha@redhat.com, test case OCP-40532
	g.It("ConnectedOnly-Author:xzha-Medium-40532-OLM should not print debug logs", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "hive-40532-operator",
				namespace:              namespaceName,
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "hive-operator",
				singleNamespace:        true,
				template:               subTemplate,
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("STEP 1: create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("STEP 2: create sub")
		sub.create(oc, itName, dr)

		g.By("STEP 3: check there is no debug logs")
		olmPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=olm-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmPodname).NotTo(o.BeEmpty())
		olmlogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(olmPodname, "-n", "openshift-operator-lifecycle-manager", "--limit-bytes", "50000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(olmlogs).NotTo(o.BeEmpty())
		o.Expect(olmlogs).NotTo(o.ContainSubstring("level=debug"))

		catPodname, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-operator-lifecycle-manager", "--selector=app=catalog-operator", "-o=jsonpath={.items..metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catPodname).NotTo(o.BeEmpty())
		catalogs, err := oc.AsAdmin().WithoutNamespace().Run("logs").Args(catPodname, "-n", "openshift-operator-lifecycle-manager", "--limit-bytes", "50000").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(catalogs).NotTo(o.BeEmpty())
		o.Expect(catalogs).NotTo(o.ContainSubstring("level=debug"))
	})

	// Test case: OCP-42829, author:xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-42829-Install plan should be blocked till a valid OperatorGroup is detected", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: oc.Namespace(),
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-42829",
				namespace:   "openshift-marketplace",
				displayName: "Test planetscale Operators",
				publisher:   "OLM QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/planetscale-index:v1-4.8",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "planetscale-sub",
				namespace:              oc.Namespace(),
				catalogSourceName:      "catsrc-42829",
				catalogSourceNamespace: "openshift-marketplace",
				channel:                "beta",
				ipApproval:             "Automatic",
				operatorPackage:        "planetscale",
				singleNamespace:        true,
				template:               subTemplate,
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By(fmt.Sprintf("1) create the catsrc in project: %s", oc.Namespace()))
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		g.By("2) install sub")
		sub.createWithoutCheck(oc, itName, dr)

		g.By("3) check ip status")
		installPlan := sub.getIP(oc)
		o.Expect(installPlan).NotTo(o.BeEmpty())
		newCheck("expect", asAdmin, withoutNamespace, contain, "no operator group found", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)
		phase := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}")
		o.Expect(phase).To(o.Equal("Installing"))

		g.By("4) install og")
		og.createwithCheck(oc, itName, dr)

		g.By("check ip and csv")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Complete", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		sub.findInstalledCSV(oc, itName, dr)
		err := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
			status := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={..status.phase}")
			if strings.Compare(status, "Succeeded") == 0 {
				e2e.Logf("get installedCSV failed")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("csv %s is not Succeeded", sub.installedCSV))
	})

	// author: xzha@redhat.com, test case OCP-43110
	g.It("ConnectedOnly-Author:xzha-High-43110-OLM provide a helpful error message when install removed api", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			catsrc = catalogSourceDescription{
				name:        "catsrc-ditto-43110",
				namespace:   namespaceName,
				displayName: "Test Catsrc ditto Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/ditto-index:v1beta1",
				template:    catsrcImageTemplate,
			}
			og = operatorGroupDescription{
				name:      "og-43110",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "sub-43110",
				namespace:              namespaceName,
				catalogSourceName:      "catsrc-ditto-43110",
				catalogSourceNamespace: namespaceName,
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "ditto-operator",
				singleNamespace:        true,
				template:               subTemplate,
				startingCSV:            "",
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("1) create the catalog source and OperatorGroup")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("2) install sub")
		defer sub.delete(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)

		g.By("3) check ip/sub conditions")
		installPlan := sub.getIP(oc)
		o.Expect(installPlan).NotTo(o.BeEmpty())
		newCheck("expect", asAdmin, withoutNamespace, compare, "Failed", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		ipConditions := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}")
		o.Expect(ipConditions).To(o.ContainSubstring("api-server resource not found installing CustomResourceDefinition"))
		o.Expect(ipConditions).To(o.ContainSubstring("apiextensions.k8s.io/v1beta1"))
		o.Expect(ipConditions).To(o.ContainSubstring("Kind=CustomResourceDefinition not found on the cluster"))
		o.Expect(ipConditions).To(o.ContainSubstring("InstallComponentFailed"))
		subConditions := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions}")
		o.Expect(subConditions).To(o.ContainSubstring("api-server resource not found installing CustomResourceDefinition"))
		o.Expect(subConditions).To(o.ContainSubstring("apiextensions.k8s.io/v1beta1"))
		o.Expect(subConditions).To(o.ContainSubstring("Kind=CustomResourceDefinition not found on the cluster"))
		o.Expect(subConditions).To(o.ContainSubstring("InstallComponentFailed"))
		g.By("4) SUCCESS")
	})

	// author: xzha@redhat.com, test case OCP-43639
	g.It("ConnectedOnly-Author:xzha-High-43639-OLM must explicitly alert on deprecated APIs in use", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			catsrc = catalogSourceDescription{
				name:        "catsrc-ditto-43639",
				namespace:   namespaceName,
				displayName: "Test Catsrc ditto Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/ditto-index:v1beta1",
				template:    catsrcImageTemplate,
			}
			og = operatorGroupDescription{
				name:      "og-43639",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}
			sub = subscriptionDescription{
				subName:                "sub-43639",
				namespace:              namespaceName,
				catalogSourceName:      "catsrc-ditto-43639",
				catalogSourceNamespace: namespaceName,
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "ditto-operator",
				singleNamespace:        true,
				template:               subTemplate,
				startingCSV:            "",
			}
		)
		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("1) create the catalog source and OperatorGroup")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)
		og.createwithCheck(oc, itName, dr)

		g.By("2) install sub")
		defer sub.delete(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)
		installPlan := sub.getIP(oc)
		o.Expect(installPlan).NotTo(o.BeEmpty())
		err := wait.Poll(20*time.Second, 120*time.Second, func() (bool, error) {
			ipPhase := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}")
			if strings.Contains(ipPhase, "Complete") {
				e2e.Logf("sub is installed")
				return true, nil
			}
			return false, nil
		})
		if err == nil {
			g.By("3) check events")
			err2 := wait.Poll(20*time.Second, 240*time.Second, func() (bool, error) {
				eventOutput, err1 := oc.AsAdmin().WithoutNamespace().Run("get").Args("event", "-n", namespaceName).Output()
				o.Expect(err1).NotTo(o.HaveOccurred())
				lines := strings.Split(eventOutput, "\n")
				for _, line := range lines {
					if strings.Contains(line, "CustomResourceDefinition is deprecated") && strings.Contains(line, "piextensions.k8s.io") && strings.Contains(line, "ditto-operator") {
						return true, nil
					}
				}
				return false, nil
			})
			exutil.AssertWaitPollNoErr(err2, "event CustomResourceDefinition is deprecated, piextensions.k8s.io and ditto-operator not found")

		} else {
			g.By("3) the opeartor cannot be installed, skip test case")
		}

		g.By("4) SUCCESS")
	})

	// It will cover test case: OCP-40958, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-40958-Indicate invalid OperatorGroup on InstallPlan status", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			ogSAtemplate        = filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			saName              = "scopedv40958"
			og1                 = operatorGroupDescription{
				name:      "og1-40958",
				namespace: "",
				template:  ogSingleTemplate,
			}
			og2 = operatorGroupDescription{
				name:      "og2-40958",
				namespace: "",
				template:  ogSingleTemplate,
			}
			ogSa = operatorGroupDescription{
				name:               "ogsa-40958",
				namespace:          "",
				serviceAccountName: saName,
				template:           ogSAtemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-40958-operator",
				namespace:   "",
				displayName: "Test Catsrc 40958 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:v40958",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "teiid",
				namespace:              "",
				channel:                "beta",
				ipApproval:             "Automatic",
				operatorPackage:        "teiid",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "teiid.v0.4.0",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og1.namespace = oc.Namespace()
		og2.namespace = oc.Namespace()
		ogSa.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("install operator without og")
		sub.createWithoutCheck(oc, itName, dr)

		g.By("The install plan is Failed, without og")
		installPlan := sub.getIP(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installing", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "no operator group found", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlanPending", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)

		g.By("delete operator")
		sub.delete(itName, dr)

		g.By("Create og1")
		og1.create(oc, itName, dr)

		g.By("Create og2")
		og2.create(oc, itName, dr)

		g.By("install operator with multiple og")
		sub.createWithoutCheck(oc, itName, dr)

		g.By("The install plan is Failed, multiple og")
		installPlan = sub.getIP(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installing", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "more than one operator group", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallPlanPending", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)

		g.By("delete resource for next step")
		sub.delete(itName, dr)
		og1.delete(itName, dr)
		og2.delete(itName, dr)

		g.By("create sa")
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", saName, "-n", sub.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create ogSa")
		ogSa.createwithCheck(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, saName, ok, []string{"og", ogSa.name, "-n", ogSa.namespace, "-o=jsonpath={.status.serviceAccountRef.name}"}).check(oc)

		g.By("delete the service account")
		_, err = oc.WithoutNamespace().AsAdmin().Run("delete").Args("sa", saName, "-n", sub.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("install operator without sa for og")
		sub.createWithoutCheck(oc, itName, dr)

		g.By("The install plan is Failed, without sa for og")
		installPlan = sub.getIP(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Installing", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "not found+2+please make sure the service account exists", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "InstallComponentFailed+2+InstallPlanPending", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.conditions}"}).check(oc)
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-41174-Periodically retry InstallPlan execution until a timeout expires", func() {
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		roletemplate := filepath.Join(buildPruningBaseDir, "role.yaml")
		rolebindingtemplate := filepath.Join(buildPruningBaseDir, "role-binding.yaml")
		ogSAtemplate := filepath.Join(buildPruningBaseDir, "operatorgroup-serviceaccount.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		oc.SetupProject()
		namespace := oc.Namespace()
		itName := g.CurrentGinkgoTestDescription().TestText
		var (
			csv = "etcdoperator.v0.9.4"
			sa  = "scoped-41174"
			og  = operatorGroupDescription{
				name:               "test-og-41174",
				namespace:          namespace,
				serviceAccountName: sa,
				template:           ogSAtemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-etcd-41174",
				namespace:   namespace,
				displayName: "Test Catsrc etcd Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/etcd-index:v1-4.8",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "etcd",
				namespace:              namespace,
				catalogSourceName:      "catsrc-etcd-41174",
				catalogSourceNamespace: namespace,
				channel:                "singlenamespace-alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "etcd",
				singleNamespace:        true,
				template:               subTemplate,
				startingCSV:            csv,
			}
			role = roleDescription{
				name:      "role-41174",
				namespace: namespace,
				template:  roletemplate,
			}
			rolebinding = rolebindingDescription{
				name:      "scoped-bindings-41174",
				namespace: namespace,
				rolename:  "role-41174",
				saname:    sa,
				template:  rolebindingtemplate,
			}
		)

		g.By("1) Create the service account, catlog resource and OperatorGroup")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)
		_, err := oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", sub.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		og.createwithCheck(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, sa, ok, []string{"og", og.name, "-n", og.namespace, "-o=jsonpath={.status.serviceAccountRef.name}"}).check(oc)

		g.By("2) Delete the service account")
		_, err = oc.WithoutNamespace().AsAdmin().Run("delete").Args("sa", sa, "-n", sub.namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		// waiting for the condition is ready
		time.Sleep(2 * time.Second)

		g.By("3) Create a Subscription, check installplan")
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer sub.update(oc, itName, dr)

		sub.createWithoutCheck(oc, itName, dr)
		installPlan := sub.getIP(oc)
		o.Expect(installPlan).NotTo(o.BeEmpty())
		newCheck("expect", asAdmin, withoutNamespace, contain, "retrying execution due to error: serviceaccounts", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.message}"}).check(oc)

		g.By("4) Create the service account, check the installplan")
		_, err = oc.WithoutNamespace().AsAdmin().Run("create").Args("sa", sa, "-n", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newCheck("expect", asAdmin, withoutNamespace, contain, "retrying execution due to error: error creating csv etcdoperator", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.message}"}).check(oc)

		g.By("5) After retry timeout, the install plan is Failed")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Failed", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)

		g.By("6) delete sub, then create sub again")
		sub.delete(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)
		installPlan = sub.getIP(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "retrying execution due to error", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.message}"}).check(oc)

		g.By("7) Grant the proper permissions to the service account")
		role.create(oc)
		rolebinding.create(oc)

		g.By("8) Checking the state of CSV")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Complete", ok, []string{"installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asUser, withNamespace, compare, "Succeeded", ok, []string{"csv", csv, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		err = wait.Poll(1*time.Second, 10*time.Second, func() (bool, error) {
			installedCSV := getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}")
			if strings.Compare(installedCSV, "") == 0 {
				e2e.Logf("get installedCSV failed")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("csv of sub %v is not installed", sub.subName))
		g.By("9) SUCCESS")
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-41035-Fail InstallPlan on bundle unpack timeout [Slow]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate    = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-41035",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-41035",
				namespace:   "",
				displayName: "Test Catsrc 41035 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/ditto-index:41035",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "ditto-operator-41035",
				namespace:              "",
				channel:                "4.8",
				ipApproval:             "Automatic",
				operatorPackage:        "ditto-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create og")
		og.create(oc, itName, dr)

		g.By("create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		defer sub.delete(itName, dr)
		sub.createWithoutCheck(oc, itName, dr)

		g.By("The install plan is Failed")
		installPlan := sub.getIP(oc)
		err := wait.Poll(15*time.Second, 900*time.Second, func() (bool, error) {
			result := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.phase}")
			if strings.Compare(result, "Failed") == 0 {
				e2e.Logf("ip is failed")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ip of sub %v is not Failed", sub.subName))
		conditions := getResource(oc, asAdmin, withoutNamespace, "installplan", installPlan, "-n", sub.namespace, "-o=jsonpath={.status.conditions}")
		o.Expect(conditions).To(o.ContainSubstring("DeadlineExceeded"))
		o.Expect(conditions).To(o.ContainSubstring("Job was active longer than specified deadline"))
		o.Expect(conditions).To(o.ContainSubstring("Bundle unpacking failed"))
	})

	// author: tbuskey@redhat.com, test case OCP-43114
	g.It("Author:tbuskey-High-43114-Subscription status should show the message for InstallPlan failure conditions", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			subFile             = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			err                 error
			errorText           = "more than one operator group(s) are managing this namespace count=2"
			exists              bool
			msg                 string
			ip                   string
			snooze              time.Duration = 360
			testCase            = "43114"
			waitErr             error
		)

		oc.SetupProject()

		var (
			og = operatorGroupDescription{
				name:      testCase + "-1",
				namespace: oc.Namespace(),
				template:  ogTemplate,
			}
			sub = subscriptionDescription{
				subName:                "etcd-" + testCase,
				namespace:              oc.Namespace(),
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				ipApproval:             "Automatic",
				channel:                "singlenamespace-alpha",
				operatorPackage:        "etcd",
				startingCSV:            "etcdoperator.v0.9.4",
				singleNamespace:        true,
				template:               subFile,
			}
		)

		g.By("check if this operator exists")
		e2e.Logf("Check if %v exists in the %v catalog", sub.operatorPackage, sub.catalogSourceName)
		exists, err = clusterPackageExists(oc, sub)
		if !exists {
			e2e.Failf("FAIL:PackageMissing %v does not exist in catalog %v", sub.operatorPackage, sub.catalogSourceName)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(exists).To(o.BeTrue())

		g.By("Create 1st og")
		og.name = testCase + "-1"
		og.create(oc, itName, dr)
		defer og.delete(itName, dr)

		g.By("Create 2nd og")
		og.name = testCase + "-2"
		og.create(oc, itName, dr)
		defer og.delete(itName, dr)

		g.By("Check og")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("og", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("og  %v\n", msg)
		// expect both og to be in msg

		g.By("Subscribe")
		sub.createWithoutCheck(oc, itName, dr)
		defer sub.delete(itName, dr)

		g.By("Wait for sub to create install plan")
		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace(), sub.subName, "-o=jsonpath={.status.installplan}").Output()
			if strings.Contains(msg, "install-") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("The installplan name was not found in the subscription: %s", msg))

		g.By("Wait for error in the install plan status")
		ip, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace(), sub.subName, "-o=jsonpath={.status.installplan.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		waitErr = wait.Poll(10*time.Second, snooze*time.Second, func() (bool, error) {
			msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("installplan", "-n", oc.Namespace(), ip, "-o=jsonpath={.status.conditions..message}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(msg, errorText) {
				e2e.Logf("InstallPlan has the expected error")
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(waitErr, fmt.Sprintf("The installplan %s did not include expected message.  The message was instead %s", ip, msg))

		g.By("Check sub for message")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("sub", "-n", oc.Namespace(), sub.subName, "-o=jsonpath={.status.conditions..message}").Output()
		// e2e.Logf("sub %v, %v\n", err, msg)
		o.Expect(strings.Contains(msg, errorText)).To(o.BeTrue())
		e2e.Logf("subscription also has the expected error")

		g.By("Finished")

	})

})

var _ = g.Describe("[sig-operators] OLM for an end user handle to support", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-cm-"+getRandomString(), exutil.KubeConfigPath())
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
	})

	g.AfterEach(func() {})

	// It will cover part of test case: OCP-29275, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-29275-label to target namespace of operator group with multi namespace", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogMultiTemplate     = filepath.Join(buildPruningBaseDir, "og-multins.yaml")
			og                  = operatorGroupDescription{
				name:         "og-1651-1",
				namespace:    "",
				multinslabel: "test-og-label-1651",
				template:     ogMultiTemplate,
			}
			p1 = projectDescription{
				name:            "test-ns1651-1",
				targetNamespace: "",
			}
			p2 = projectDescription{
				name:            "test-ns1651-2",
				targetNamespace: "",
			}
		)

		defer p1.delete(oc)
		defer p2.delete(oc)
		//oc.TeardownProject()
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		p1.targetNamespace = oc.Namespace()
		p2.targetNamespace = oc.Namespace()
		og.namespace = oc.Namespace()
		g.By("Create new projects and label them")
		p1.create(oc, itName, dr)
		p1.label(oc, "test-og-label-1651")
		p2.create(oc, itName, dr)
		p2.label(oc, "test-og-label-1651")

		g.By("Create og and check the label")
		og.create(oc, itName, dr)
		ogUID := getResource(oc, asAdmin, withNamespace, "og", og.name, "-o=jsonpath={.metadata.uid}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, ok,
			[]string{"ns", p1.name, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, ok,
			[]string{"ns", p2.name, "-o=jsonpath={.metadata.labels}"}).check(oc)

		g.By("delete og and check there is no label")
		og.delete(itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, nok,
			[]string{"ns", p1.name, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, nok,
			[]string{"ns", p2.name, "-o=jsonpath={.metadata.labels}"}).check(oc)

		g.By("create another og to check the label")
		og.name = "og-1651-2"
		og.create(oc, itName, dr)
		ogUID = getResource(oc, asAdmin, withNamespace, "og", og.name, "-o=jsonpath={.metadata.uid}")
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, ok,
			[]string{"ns", p1.name, "-o=jsonpath={.metadata.labels}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "olm.operatorgroup.uid/"+ogUID, ok,
			[]string{"ns", p2.name, "-o=jsonpath={.metadata.labels}"}).check(oc)
	})

	// It will cover test case: OCP-22200, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-22200-add minimum kube version to CSV [Slow]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			cmNcTemplate        = filepath.Join(buildPruningBaseDir, "cm-namespaceconfig.yaml")
			catsrcCmTemplate    = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogTemplate,
			}
			cmNc = configMapDescription{
				name:      "cm-community-namespaceconfig-operators",
				namespace: "", //must be set in iT
				template:  cmNcTemplate,
			}
			catsrcNc = catalogSourceDescription{
				name:        "catsrc-community-namespaceconfig-operators",
				namespace:   "", //must be set in iT
				displayName: "Community namespaceconfig Operators",
				publisher:   "Community",
				sourceType:  "configmap",
				address:     "cm-community-namespaceconfig-operators",
				template:    catsrcCmTemplate,
			}
			subNc = subscriptionDescription{
				subName:                "namespace-configuration-operator",
				namespace:              "", //must be set in iT
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "namespace-configuration-operator",
				catalogSourceName:      "catsrc-community-namespaceconfig-operators",
				catalogSourceNamespace: "", //must be set in iT
				startingCSV:            "",
				currentCSV:             "namespace-configuration-operator.v0.1.0", //it matches to that in cm, so set it.
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			cm     = cmNc
			catsrc = catsrcNc
			sub    = subNc
		)

		//oc.TeardownProject()
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		cm.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace
		og.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create configmap of csv")
		cm.create(oc, itName, dr)

		g.By("Get minKubeVersionRequired and kubeVersionUpdated")
		output := getResource(oc, asUser, withoutNamespace, "cm", cm.name, "-o=json")
		csvDesc := strings.TrimSuffix(strings.TrimSpace(strings.SplitN(strings.SplitN(output, "\"clusterServiceVersions\": ", 2)[1], "\"customResourceDefinitions\":", 2)[0]), ",")
		o.Expect(strings.Contains(csvDesc, "minKubeVersion:")).To(o.BeTrue())
		minKubeVersionRequired := strings.TrimSpace(strings.SplitN(strings.SplitN(csvDesc, "minKubeVersion:", 2)[1], "\\n", 2)[0])
		kubeVersionUpdated := generateUpdatedKubernatesVersion(oc)
		e2e.Logf("the kubeVersionUpdated version is %s, and minKubeVersionRequired is %s", kubeVersionUpdated, minKubeVersionRequired)

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Update the minKubeVersion greater than the cluster KubeVersion")
		cm.patch(oc, fmt.Sprintf("{\"data\": {\"clusterServiceVersions\": %s}}", strings.ReplaceAll(csvDesc, "minKubeVersion: "+minKubeVersionRequired, "minKubeVersion: "+kubeVersionUpdated)))

		g.By("Create sub with greater KubeVersion")
		sub.create(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, contain, "CSV version requirement not met", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.requirementStatus[?(@.kind==\"ClusterServiceVersion\")].message}"}).check(oc)

		g.By("Remove sub and csv and update the minKubeVersion to orignl")
		sub.delete(itName, dr)
		sub.deleteCSV(itName, dr)
		cm.patch(oc, fmt.Sprintf("{\"data\": {\"clusterServiceVersions\": %s}}", csvDesc))

		g.By("Create sub with orignal KubeVersion")
		sub.create(oc, itName, dr)
		err := wait.Poll(15*time.Second, 360*time.Second, func() (bool, error) {
			csvPhase := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}")
			if strings.Contains(csvPhase, "Succeeded") {
				e2e.Logf("sub is installed")
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			msg := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.requirementStatus[?(@.kind==\"ClusterServiceVersion\")].message}")
			if strings.Contains(msg, "CSV version requirement not met") && !strings.Contains(msg, kubeVersionUpdated) {
				e2e.Failf("the csv can not be installed with correct kube version")
			}
		}
	})

	// It will cover test case: OCP-23473, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-23473-permit z-stream releases skipping during operator updates", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			cmNcTemplate        = filepath.Join(buildPruningBaseDir, "cm-namespaceconfig.yaml")
			catsrcCmTemplate    = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogTemplate,
			}
			skippedVersion = "namespace-configuration-operator.v0.0.2"
			cmNc           = configMapDescription{
				name:      "cm-community-namespaceconfig-operators",
				namespace: "", //must be set in iT
				template:  cmNcTemplate,
			}
			catsrcNc = catalogSourceDescription{
				name:        "catsrc-community-namespaceconfig-operators",
				namespace:   "", //must be set in iT
				displayName: "Community namespaceconfig Operators",
				publisher:   "Community",
				sourceType:  "configmap",
				address:     "cm-community-namespaceconfig-operators",
				template:    catsrcCmTemplate,
			}
			subNc = subscriptionDescription{
				subName:                "namespace-configuration-operator",
				namespace:              "", //must be set in iT
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "namespace-configuration-operator",
				catalogSourceName:      "catsrc-community-namespaceconfig-operators",
				catalogSourceNamespace: "", //must be set in iT
				startingCSV:            "",
				currentCSV:             "namespace-configuration-operator.v0.1.0", //it matches to that in cm, so set it.
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			cm     = cmNc
			catsrc = catsrcNc
			sub    = subNc
		)

		//oc.TeardownProject()
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		cm.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace
		og.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create configmap of csv")
		cm.create(oc, itName, dr)

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create sub")
		sub.ipApproval = "Manual"
		sub.startingCSV = "namespace-configuration-operator.v0.0.1"
		sub.create(oc, itName, dr)

		g.By("manually approve sub")
		sub.approve(oc, itName, dr)

		g.By(fmt.Sprintf("there is skipped csv version %s", skippedVersion))
		o.Expect(strings.Contains(sub.ipCsv, skippedVersion)).To(o.BeFalse())
	})

	// It will cover test case: OCP-24664, author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-24664-CRD updates if new schemas are backwards compatible", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			ogTemplate          = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			cmLearnV1Template   = filepath.Join(buildPruningBaseDir, "cm-learn-v1.yaml")
			cmLearnV2Template   = filepath.Join(buildPruningBaseDir, "cm-learn-v2.yaml")
			catsrcCmTemplate    = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			og                  = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogTemplate,
			}
			cmLearn = configMapDescription{
				name:      "cm-learn-operators",
				namespace: "", //must be set in iT
				template:  cmLearnV1Template,
			}
			catsrcLearn = catalogSourceDescription{
				name:        "catsrc-learn-operators",
				namespace:   "", //must be set in iT
				displayName: "Learn Operators",
				publisher:   "Community",
				sourceType:  "configmap",
				address:     "cm-learn-operators",
				template:    catsrcCmTemplate,
			}
			subLearn = subscriptionDescription{
				subName:                "learn-operator",
				namespace:              "", //must be set in iT
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "learn-operator",
				catalogSourceName:      "catsrc-learn-operators",
				catalogSourceNamespace: "", //must be set in iT
				startingCSV:            "",
				currentCSV:             "learn-operator.v0.0.1", //it matches to that in cm, so set it.
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			cm     = cmLearn
			catsrc = catsrcLearn
			sub    = subLearn
			crd    = crdDescription{
				name: "learns.app.learn.com",
			}
		)

		defer crd.delete(oc)

		//oc.TeardownProject()
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		cm.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace
		og.namespace = oc.Namespace()

		g.By("ensure no such crd")
		crd.delete(oc)

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create configmap of csv")
		cm.create(oc, itName, dr)

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create sub")
		sub.create(oc, itName, dr)
		newCheck("expect", asUser, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, contain, "v2", nok, []string{"crd", crd.name, "-A", "-o=jsonpath={.status.storedVersions}"}).check(oc)

		g.By("update the cm to update csv definition")
		cm.template = cmLearnV2Template
		cm.create(oc, itName, dr)

		g.By("update channel of Sub")
		sub.patch(oc, "{\"spec\": {\"channel\": \"beta\"}}")
		sub.expectCSV(oc, itName, dr, "learn-operator.v0.0.2")
		newCheck("expect", asAdmin, withoutNamespace, contain, "v2", ok, []string{"crd", crd.name, "-A", "-o=jsonpath={.status.storedVersions}"}).check(oc)
	})

	// It will cover test case: OCP-21824, author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-21824-verify CRD should be ready before installing the operator", func() {
		var (
			itName               = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir  = exutil.FixturePath("testdata", "olm")
			ogTemplate           = filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
			cmReadyTestTemplate  = filepath.Join(buildPruningBaseDir, "cm-certutil-readytest.yaml")
			catsrcCmTemplate     = filepath.Join(buildPruningBaseDir, "catalogsource-configmap.yaml")
			subTemplate          = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			crdOlmtestTemplate   = filepath.Join(buildPruningBaseDir, "crd-olmtest.yaml")
			cmReadyTestsTemplate = filepath.Join(buildPruningBaseDir, "cm-certutil-readytests.yaml")
			og                   = operatorGroupDescription{
				name:      "og-singlenamespace",
				namespace: "",
				template:  ogTemplate,
			}
			cmCertUtilReadytest = configMapDescription{
				name:      "cm-certutil-readytest-operators",
				namespace: "", //must be set in iT
				template:  cmReadyTestTemplate,
			}
			catsrcCertUtilReadytest = catalogSourceDescription{
				name:        "catsrc-certutil-readytest-operators",
				namespace:   "", //must be set in iT
				displayName: "certutil readytest Operators",
				publisher:   "Community",
				sourceType:  "configmap",
				address:     "cm-certutil-readytest-operators",
				template:    catsrcCmTemplate,
			}
			subCertUtilReadytest = subscriptionDescription{
				subName:                "cert-utils-operator",
				namespace:              "", //must be set in iT
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cert-utils-operator",
				catalogSourceName:      "catsrc-certutil-readytest-operators",
				catalogSourceNamespace: "", //must be set in iT
				startingCSV:            "",
				currentCSV:             "cert-utils-operator.v0.0.3", //it matches to that in cm, so set it.
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
			cm           = cmCertUtilReadytest
			catsrc       = catsrcCertUtilReadytest
			sub          = subCertUtilReadytest
			crdReadytest = crdDescription{
				name:     "readytest.stable.example.com",
				template: crdOlmtestTemplate,
			}
			crdReadytests = crdDescription{
				name:     "readytests.stable.example.com",
				template: crdOlmtestTemplate,
			}
		)

		defer crdReadytest.delete(oc)
		defer crdReadytests.delete(oc)

		//oc.TeardownProject()
		oc.SetupProject() //project and its resource are deleted automatically when out of It, so no need derfer or AfterEach
		cm.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace
		og.namespace = oc.Namespace()

		g.By("Create og")
		og.create(oc, itName, dr)

		g.By("Create cm with wrong crd")
		cm.create(oc, itName, dr)

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("Create sub and canot succeed")
		sub.createWithoutCheck(oc, itName, dr)
		newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}"}).check(oc)

		g.By("update cm to correct crd")
		cm.template = cmReadyTestsTemplate
		cm.create(oc, itName, dr)

		g.By("sub succeed and csv succeed")
		sub.findInstalledCSV(oc, itName, dr)
		newCheck("expect", asUser, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.status.phase}"}).check(oc)
	})

	// It will cover test case: OCP-43642, author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-Medium-43642-Alerts should be raised if the catalogsources are missing [Disruptive]", func() {
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform")
		}
		g.By("make all worker nodes as unschedulable")
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "--selector=node-role.kubernetes.io/worker=", "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Node Names are %v", nodeName)
		node := strings.Fields(nodeName)

		defer func() {
			for _, nodeIndex := range node {
				oc.AsAdmin().WithoutNamespace().Run("adm").Args("uncordon", fmt.Sprintf("%s", nodeIndex)).Execute()
			}
			err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
				catalogstrings := []string{"Certified Operators", "Community Operators", "Red Hat Operators", "Red Hat Marketplace"}
				output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifests").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				for _, catalogstring := range catalogstrings {
					if !strings.Contains(output, catalogstring) {
						e2e.Logf("cannot get packagemanifests for %s", catalogstring)
						return false, nil
					}
				}
				e2e.Logf("get packagemanifests for %s success", strings.Join(catalogstrings, ", "))
				return true, nil
			})
			exutil.AssertWaitPollNoErr(err, "cannot get packagemanifests for Certified Operators, Community Operators, Red Hat Operators and Red Hat Marketplace")
		}()

		for _, nodeIndex := range node {
			err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("cordon", fmt.Sprintf("%s", nodeIndex)).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("delete default catsrc certified-operators community-operators redhat-marketplace redhat-operators")
		catalogs := []string{"certified-operators", "community-operators", "redhat-marketplace", "redhat-operators"}
		for _, catalog := range catalogs {
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("catsrc", catalog, "-n", "openshift-marketplace").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("check alert has been raised")
		alerts := []string{"CommunityOperatorsCatalogError", "CertifiedOperatorsCatalogError", "RedhatOperatorsCatalogError", "RedhatMarketplaceCatalogError"}
		token, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("get-token", "prometheus-k8s", "-n", "openshift-monitoring").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		url, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "prometheus-k8s", "-n", "openshift-monitoring", "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(60*time.Second, 600*time.Second, func() (bool, error) {
			for _, alertString := range alerts {
				alertCMD := fmt.Sprintf("curl -s -k -H \"Authorization: Bearer %s\" https://%s/api/v1/alerts | jq -r '.data.alerts[] | select (.labels.alertname == \"%s\")'", token, url, alertString)
				output, err := exec.Command("bash", "-c", alertCMD).Output()
				if err != nil {
					e2e.Logf("Error retrieving prometheus alert metrics: %v, retry ...", err)
					return false, nil
				}
				if len(string(output)) == 0 {
					e2e.Logf("Prometheus alert is nil, retry ...")
					return false, nil
				}
				if !strings.Contains(string(output), "firing") && !strings.Contains(string(output), "pending") {
					e2e.Logf(string(output))
					return false, fmt.Errorf("alert state is not firing or pending")
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "alert state is not firing or pending")
	})

})

var _ = g.Describe("[sig-operators] OLM for an end user handle within all namespace", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-all-"+getRandomString(), exutil.KubeConfigPath())
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
	})

	g.AfterEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.getIr(itName).cleanup()
		dr.rmIr(itName)
	})

	// It will cover test case: OCP-21484, OCP-21532(acutally it covers OCP-21484), author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-21484-High-21532-watch special or all namespace by operator group", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			catsrc              = catalogSourceDescription{
				name:        "olm-21532-catalog",
				namespace:   "openshift-marketplace",
				displayName: "OLM 21532 Catalog",
				publisher:   "QE",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-dep:vcompos-v1",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "composable-operator",
				namespace:              "openshift-operators",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "composable-operator",
				catalogSourceName:      "olm-21532-catalog",
				catalogSourceNamespace: "openshift-marketplace",
				// startingCSV:            "composable-operator.v0.1.3",
				startingCSV:     "", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:      "",
				installedCSV:    "",
				template:        subTemplate,
				singleNamespace: false,
			}

			project = projectDescription{
				name:            "olm-enduser-specific-21484",
				targetNamespace: oc.Namespace(),
			}
			cl = checkList{}
		)

		// OCP-21532
		g.By("Check the global operator global-operators support all namesapces")
		cl.add(newCheck("expect", asAdmin, withoutNamespace, compare, "[]", ok, []string{"og", "global-operators", "-n", "openshift-operators", "-o=jsonpath={.status.namespaces}"}))

		g.By("create catsrc")
		catsrc.create(oc, itName, dr)
		defer catsrc.delete(itName, dr)

		// OCP-21484, OCP-21532
		g.By("Create operator targeted at all namespace")
		sub.create(oc, itName, dr) // the resource is cleaned within g.AfterEach

		g.By("Create new namespace")
		project.create(oc, itName, dr) // the resource is cleaned within g.AfterEach

		// OCP-21532
		g.By("New annotations is added to copied CSV in current namespace")
		cl.add(newCheck("expect", asUser, withNamespace, contain, "alm-examples", ok, []string{"csv", sub.installedCSV, "-o=jsonpath={.metadata.annotations}"}))

		// OCP-21484, OCP-21532
		g.By("Check the csv within new namespace is copied. note: the step is slow because it wait to copy csv to new namespace")
		cl.add(newCheck("expect", asAdmin, withoutNamespace, compare, "Copied", ok, []string{"csv", sub.installedCSV, "-n", project.name, "-o=jsonpath={.status.reason}"}))

		cl.check(oc)

	})

	// It will cover test case: OCP-24906, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-24906-Operators requesting cluster-scoped permission can trigger kube GC bug [Serial]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			sub                 = subscriptionDescription{
				subName:                "keda",
				namespace:              "openshift-operators",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "keda",
				catalogSourceName:      "community-operators",
				catalogSourceNamespace: "openshift-marketplace",
				startingCSV:            "", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
			}
			cl = checkList{}
		)

		g.By("Create operator targeted at all namespace")
		sub.create(oc, itName, dr)
		sub.update(oc, itName, dr)

		g.By("Check clusterrolebinding has no OwnerReferences")
		cl.add(newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"clusterrolebinding", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..OwnerReferences}"}))

		g.By("Check clusterrole has no OwnerReferences")
		cl.add(newCheck("expect", asAdmin, withoutNamespace, compare, "", ok, []string{"clusterrole", fmt.Sprintf("--selector=olm.owner=%s", sub.installedCSV), "-n", sub.namespace, "-o=jsonpath={..OwnerReferences}"}))
		//do check parallelly
		cl.check(oc)
	})

	// It will cover test case: OCP-33241, author: kuiwang@redhat.com
	g.It("ConnectedOnly-Author:kuiwang-Medium-33241-Enable generated operator component adoption for operators with all ns mode [Serial]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catsrc              = catalogSourceDescription{
				name:        "catsrc-33241-operator",
				namespace:   "openshift-marketplace",
				displayName: "Test Catsrc 33241 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/olm-api:v2",
				template:    catsrcImageTemplate,
			}
			subCockroachdb = subscriptionDescription{
				subName:                "cockroachdb33241",
				namespace:              "openshift-operators",
				channel:                "stable-5.x",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: catsrc.namespace,
				startingCSV:            "", //get it from package based on currentCSV if ipApproval is Automatic
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        false,
			}
		)

		g.By("check if cockroachdb is already installed with all ns.")
		csvList := getResource(oc, asAdmin, withoutNamespace, "csv", "-n", subCockroachdb.namespace, "-o=jsonpath={.items[*].metadata.name}")
		if !strings.Contains(csvList, subCockroachdb.operatorPackage) {
			g.By("create catsrc")
			catsrc.create(oc, itName, dr)
			defer catsrc.delete(itName, dr)

			g.By("Create operator targeted at all namespace")
			subCockroachdb.create(oc, itName, dr)
			csvCockroachdb := csvDescription{
				name:      subCockroachdb.installedCSV,
				namespace: subCockroachdb.namespace,
			}
			defer subCockroachdb.delete(itName, dr)
			defer csvCockroachdb.delete(itName, dr)
			crdName := getResource(oc, asAdmin, withoutNamespace, "operator", subCockroachdb.operatorPackage+"."+subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='CustomResourceDefinition')].name}")
			o.Expect(crdName).NotTo(o.BeEmpty())
			defer doAction(oc, "delete", asAdmin, withoutNamespace, "crd", crdName)
			defer doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subCockroachdb.operatorPackage+"."+subCockroachdb.namespace)

			g.By("Check all resources via operators")
			resourceKind := getResource(oc, asAdmin, withoutNamespace, "operator", subCockroachdb.operatorPackage+"."+subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}")
			o.Expect(resourceKind).To(o.ContainSubstring("Deployment"))
			o.Expect(resourceKind).To(o.ContainSubstring("Role"))
			o.Expect(resourceKind).To(o.ContainSubstring("RoleBinding"))
			o.Expect(resourceKind).To(o.ContainSubstring("ClusterRole"))
			o.Expect(resourceKind).To(o.ContainSubstring("ClusterRoleBinding"))
			o.Expect(resourceKind).To(o.ContainSubstring("CustomResourceDefinition"))
			o.Expect(resourceKind).To(o.ContainSubstring("Subscription"))
			o.Expect(resourceKind).To(o.ContainSubstring("InstallPlan"))
			o.Expect(resourceKind).To(o.ContainSubstring("ClusterServiceVersion"))
			newCheck("expect", asAdmin, withoutNamespace, contain, subCockroachdb.namespace, ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].namespace}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

			g.By("unlabel resource and it is relabeled automatically")
			roleName := getResource(oc, asAdmin, withoutNamespace, "operator", subCockroachdb.operatorPackage+"."+subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='Role')].name}")
			o.Expect(roleName).NotTo(o.BeEmpty())
			_, err := doAction(oc, "label", asAdmin, withoutNamespace, "-n", subCockroachdb.namespace, "Role", roleName, "operators.coreos.com/"+subCockroachdb.operatorPackage+"."+subCockroachdb.namespace+"-")
			o.Expect(err).NotTo(o.HaveOccurred())
			newCheck("expect", asAdmin, withoutNamespace, contain, "Role", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

			g.By("delete opertor and the Operator still exists because of crd")
			subCockroachdb.delete(itName, dr)
			csvCockroachdb.delete(itName, dr)
			newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subCockroachdb.operatorPackage + "." + subCockroachdb.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

			g.By("reinstall operator and check resource via Operator")
			subCockroachdb1 := subCockroachdb
			subCockroachdb1.create(oc, itName, dr)
			defer subCockroachdb1.delete(itName, dr)
			defer doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subCockroachdb1.installedCSV, "-n", subCockroachdb1.namespace)
			newCheck("expect", asAdmin, withoutNamespace, contain, "ClusterServiceVersion", ok, []string{"operator", subCockroachdb1.operatorPackage + "." + subCockroachdb1.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, subCockroachdb1.namespace, ok, []string{"operator", subCockroachdb1.operatorPackage + "." + subCockroachdb1.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].namespace}"}).check(oc)
			newCheck("expect", asAdmin, withoutNamespace, contain, "InstallSucceeded", ok, []string{"operator", subCockroachdb1.operatorPackage + "." + subCockroachdb1.namespace, "-o=jsonpath={.status.components.refs[?(.kind=='ClusterServiceVersion')].conditions[*].reason}"}).check(oc)

			g.By("delete operator and delete Operator and it will be recreated because of crd")
			subCockroachdb1.delete(itName, dr)
			_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "csv", subCockroachdb1.installedCSV, "-n", subCockroachdb1.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = doAction(oc, "delete", asAdmin, withoutNamespace, "operator", subCockroachdb1.operatorPackage+"."+subCockroachdb1.namespace)
			o.Expect(err).NotTo(o.HaveOccurred())
			// here there is issue and take WA
			_, err = doAction(oc, "label", asAdmin, withoutNamespace, "crd", crdName, "operators.coreos.com/"+subCockroachdb1.operatorPackage+"."+subCockroachdb1.namespace+"-")
			o.Expect(err).NotTo(o.HaveOccurred())
			_, err = doAction(oc, "label", asAdmin, withoutNamespace, "crd", crdName, "operators.coreos.com/"+subCockroachdb1.operatorPackage+"."+subCockroachdb1.namespace+"=")
			o.Expect(err).NotTo(o.HaveOccurred())
			//done for WA
			newCheck("expect", asAdmin, withoutNamespace, contain, "CustomResourceDefinition", ok, []string{"operator", subCockroachdb1.operatorPackage + "." + subCockroachdb1.namespace, "-o=jsonpath={.status.components.refs[*].kind}"}).check(oc)

		} else {
			g.By("it already exists")
		}
	})

	// author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-34181-can add conversion webhooks for singleton operators", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			crwebhook           = filepath.Join(buildPruningBaseDir, "cr-webhookTest.yaml")

			catsrc = catalogSourceDescription{
				name:        "catsrc-34181",
				namespace:   "openshift-marketplace",
				displayName: "Test Catsrc 34181 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/webhook-operator-index:0.0.3-v1",
				template:    catsrcImageTemplate,
			}
			sub = subscriptionDescription{
				subName:                "webhook-operator-34181",
				namespace:              "openshift-operators",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "webhook-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "openshift-marketplace",
				template:               subTemplate,
				singleNamespace:        false,
			}
		)

		g.By("create catlog resource")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		g.By("Check if the global operator global-operators support all namesapces")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[]", ok, []string{"og", "global-operators", "-n", "openshift-operators", "-o=jsonpath={.status.namespaces}"})

		g.By("create subscription targeted at all namespace")
		sub.create(oc, itName, dr)
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("crd", "webhooktests.webhook.operators.coreos.io", "-n", "openshift-operators").Execute()

		err := wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
			output, err := oc.AsAdmin().WithoutNamespace().Run("api-resources").Args("-o", "name").Output()
			if err != nil {
				e2e.Logf("There is no WebhookTest, err:%v", err)
				return false, nil
			}
			if strings.Contains(output, "webhooktests.webhook.operators.coreos.io") {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, "webhooktests.webhook.operators.coreos.io does exist")

		g.By("check invalid CR")
		configFile, err := oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", crwebhook, "-p", "NAME=webhooktest-34181",
			"NAMESPACE=openshift-operators", "VALID=false").OutputToFile("config-34181.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(15*time.Second, 180*time.Second, func() (bool, error) {
			erra := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
			if erra == nil {
				e2e.Logf("expect fail and try next")
				oc.AsAdmin().WithoutNamespace().Run("delete").Args("WebhookTest", "webhooktest-34181", "-n", "openshift-operators").Execute()
				return false, nil
			}
			e2e.Logf("err:%v", err)
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "can not apply webhooktest-34181")

		g.By("check valid CR")
		configFile, err = oc.AsAdmin().Run("process").Args("--ignore-unknown-parameters=true", "-f", crwebhook, "-p", "NAME=webhooktest-34181",
			"NAMESPACE=openshift-operators", "VALID=true").OutputToFile("config-34181.json")
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("WebhookTest", "webhooktest-34181", "-n", "openshift-operators").Execute()
		err = wait.Poll(15*time.Second, 300*time.Second, func() (bool, error) {
			erra := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
			if erra != nil {
				e2e.Logf("try next, err:%v", err)
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "can not apply webhooktest-34181 again")
	})

	// It will cover test case: OCP-40531, author: xzha@redhat.com
	g.It("ConnectedOnly-Author:xzha-High-40531-High-41051-the value of lastUpdateTime of csv and Components of Operator should be correct [Serial]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
			catsrcImageTemplate = filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
			sub                 = subscriptionDescription{
				subName:                "sub-40531",
				namespace:              "openshift-operators",
				channel:                "clusterwide-alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "etcd",
				catalogSourceName:      "catsrc-etcd-40531",
				catalogSourceNamespace: "openshift-marketplace",
				template:               subTemplate,
				singleNamespace:        false,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-etcd-40531",
				namespace:   "openshift-marketplace",
				displayName: "Test Catsrc etcd Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     "quay.io/olmqe/etcd-index:v1-4.8",
				template:    catsrcImageTemplate,
			}
		)
		g.By("0, create catlog resource")
		defer catsrc.delete(itName, dr)
		catsrc.create(oc, itName, dr)

		g.By("1, Check if the global operator global-operators support all namesapces")
		newCheck("expect", asAdmin, withoutNamespace, compare, "[]", ok, []string{"og", "global-operators", "-n", "openshift-operators", "-o=jsonpath={.status.namespaces}"})

		g.By("2, Create operator targeted at all namespace")
		sub.create(oc, itName, dr)
		defer sub.delete(itName, dr)
		defer sub.deleteCSV(itName, dr)

		g.By("3, Create new namespace")
		oc.SetupProject()

		g.By("4, Check the csv within new namespace is copied.")
		newCheck("expect", asAdmin, withoutNamespace, compare, "Succeeded", ok, []string{"csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.phase}"}).check(oc)
		newCheck("expect", asAdmin, withoutNamespace, compare, "Copied", ok, []string{"csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.reason}"})

		g.By("5, OCP-40531-Check the lastUpdateTime of copied CSV is equal to the original CSV.")
		originCh := make(chan string)
		defer close(originCh)
		copyCh := make(chan string)
		defer close(copyCh)
		go func() {
			originCh <- getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", "openshift-operators", "-o=jsonpath={.status.lastUpdateTime}")
		}()
		go func() {
			copyCh <- getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", oc.Namespace(), "-o=jsonpath={.status.lastUpdateTime}")
		}()
		lastUpdateTimeOrigin := <-originCh
		lastUpdateTimeNew := <-copyCh
		e2e.Logf("OriginTimeStamp:%s, CopiedTimeStamp:%s", lastUpdateTimeOrigin, lastUpdateTimeNew)
		o.Expect(lastUpdateTimeNew).To(o.Equal(lastUpdateTimeOrigin))

		g.By("6, OCP-41051-Check Operator.Status.Components does not contain copied CSVs.")
		operatorname := sub.operatorPackage + ".openshift-operators"
		operatorinfo := getResource(oc, asAdmin, withoutNamespace, "operator", operatorname, "-n", oc.Namespace(), "-o=jsonpath={.status.components.refs}")
		o.Expect(operatorinfo).NotTo(o.BeEmpty())
		o.Expect(operatorinfo).NotTo(o.ContainSubstring("Copied"))
	})
})

var _ = g.Describe("[sig-operators] OLM on VM for an end user handle within a namespace", func() {
	defer g.GinkgoRecover()

	var (
		oc = exutil.NewCLI("olm-vm-"+getRandomString(), exutil.KubeConfigPath())
		dr = make(describerResrouce)
	)

	g.BeforeEach(func() {
		itName := g.CurrentGinkgoTestDescription().TestText
		dr.addIr(itName)
	})

	g.AfterEach(func() {})

	// Test case: OCP-27672, author:xzha@redhat.com
	g.It("VMonly-ConnectedOnly-Author:xzha-Medium-27672-Allow Operator Registry Update Polling with automatic ipApproval [Slow]", func() {
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		quayCLI := container.NewQuayCLI()
		sqlit := db.NewSqlit()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		defer DeleteDir(buildPruningBaseDir, "fixture-testdata")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		bundleImageTag1 := "quay.io/olmqe/ditto-operator:0.1.0"
		bundleImageTag2 := "quay.io/olmqe/ditto-operator:0.1.1"
		indexTag := "quay.io/olmqe/ditto-index:" + getRandomString()
		defer containerCLI.RemoveImage(indexTag)
		catsrcName := "catsrc-27672-operator" + getRandomString()
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        catsrcName,
				namespace:   namespaceName,
				displayName: "Test-Catsrc-17672-Operators",
				publisher:   "Red-Hat",
				sourceType:  "grpc",
				address:     indexTag,
				interval:    "2m0s",
				template:    catsrcImageTemplate,
			}

			sub = subscriptionDescription{
				subName:                "ditto-27672-operator",
				namespace:              namespaceName,
				catalogSourceName:      catsrcName,
				catalogSourceNamespace: namespaceName,
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "ditto-operator",
				singleNamespace:        true,
				template:               subTemplate,
			}
		)

		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("STEP: create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("STEP 1: prepare CatalogSource index image")
		_, err := containerCLI.Run("pull").Args(bundleImageTag1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("catsrc image is %s", indexTag)
		output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag1, "-t", indexTag, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("push image")
		output, err = containerCLI.Run("push").Args(indexTag).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		defer quayCLI.DeleteTag(strings.Replace(indexTag, "quay.io/", "", 1))
		e2e.Logf("check image")
		indexdbPath := filepath.Join(buildPruningBaseDir, getRandomString())
		err = os.Mkdir(indexdbPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexTag, "--path", "/database/index.db:"+indexdbPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbPath, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err := sqlit.CheckOperatorBundlePathExist(path.Join(indexdbPath, "index.db"), bundleImageTag2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeFalse())

		g.By("STEP 2: Create catalog source")
		catsrc.create(oc, itName, dr)
		g.By("STEP 3: install operator ")
		sub.create(oc, itName, dr)
		o.Expect(sub.getCSV().name).To(o.Equal("ditto-operator.v0.1.0"))

		g.By("STEP 4: update CatalogSource index image")
		output, err = opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag2, "-f", indexTag, "-t", indexTag, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("push image")
		output, err = containerCLI.Run("push").Args(indexTag).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("check index image")
		indexdbPath = filepath.Join(buildPruningBaseDir, getRandomString())
		err = os.Mkdir(indexdbPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexTag, "--path", "/database/index.db:"+indexdbPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbPath, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err = sqlit.CheckOperatorBundlePathExist(path.Join(indexdbPath, "index.db"), bundleImageTag2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		g.By("STEP 5: check the operator has been updated")
		err = wait.Poll(3*time.Second, 300*time.Second, func() (bool, error) {
			sub.findInstalledCSV(oc, itName, dr)
			if strings.Compare(sub.installedCSV, "ditto-operator.v0.1.1") == 0 {
				return true, nil
			}
			return false, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ditto-operator.v0.1.1 of sub %s fails", sub.subName))

		g.By("STEP 6: delete the catsrc sub csv")
		catsrc.delete(itName, dr)
		sub.delete(itName, dr)
		sub.getCSV().delete(itName, dr)
	})

	g.It("VMonly-ConnectedOnly-Author:xzha-Medium-27672-Allow Operator Registry Update Polling with manual ipApproval [Slow]", func() {
		containerCLI := container.NewPodmanCLI()
		containerTool := "podman"
		quayCLI := container.NewQuayCLI()
		sqlit := db.NewSqlit()
		buildPruningBaseDir := exutil.FixturePath("testdata", "olm")
		defer DeleteDir(buildPruningBaseDir, "fixture-testdata")
		ogSingleTemplate := filepath.Join(buildPruningBaseDir, "operatorgroup.yaml")
		subTemplate := filepath.Join(buildPruningBaseDir, "olm-subscription.yaml")
		catsrcImageTemplate := filepath.Join(buildPruningBaseDir, "catalogsource-image.yaml")
		bundleImageTag1 := "quay.io/olmqe/ditto-operator:0.1.0"
		bundleImageTag2 := "quay.io/olmqe/ditto-operator:0.1.1"
		indexTag := "quay.io/olmqe/ditto-index:" + getRandomString()
		defer containerCLI.RemoveImage(indexTag)
		catsrcName := "catsrc-27672-operator" + getRandomString()
		oc.SetupProject()
		namespaceName := oc.Namespace()
		var (
			og = operatorGroupDescription{
				name:      "test-og",
				namespace: namespaceName,
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        catsrcName,
				namespace:   namespaceName,
				displayName: "Test-Catsrc-17672-Operators",
				publisher:   "Red-Hat",
				sourceType:  "grpc",
				address:     indexTag,
				interval:    "2m0s",
				template:    catsrcImageTemplate,
			}
			sub_manual = subscriptionDescription{
				subName:                "ditto-27672-operator",
				namespace:              namespaceName,
				catalogSourceName:      catsrcName,
				catalogSourceNamespace: namespaceName,
				channel:                "alpha",
				ipApproval:             "Manual",
				operatorPackage:        "ditto-operator",
				singleNamespace:        true,
				template:               subTemplate,
			}
		)

		itName := g.CurrentGinkgoTestDescription().TestText
		g.By("STEP: create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		e2e.Logf("catsrc image is %s", indexTag)
		g.By("STEP 1: prepare CatalogSource index image")
		_, err := containerCLI.Run("pull").Args(bundleImageTag1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag1, "-t", indexTag, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexTag).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		defer quayCLI.DeleteTag(strings.Replace(indexTag, "quay.io/", "", 1))
		e2e.Logf("check image")
		indexdbPath := filepath.Join(buildPruningBaseDir, getRandomString())
		err = os.Mkdir(indexdbPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexTag, "--path", "/database/index.db:"+indexdbPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbPath, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err := sqlit.CheckOperatorBundlePathExist(path.Join(indexdbPath, "index.db"), bundleImageTag2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeFalse())

		g.By("STEP 2: Create catalog source")
		catsrc.create(oc, itName, dr)
		g.By("STEP 3: install operator ")
		sub_manual.create(oc, itName, dr)
		e2e.Logf("approve the install plan")
		sub_manual.approve(oc, itName, dr)
		sub_manual.expectCSV(oc, itName, dr, "ditto-operator.v0.1.0")

		g.By("STEP 4: update CatalogSource index image")
		output, err = opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag2, "-f", indexTag, "-t", indexTag, "-c", containerTool).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		output, err = containerCLI.Run("push").Args(indexTag).Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("check index image")
		indexdbPath = filepath.Join(buildPruningBaseDir, getRandomString())
		err = os.Mkdir(indexdbPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("get index.db")
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexTag, "--path", "/database/index.db:"+indexdbPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", path.Join(indexdbPath, "index.db"))
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err = sqlit.CheckOperatorBundlePathExist(path.Join(indexdbPath, "index.db"), bundleImageTag2)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.BeTrue())

		g.By("STEP 5: approve the install plan")
		err = wait.Poll(3*time.Second, 300*time.Second, func() (bool, error) {
			ipCsv := getResource(oc, asAdmin, withoutNamespace, "sub", sub_manual.subName, "-n", sub_manual.namespace, "-o=jsonpath={.status.installplan.name}{\" \"}{.status.currentCSV}")
			if strings.Contains(ipCsv, "ditto-operator.v0.1.1") {
				return true, nil
			} else {
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("ditto-operator.v0.1.1 of sub %s fails", sub_manual.subName))
		sub_manual.approveSpecificIP(oc, itName, dr, "ditto-operator.v0.1.1", "Complete")
		g.By("STEP 6: check the csv")
		sub_manual.expectCSV(oc, itName, dr, "ditto-operator.v0.1.1")
		e2e.Logf("delete the catsrc sub csv")
		catsrc.delete(itName, dr)
		sub_manual.delete(itName, dr)
		sub_manual.getCSV().delete(itName, dr)
	})

	// OCP-43641 author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-VMonly-Medium-43641-opm index add fails during image extraction", func() {
		containerCLI := container.NewPodmanCLI()
		bundleImage := "quay.io/olmqe/etcd:0.9.4"
		indexImage := "quay.io/olmqe/etcd-index:v1-4.8"
		defer containerCLI.RemoveImage(bundleImage)
		defer containerCLI.RemoveImage(indexImage)

		g.By("step: checking user account is no-root")
		user, err := exec.Command("bash", "-c", "whoami").Output()
		e2e.Logf("User:%s", user)
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Compare(string(user), "root") == -1 {
			g.By("step: opm index add")
			output1, err := opm.NewOpmCLI().Run("index").Args("add", "--generate", "--bundles", bundleImage, "--from-index", indexImage, "--overwrite-latest").Output()
			e2e.Logf(output1)
			o.Expect(err).NotTo(o.HaveOccurred())
			g.By("test case 43641 SUCCESS")
		} else {
			e2e.Logf("User is %s. the case should login as no-root account", user)
		}
	})

	// author: xzha@redhat.com
	g.It("Author:xzha-ConnectedOnly-VMonly-Medium-25920-Expose bundle data from bundle image container", func() {
		var (
			opmBaseDir          = exutil.FixturePath("testdata", "opm")
			TestDataPath        = filepath.Join(opmBaseDir, "etcd_operator")
			buildPruningBaseDir = exutil.FixturePath("testdata", "olm")
			cmTemplate          = filepath.Join(buildPruningBaseDir, "cm-template.yaml")
			cmName              = "cm-25920"
			cm                  = configMapDescription{
				name:      cmName,
				namespace: oc.Namespace(),
				template:  cmTemplate,
			}
			itName = g.CurrentGinkgoTestDescription().TestText
		)

		opmCLI := opm.NewOpmCLI()
		defer DeleteDir(TestDataPath, "fixture-testdata")
		defer DeleteDir(buildPruningBaseDir, "fixture-testdata")

		g.By("1) create a ConfigMap")
		defer cm.delete(itName, dr)
		cm.create(oc, itName, dr)

		g.By("2) opm alpha bundle extract")
		_, err := opmCLI.Run("alpha").Args("bundle", "extract", "-c", cmName, "-n", oc.Namespace(), "-k", exutil.KubeConfigPath(), "-m", TestDataPath+"/0.9.2/").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("3) Check the data of this ConfigMap object.")
		data, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cm", cmName, "-n", oc.Namespace(), "-o=jsonpath={.metadata.annotations}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.channel.default.v1"))
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.channels.v1"))
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.manifests.v1"))
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.mediatype.v1"))
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.metadata.v1"))
		o.Expect(data).To(o.ContainSubstring("operators.operatorframework.io.bundle.package.v1"))
	})

	// author: xzha@redhat.com
	g.It("VMonly-ConnectedOnly-Author:xzha-Medium-40528-opm can filter the platform/arch of the index image", func() {
		baseDir := exutil.FixturePath("testdata", "olm")
		TestDataPath := filepath.Join(baseDir, "temp")
		indexTmpPath := filepath.Join(TestDataPath, getRandomString())
		defer DeleteDir(TestDataPath, indexTmpPath)
		err := os.MkdirAll(indexTmpPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		indexImage := "registry.redhat.io/redhat/redhat-operator-index:v4.6"

		g.By("1) check oc adm calalog mirror help")
		output, err := oc.AsAdmin().Run("adm").Args("catalog", "mirror", "--help").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("--index-filter-by-os"))
		o.Expect(output).NotTo(o.ContainSubstring("--filter-by-os"))

		g.By("2) run oc adm calalog mirror with --index-filter-by-os=linux/amd64")
		dockerconfigjsonpath := filepath.Join(indexTmpPath, ".dockerconfigjson")
		defer exec.Command("rm", "-f", dockerconfigjsonpath).Output()
		_, err = oc.AsAdmin().Run("extract").Args("secret/pull-secret", "-n", "openshift-config", "--confirm", "--to="+indexTmpPath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		tmpPath1 := filepath.Join(indexTmpPath, "amd64")
		output, err = oc.AsAdmin().Run("adm").Args("catalog", "mirror", "--index-filter-by-os=linux/amd64", indexImage,
			"localhost:5000", "--manifests-only", "--to-manifests="+tmpPath1, "-a", dockerconfigjsonpath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Chose linux/amd64 manifest from the manifest list"))
		o.Expect(output).To(o.ContainSubstring("wrote mirroring manifests to "))

		g.By("3) Check the data of mapping.txt")
		result, err := exec.Command("bash", "-c", "cat "+tmpPath1+"/mapping.txt|grep -E redhat-operator-index").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("localhost:5000/redhat/redhat-operator-index:v4.6"))

		g.By("4) run oc adm calalog mirror with --index-filter-by-os=linux/s390x")
		tmpPath2 := filepath.Join(indexTmpPath, "s390x")
		output, err = oc.AsAdmin().Run("adm").Args("catalog", "mirror", "--index-filter-by-os=linux/s390x", indexImage,
			"localhost:5000", "--manifests-only", "--to-manifests="+tmpPath2, "-a", dockerconfigjsonpath).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Chose linux/s390x manifest from the manifest list"))
		o.Expect(output).To(o.ContainSubstring("wrote mirroring manifests to "))

		g.By("5) Check the data of mapping.txt")
		result, err = exec.Command("bash", "-c", "cat "+tmpPath2+"/mapping.txt|grep -E redhat-operator-index").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("localhost:5000/redhat/redhat-operator-index:v4.6"))

		g.By("6) run oc adm calalog mirror with --index-filter-by-os=linux/abc")
		tmpPath3 := filepath.Join(indexTmpPath, "abc")
		output, _ = oc.AsAdmin().Run("adm").Args("catalog", "mirror", "--index-filter-by-os=linux/abc", indexImage,
			"localhost:5000", "--manifests-only", "--to-manifests="+tmpPath3, "-a", dockerconfigjsonpath).Output()
		o.Expect(output).To(o.ContainSubstring("error: the image is a manifest list and contains multiple images"))

	})

	g.It("VMonly-ConnectedOnly-Author:xzha-High-42979-Bundle authors can explicitly specify arbitrary properties", func() {
		var (
			containerCLI    = container.NewPodmanCLI()
			containerTool   = "podman"
			quayCLI         = container.NewQuayCLI()
			opmCLI          = opm.NewOpmCLI()
			bundleImageTag1 = "quay.io/olmqe/cockroachdb-operator:5.0.3-42979-" + getRandomString()
			bundleImageTag2 = "quay.io/olmqe/cockroachdb-operator:5.0.4-42979-" + getRandomString()
			indexImageTag   = "quay.io/olmqe/cockroachdb-index:42979-" + getRandomString()
		)

		defer containerCLI.RemoveImage(indexImageTag)
		defer containerCLI.RemoveImage(bundleImageTag1)
		defer containerCLI.RemoveImage(bundleImageTag2)
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag2, "quay.io/", "", 1))

		output := ""
		var err error
		g.By("build bundle image 1")
		opmBaseDir := exutil.FixturePath("testdata", "opm", "cockroachdb", "supportproperties")
		TestDataPath1 := filepath.Join(opmBaseDir, "5.0.3")
		defer DeleteDir(TestDataPath1, "fixture-testdata")
		opmCLI.ExecCommandPath = TestDataPath1
		if output, err = opmCLI.Run("alpha").Args("bundle", "build", "-d", "manifests", "-b", containerTool, "-t", bundleImageTag1, "-p", "cockroachdb", "-c", "alpha", "-e", "alpha").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle build : %s", output)
		}
		if output, err = containerCLI.Run("push").Args(bundleImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		DeleteDir(TestDataPath1, "fixture-testdata")

		g.By("build bundle image 2")
		opmBaseDir = exutil.FixturePath("testdata", "opm", "cockroachdb", "supportproperties")
		TestDataPath2 := filepath.Join(opmBaseDir, "5.0.4")
		defer DeleteDir(TestDataPath2, "fixture-testdata")
		opmCLI.ExecCommandPath = TestDataPath2
		if output, err = opmCLI.Run("alpha").Args("bundle", "build", "-d", "manifests", "-b", containerTool, "-t", bundleImageTag2, "-p", "cockroachdb", "-c", "alpha", "-e", "alpha").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle build : %s", output)
		}
		if output, err = containerCLI.Run("push").Args(bundleImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image")
		if output, err := opmCLI.Run("index").Args("add", "-b", bundleImageTag1+","+bundleImageTag2, "-t", indexImageTag, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("get index.db")
		TmpDataPath := filepath.Join(opmBaseDir, "tmp")
		dbFilePath := filepath.Join(TmpDataPath, "index.db")
		err = os.MkdirAll(TmpDataPath, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("image").Args("extract", indexImageTag, "--path", "/database/index.db:"+TmpDataPath).Output()
		e2e.Logf("get index.db SUCCESS, path is %s", dbFilePath)
		if _, err := os.Stat(dbFilePath); os.IsNotExist(err) {
			e2e.Logf("get index.db Failed")
		}

		g.By("Run the opm registry server binary to load manifest and serves a grpc API to query it.")
		defer exec.Command("kill", "-9", "$(lsof -t -i:42979)").Output()
		e2e.Logf("step: Run the registry-server")
		cmd := exec.Command("opm", "registry", "serve", "-d", dbFilePath, "-t", filepath.Join(TmpDataPath, "42979.log"), "-p", "42979")
		cmd.Dir = TmpDataPath
		err = cmd.Start()
		o.Expect(err).NotTo(o.HaveOccurred())
		time.Sleep(time.Second * 1)
		e2e.Logf("step: check api.Registry/ListBundles")
		outputCurl, err := exec.Command("grpcurl", "-plaintext", "localhost:42979", "api.Registry/ListBundles").Output()
		e2e.Logf(string(outputCurl))
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(outputCurl)).To(o.ContainSubstring("cockroachdb.v5.0.3"))
		o.Expect(string(outputCurl)).To(o.ContainSubstring("cockroachdb.v5.0.4"))
		o.Expect(string(outputCurl)).To(o.ContainSubstring("olm.maxOpenShiftVersion"))
		o.Expect(string(outputCurl)).To(o.ContainSubstring("version is 5.0.3"))
		o.Expect(string(outputCurl)).To(o.ContainSubstring("version is 5.0.4"))
		cmd.Process.Kill()

		var (
			itName            = g.CurrentGinkgoTestDescription().TestText
			buildIndexBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate  = filepath.Join(buildIndexBaseDir, "operatorgroup.yaml")
			catsrcTemplate    = filepath.Join(buildIndexBaseDir, "catalogsource-image.yaml")
			subTemplate       = filepath.Join(buildIndexBaseDir, "olm-subscription.yaml")
			og                = operatorGroupDescription{
				name:      "test-og",
				namespace: "",
				template:  ogSingleTemplate,
			}
			catsrc = catalogSourceDescription{
				name:        "catsrc-42979",
				namespace:   "",
				displayName: "Test Catsrc 42979 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     indexImageTag,
				template:    catsrcTemplate,
			}
			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.3",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		defer DeleteDir(buildIndexBaseDir, "fixture-testdata")
		oc.SetupProject()
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = oc.Namespace()

		g.By("create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)
		err = wait.Poll(3*time.Second, 120*time.Second, func() (bool, error) {
			exists, error := clusterPackageExists(oc, sub)
			if !exists || error != nil {
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("package of sub %s does not exist", sub.subName))

		g.By("install operator")
		sub.createWithoutCheck(oc, itName, dr)
		sub.expectCSV(oc, itName, dr, "cockroachdb.v5.0.4")
		csvOutput := getResource(oc, asAdmin, withoutNamespace, "csv", sub.installedCSV, "-n", sub.namespace, "-o=jsonpath={.metadata.annotations}")
		o.Expect(string(csvOutput)).To(o.ContainSubstring("version is 5.0.4"))
		o.Expect(string(csvOutput)).To(o.ContainSubstring("olm.maxOpenShiftVersion"))

		g.By("SUCCESS")
	})

	// Test case: OCP-30835, author:kuiwang@redhat.com
	g.It("VMonly-ConnectedOnly-Author:kuiwang-Medium-30835-complete operator upgrades automatically based on SemVer setting default channel in opm alpha bundle build", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildIndexBaseDir   = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildIndexBaseDir, "olm-subscription.yaml")
			ogSingleTemplate    = filepath.Join(buildIndexBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildIndexBaseDir, "catalogsource-image.yaml")

			containerCLI  = container.NewPodmanCLI()
			containerTool = "podman"
			quayCLI       = container.NewQuayCLI()

			// these bundles are prepared data, do not need to remove them after case exits.
			bundleImageTag1 = "quay.io/olmqe/cockroachdb-operator:5.0.3-30835"
			bundleImageTag2 = "quay.io/olmqe/cockroachdb-operator:5.0.4-30835"

			// these index are generated by case, need to ensure to remove them after case exits.
			indexImageTag1 = "quay.io/olmqe/cockroachdb-index:5.0.3-30835-" + getRandomString()
			indexImageTag2 = "quay.io/olmqe/cockroachdb-index:5.0.4-30835-" + getRandomString()

			og = operatorGroupDescription{
				name:      "test-og",
				namespace: "",
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        "catsrc-30835",
				namespace:   "",
				displayName: "Test Catsrc 30835 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     indexImageTag2,
				template:    catsrcImageTemplate,
			}

			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.3",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		defer DeleteDir(buildIndexBaseDir, "fixture-testdata")
		defer containerCLI.RemoveImage(indexImageTag1)
		defer containerCLI.RemoveImage(indexImageTag2)
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag2, "quay.io/", "", 1))

		oc.SetupProject()
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("pull bundle image for index image")
		_, err := containerCLI.Run("pull").Args(bundleImageTag1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("build index image 1")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag1, "-t", indexImageTag1, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 2")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag2, "-f", indexImageTag1, "-t", indexImageTag2, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		sub.createWithoutCheck(oc, itName, dr)
		sub.expectCSV(oc, itName, dr, "cockroachdb.v5.0.4")

		g.By("delete the catsrc sub csv") // actually this step could not be necessary because the resource of the project will be removed when the project is removed
		catsrc.delete(itName, dr)
		sub.delete(itName, dr)
		sub.getCSV().delete(itName, dr)
	})

	// Test case: OCP-30860, author:kuiwang@redhat.com
	g.It("VMonly-ConnectedOnly-Author:kuiwang-Medium-30860-complete operator upgrades automatically based on SemVer instead of replaces or skips [Slow]", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildIndexBaseDir   = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildIndexBaseDir, "olm-subscription.yaml")
			ogSingleTemplate    = filepath.Join(buildIndexBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildIndexBaseDir, "catalogsource-image.yaml")

			containerCLI  = container.NewPodmanCLI()
			containerTool = "podman"
			quayCLI       = container.NewQuayCLI()

			// these bundles are prepared data, do not need to remove them after case exits.
			bundleImageTag1 = "quay.io/olmqe/mta-operator:0.0.3-30860"
			bundleImageTag2 = "quay.io/olmqe/mta-operator:0.0.5-30860"
			bundleImageTag3 = "quay.io/olmqe/mta-operator:0.0.4-30860"

			// these index are generated by case, need to ensure to remove them after case exits.
			indexImageTag1 = "quay.io/olmqe/mta-index:0.0.3-30860-" + getRandomString()
			indexImageTag2 = "quay.io/olmqe/mta-index:0.0.5-30860-" + getRandomString()
			indexImageTag3 = "quay.io/olmqe/mta-index:0.0.4-30860-" + getRandomString()

			og = operatorGroupDescription{
				name:      "test-og",
				namespace: "",
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        "catsrc-30860",
				namespace:   "",
				displayName: "Test Catsrc 30860 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     indexImageTag3,
				template:    catsrcImageTemplate,
			}

			sub = subscriptionDescription{
				subName:                "mta",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "mta-operator",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "windup-operator.0.0.3",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		defer DeleteDir(buildIndexBaseDir, "fixture-testdata")
		defer containerCLI.RemoveImage(indexImageTag1)
		defer containerCLI.RemoveImage(indexImageTag2)
		defer containerCLI.RemoveImage(indexImageTag3)
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag2, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag3, "quay.io/", "", 1))

		oc.SetupProject()
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("pull bundle image for index image")
		_, err := containerCLI.Run("pull").Args(bundleImageTag1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag3).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("build index image 1")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag1, "-t", indexImageTag1, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 2")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag2, "-f", indexImageTag1, "-t", indexImageTag2, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 3")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag3, "-f", indexImageTag2, "-t", indexImageTag3, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag3).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		sub.createWithoutCheck(oc, itName, dr) // actually it is operator upgrade
		state := ""
		err = wait.Poll(20*time.Second, 240*time.Second, func() (bool, error) {
			state = getResource(oc, asAdmin, withoutNamespace, "sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.state}")
			if strings.Compare(state, "AtLatestKnown") == 0 {
				return true, nil
			}
			return false, nil
		})
		if err != nil {
			e2e.Logf("state is %v", state)
			if strings.Compare(state, "UpgradeAvailable") == 0 {
				newCheck("expect", asAdmin, withoutNamespace, compare, "windup-operator.0.0.4", ok, []string{"sub", sub.subName, "-n", sub.namespace, "-o=jsonpath={.status.installedCSV}"}).check(oc)
			} else {
				e2e.Failf("the operator does not start upgrade")
			}
		} else {
			sub.expectCSV(oc, itName, dr, "windup-operator.0.0.5")
		}

		g.By("delete the catsrc sub csv") // actually this step could not be necessary because the resource of the project will be removed when the project is removed
		catsrc.delete(itName, dr)
		sub.delete(itName, dr)
		sub.getCSV().delete(itName, dr)
	})

	// Test case: OCP-30674, author:kuiwang@redhat.com
	g.It("VMonly-ConnectedOnly-Author:kuiwang-Medium-30674-complete operator upgrades automatically based on SemVer without setting default channel", func() {
		var (
			itName              = g.CurrentGinkgoTestDescription().TestText
			buildIndexBaseDir   = exutil.FixturePath("testdata", "olm")
			subTemplate         = filepath.Join(buildIndexBaseDir, "olm-subscription.yaml")
			ogSingleTemplate    = filepath.Join(buildIndexBaseDir, "operatorgroup.yaml")
			catsrcImageTemplate = filepath.Join(buildIndexBaseDir, "catalogsource-image.yaml")

			containerCLI  = container.NewPodmanCLI()
			containerTool = "podman"
			quayCLI       = container.NewQuayCLI()

			// these bundles are prepared data, do not need to remove them after case exits.
			bundleImageTag1 = "quay.io/olmqe/cockroachdb-operator:5.0.3-30674"
			bundleImageTag2 = "quay.io/olmqe/cockroachdb-operator:5.0.4-30674"

			// these index are generated by case, need to ensure to remove them after case exits.
			indexImageTag1 = "quay.io/olmqe/cockroachdb-index:5.0.3-30674-" + getRandomString()
			indexImageTag2 = "quay.io/olmqe/cockroachdb-index:5.0.4-30674-" + getRandomString()

			og = operatorGroupDescription{
				name:      "test-og",
				namespace: "",
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        "catsrc-30674",
				namespace:   "",
				displayName: "Test Catsrc 30674 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     indexImageTag2,
				template:    catsrcImageTemplate,
			}

			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.3",
				currentCSV:             "",
				installedCSV:           "",
				template:               subTemplate,
				singleNamespace:        true,
			}
		)

		defer DeleteDir(buildIndexBaseDir, "fixture-testdata")
		defer containerCLI.RemoveImage(indexImageTag1)
		defer containerCLI.RemoveImage(indexImageTag2)
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag2, "quay.io/", "", 1))

		oc.SetupProject()
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		g.By("pull bundle image for index image")
		_, err := containerCLI.Run("pull").Args(bundleImageTag1).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = containerCLI.Run("pull").Args(bundleImageTag2).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("build index image 1")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag1, "-t", indexImageTag1, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 2")
		if output, err := opm.NewOpmCLI().Run("index").Args("add", "-b", bundleImageTag2, "-f", indexImageTag1, "-t", indexImageTag2, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Create catalog source")
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		sub.createWithoutCheck(oc, itName, dr)
		sub.expectCSV(oc, itName, dr, "cockroachdb.v5.0.4")

		g.By("delete the catsrc sub csv") // actually this step could not be necessary because the resource of the project will be removed when the project is removed
		catsrc.delete(itName, dr)
		sub.delete(itName, dr)
		sub.getCSV().delete(itName, dr)
	})

	// Test case: OCP-29810, author:kuiwang@redhat.com
	g.It("VMonly-ConnectedOnly-Author:kuiwang-Medium-29810-The bundle and index image reated successfully when spec replaces field is null", func() {
		var (
			itName            = g.CurrentGinkgoTestDescription().TestText
			buildIndexBaseDir = exutil.FixturePath("testdata", "olm")
			ogSingleTemplate  = filepath.Join(buildIndexBaseDir, "operatorgroup.yaml")
			opmBaseDir        = exutil.FixturePath("testdata", "opm")

			containerCLI  = container.NewPodmanCLI()
			containerTool = "podman"
			quayCLI       = container.NewQuayCLI()
			opmCLI        = opm.NewOpmCLI()

			// these bundles are generated by case, need to ensure to remove them after case exits.
			bundleImageTag1 = "quay.io/olmqe/cockroachdb-operator:5.0.3-29810-" + getRandomString()
			bundleImageTag2 = "quay.io/olmqe/cockroachdb-operator:5.0.4-29810-" + getRandomString()

			// these index are generated by case, need to ensure to remove them after case exits.
			indexImageTag1 = "quay.io/olmqe/cockroachdb-index:5.0.3-29810-" + getRandomString()
			indexImageTag2 = "quay.io/olmqe/cockroachdb-index:5.0.4-29810-" + getRandomString()

			og = operatorGroupDescription{
				name:      "test-og",
				namespace: "",
				template:  ogSingleTemplate,
			}

			catsrc = catalogSourceDescription{
				name:        "catsrc-29810",
				namespace:   "",
				displayName: "Test Catsrc 29810 Operators",
				publisher:   "Red Hat",
				sourceType:  "grpc",
				address:     indexImageTag2,
				template:    "",
			}

			sub = subscriptionDescription{
				subName:                "cockroachdb",
				namespace:              "",
				channel:                "alpha",
				ipApproval:             "Automatic",
				operatorPackage:        "cockroachdb",
				catalogSourceName:      catsrc.name,
				catalogSourceNamespace: "",
				startingCSV:            "cockroachdb.v5.0.3",
				currentCSV:             "",
				installedCSV:           "",
				template:               "",
				singleNamespace:        true,
			}
		)

		defer DeleteDir(buildIndexBaseDir, "fixture-testdata")
		defer containerCLI.RemoveImage(indexImageTag1)
		defer containerCLI.RemoveImage(indexImageTag2)
		defer containerCLI.RemoveImage(bundleImageTag1)
		defer containerCLI.RemoveImage(bundleImageTag2)
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(indexImageTag2, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag1, "quay.io/", "", 1))
		defer quayCLI.DeleteTag(strings.Replace(bundleImageTag2, "quay.io/", "", 1))

		oc.SetupProject()
		og.namespace = oc.Namespace()
		catsrc.namespace = oc.Namespace()
		sub.namespace = oc.Namespace()
		sub.catalogSourceNamespace = catsrc.namespace

		g.By("create the OperatorGroup ")
		og.createwithCheck(oc, itName, dr)

		output := ""
		var err error
		g.By("build bundle image 1")
		TestDataPath1 := filepath.Join(opmBaseDir, "cockroachdb", "supportsemver")
		defer DeleteDir(TestDataPath1, "fixture-testdata")
		opmCLI.ExecCommandPath = TestDataPath1

		if output, err = opmCLI.Run("alpha").Args("bundle", "build", "-d", "5.0.3", "-b", "podman", "-t", bundleImageTag1, "-p", "cockroachdb", "-c", "alpha", "-e", "alpha").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle build : %s", output)
		}
		if output, err = containerCLI.Run("push").Args(bundleImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		DeleteDir(TestDataPath1, "fixture-testdata")

		g.By("build bundle image 2")
		opmBaseDir = exutil.FixturePath("testdata", "opm")
		TestDataPath2 := filepath.Join(opmBaseDir, "cockroachdb", "supportsemver")
		defer DeleteDir(TestDataPath2, "fixture-testdata")
		opmCLI.ExecCommandPath = TestDataPath2

		if output, err = opmCLI.Run("alpha").Args("bundle", "build", "-d", "5.0.4", "-b", "podman", "-t", bundleImageTag2, "-p", "cockroachdb", "-c", "alpha", "-e", "alpha").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if !strings.Contains(output, "Writing annotations.yaml") || !strings.Contains(output, "Writing bundle.Dockerfile") {
			e2e.Failf("Failed to execute opm alpha bundle build : %s", output)
		}
		if output, err = containerCLI.Run("push").Args(bundleImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 1")
		if output, err := opmCLI.Run("index").Args("add", "-b", bundleImageTag1, "-t", indexImageTag1, "-c", containerTool).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag1).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("build index image 2")
		if output, err := opmCLI.Run("index").Args("add", "-b", bundleImageTag2, "-f", indexImageTag1, "-t", indexImageTag2, "-c", containerTool, "--mode", "semver").Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		if output, err := containerCLI.Run("push").Args(indexImageTag2).Output(); err != nil {
			e2e.Logf(output)
			o.Expect(err).NotTo(o.HaveOccurred())
		}

		g.By("Create catalog source")
		buildIndexBaseDir = exutil.FixturePath("testdata", "olm")
		catsrc.template = filepath.Join(buildIndexBaseDir, "catalogsource-image.yaml")
		catsrc.create(oc, itName, dr)

		g.By("install operator")
		sub.template = filepath.Join(buildIndexBaseDir, "olm-subscription.yaml")
		sub.createWithoutCheck(oc, itName, dr)
		sub.expectCSV(oc, itName, dr, "cockroachdb.v5.0.4")

		g.By("delete the catsrc sub csv") // actually this step could not be necessary because the resource of the project will be removed when the project is removed
		catsrc.delete(itName, dr)
		sub.delete(itName, dr)
		sub.getCSV().delete(itName, dr)
	})

	// Test case: OCP-30695, author:kuiwang@redhat.com
	g.It("VMonly-ConnectedOnly-Author:kuiwang-Medium-30695-oc adm catalog mirror should mirror bundle images", func() {
		var (
			// it is prepared index, and no need to remove it.
			indexImageTag   = "quay.io/olmqe/cockroachdb-index:2.1.11-30695"
			cockroachdbPath = "operators-cockroachdb-manifests-" + getRandomString()
		)
		defer exec.Command("bash", "-c", "rm -fr ./"+cockroachdbPath).Output()

		g.By("mirror to localhost:5000")
		output, err := oc.AsAdmin().WithoutNamespace().Run("adm", "catalog", "mirror").Args("--manifests-only", "--to-manifests="+cockroachdbPath, indexImageTag, "localhost:5000").Output()
		e2e.Logf("the output is %v", output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("operators-cockroachdb-manifests"))

		g.By("check mapping.txt to localhost:5000")
		result, err := exec.Command("bash", "-c", "cat ./"+cockroachdbPath+"/mapping.txt|grep -E \"quay.io/kuiwang/cockroachdb-operator\"").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("cockroachdb-operator:2.1.11"))
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-21953-Ensure that operator deployment is in the master node", func() {
		var (
			err            error
			msg            string
			olmErrs        = true
			olmJpath       = "-o=jsonpath={@.spec.template.spec.nodeSelector}"
			olmNamespace   = "openshift-marketplace"
			olmNodeName    string
			olmPodFullName string
			olmPodName     = "marketplace-operator"
			nodeRole       = "node-role.kubernetes.io/master"
			nodes          string
			nodeStatus     bool
			pod            string
			pods           string
			status         []string
			x              []string
		)

		g.By("Get deployment")
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deployment", "-n", olmNamespace, olmPodName, olmJpath).Output()
		if err != nil {
			e2e.Logf("Unable to get deployment -n %v %v %v.", olmNamespace, olmPodName, olmJpath)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(msg) < 1 || !strings.Contains(msg, nodeRole) {
			e2e.Failf("Could not find %v variable %v for %v: %v", olmJpath, nodeRole, olmPodName, msg)
		}

		g.By("Look at pods")
		// look for the marketplace-operator pod's full name
		pods, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", olmNamespace, "-o", "wide").Output()
		if err != nil {
			e2e.Logf("Unable to query pods -n %v %v %v.", olmNamespace, err, pods)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(pods).NotTo(o.ContainSubstring("No resources found"))
		// e2e.Logf("Pods %v ", pods)

		for _, pod = range strings.Split(pods, "\n") {
			if len(pod) <= 0 {
				continue
			}
			// Find the node in the pod
			if strings.Contains(pod, olmPodName) {
				x = strings.Fields(pod)
				olmPodFullName = x[0]
				// olmNodeName = x[6]
				olmNodeName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", olmNamespace, olmPodFullName, "-o=jsonpath={.spec.nodeName}").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				olmErrs = false
				// e2e.Logf("Found pod is %v", pod)
				break
			}
		}
		if olmErrs {
			e2e.Failf("Unable to find the full pod name for %v in %v: %v.", olmPodName, olmNamespace, pods)
		}

		g.By("Query node label value")
		// Look at the setting for the node to be on the master
		olmErrs = true
		olmJpath = fmt.Sprintf("-o=jsonpath={.metadata.labels}")
		nodes, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", olmNamespace, olmNodeName, olmJpath).Output()
		if err != nil {
			e2e.Failf("Unable to query nodes -n %v %v %v.", olmNamespace, err, nodes)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(nodes).To(o.ContainSubstring("node-role.kubernetes.io/master"))

		g.By("look at oc get nodes")
		// Found the setting, verify that it's really on the master node
		msg, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-n", olmNamespace, olmNodeName, "--show-labels", "--no-headers").Output()
		if err != nil {
			e2e.Failf("Unable to query the %v node of pod %v for %v's status", olmNodeName, olmPodFullName, err, msg)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No resources found"))
		status = strings.Fields(msg)
		if strings.Contains(status[2], "master") {
			olmErrs = false
			nodeStatus = true
			e2e.Logf("node %v is a %v", olmNodeName, status[2])
		}
		if olmErrs || !nodeStatus {
			e2e.Failf("The node %v of %v pod is not a master:%v", olmNodeName, olmPodFullName, msg)
		}
		g.By("Finish")
		e2e.Logf("The pod %v is on the master node %v", olmPodFullName, olmNodeName)
	})

})
