package operators

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type Packagemanifest struct {
	Name                    string
	SupportsOwnNamespace    bool
	SupportsSingleNamespace bool
	SupportsAllNamespaces   bool
	CsvVersion              string
	Namespace               string
	DefaultChannel          string
	CatalogSource           string
	CatalogSourceNamespace  string
}

var CertifiedOperators = []string{"3scale-community-operator", "amq-streams", "appdynamics-operator",
	"argocd-operator", "cert-utils-operator", "couchbase-enterprise", "dotscience-operator",
	"jaeger", "keycloak-operator", "kiali", "mongodb-enterprise", "must-gather-operator",
	"percona-server-mongodb-operator", "percona-xtradb-cluster-operator", "planetscale",
	"portworx", "postgresql", "presto-operator", "prometheus", "radanalytics-spark",
	"resource-locker-operator", "spark-gcp", "storageos", "strimzi-kafka-operator",
	"syndesis", "tidb-operator"}
var CatalogLabels = []string{"certified-operators", "redhat-operators", "community-operators"}
var BasicPrefix = "[Basic]"

var _ = g.Describe("[Suite:openshift/operators]", func() {

	var (
		oc                      = exutil.NewCLI("operator", exutil.KubeConfigPath())
		output, _               = oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("get").Args("packagemanifest", "-l catalog="+CatalogLabels[0], "-o=jsonpath={range .items[*]}{.metadata.labels.catalog}:{.metadata.name}{'\\n'}{end}").Output()
		certifiedPackages       = strings.Split(output, "\n")
		output2, _              = oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("get").Args("packagemanifest", "-l catalog="+CatalogLabels[1], "-o=jsonpath={range .items[*]}{.metadata.labels.catalog}:{.metadata.name}{'\\n'}{end}").Output()
		redhatOperatorsPackages = strings.Split(output2, "\n")
		output3, _              = oc.AsAdmin().WithoutNamespace().NotShowInfo().Run("get").Args("packagemanifest", "-l catalog="+CatalogLabels[2], "-o=jsonpath={range .items[*]}{.metadata.labels.catalog}:{.metadata.name}{'\\n'}{end}").Output()
		communityPackages       = strings.Split(output3, "\n")
		packages1               = append(certifiedPackages, redhatOperatorsPackages...)
		allPackages             = append(packages1, communityPackages...)
		currentPackage          Packagemanifest
	)
	defer g.GinkgoRecover()

	for i := range allPackages {

		operator := allPackages[i]
		packageSplitted := strings.Split(operator, ":")

		if len(packageSplitted) > 1 {
			packageName := packageSplitted[1]

			g.It(TestCaseName(packageName, BasicPrefix), func() {
				g.By("by installing", func() {
					currentPackage = CreateSubscription(packageName, oc)
					CheckDeployment(currentPackage, oc)
				})
				g.By("by uninstalling", func() {
					RemoveOperatorDependencies(currentPackage, oc, true)
				})
			})
		}

	}

})

func TestCaseName(operator string, initialPrefix string) string {
	suffix := " should work properly"
	prefix := " Operator "
	return initialPrefix + prefix + operator + suffix
}

func IsCertifiedOperator(operator string) bool {
	if contains(CertifiedOperators, operator) {
		return true
	}
	return false
}

func contains(s []string, searchterm string) bool {
	i := sort.SearchStrings(s, searchterm)
	return i < len(s) && s[i] == searchterm
}

func checkOperatorInstallModes(p Packagemanifest, oc *exutil.CLI) Packagemanifest {
	supportsAllNamespaces, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", p.Name, "-o=jsonpath={.status.channels[?(.name=='"+p.DefaultChannel+"')].currentCSVDesc.installModes[?(.type=='AllNamespaces')].supported}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	supportsAllNamespacesAsBool, _ := strconv.ParseBool(supportsAllNamespaces)

	supportsSingleNamespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", p.Name, "-o=jsonpath={.status.channels[?(.name=='"+p.DefaultChannel+"')].currentCSVDesc.installModes[?(.type=='SingleNamespace')].supported}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	supportsSingleNamespaceAsBool, _ := strconv.ParseBool(supportsSingleNamespace)

	supportsOwnNamespace, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", p.Name, "-o=jsonpath={.status.channels[?(.name=='"+p.DefaultChannel+"')].currentCSVDesc.installModes[?(.type=='OwnNamespace')].supported}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	supportsOwnNamespaceAsBool, _ := strconv.ParseBool(supportsOwnNamespace)

	p.SupportsAllNamespaces = supportsAllNamespacesAsBool
	p.SupportsSingleNamespace = supportsSingleNamespaceAsBool
	p.SupportsOwnNamespace = supportsOwnNamespaceAsBool
	return p
}

func CreatePackageManifest(operator string, oc *exutil.CLI) Packagemanifest {
	msg, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", operator, "-o=jsonpath={.status.catalogSource}:{.status.catalogSourceNamespace}:{.status.defaultChannel}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	packageData := strings.Split(msg, ":")
	p := Packagemanifest{CatalogSource: packageData[0], CatalogSourceNamespace: packageData[1], DefaultChannel: packageData[2], Name: operator}

	csvVersion, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("packagemanifest", p.Name, "-o=jsonpath={.status.channels[?(.name=='"+p.DefaultChannel+"')].currentCSV}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	p.CsvVersion = csvVersion

	p = checkOperatorInstallModes(p, oc)
	return p
}
func CreateSubscription(operator string, oc *exutil.CLI) Packagemanifest {
	p := CreatePackageManifest(operator, oc)
	if p.SupportsSingleNamespace || p.SupportsOwnNamespace {
		p = CreateNamespace(p, oc)
		CreateOperatorGroup(p, oc)
	} else if p.SupportsAllNamespaces {
		p.Namespace = "openshift-operators"

	} else {
		g.Skip("Install Modes AllNamespaces and  SingleNamespace are disabled for Operator: " + operator)
	}

	templateSubscriptionYAML := writeSubscription(p)
	_, err := oc.SetNamespace(p.Namespace).AsAdmin().Run("create").Args("-f", templateSubscriptionYAML).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return p
}

func CreateSubscriptionSpecificNamespace(operator string, oc *exutil.CLI, namespaceCreate bool, operatorGroupCreate bool, namespace string) Packagemanifest {
	p := CreatePackageManifest(operator, oc)
	p.Namespace = namespace
	if namespaceCreate {
		CreateNamespace(p, oc)
	}
	if operatorGroupCreate {
		CreateOperatorGroup(p, oc)
	}
	templateSubscriptionYAML := writeSubscription(p)
	_, err := oc.SetNamespace(p.Namespace).AsAdmin().Run("create").Args("-f", templateSubscriptionYAML).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return p
}

func CreateNamespace(p Packagemanifest, oc *exutil.CLI) Packagemanifest {
	if p.Namespace == "" {
		p.Namespace = names.SimpleNameGenerator.GenerateName("test-operators-")
	}
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("ns", p.Namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return p
}

func RemoveNamespace(namespace string, oc *exutil.CLI) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("ns", namespace).Output()

	if err == nil {
		_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("ns", namespace).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func CreateOperatorGroup(p Packagemanifest, oc *exutil.CLI) {

	templateOperatorGroupYAML := writeOperatorGroup(p.Namespace)
	_, err := oc.SetNamespace(p.Namespace).AsAdmin().Run("create").Args("-f", templateOperatorGroupYAML).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func writeOperatorGroup(namespace string) (templateOperatorYAML string) {
	operatorBaseDir := exutil.FixturePath("testdata", "operators")
	operatorGroupYAML := filepath.Join(operatorBaseDir, "operator_group.yaml")
	fileOperatorGroup, _ := os.Open(operatorGroupYAML)
	operatorGroup, _ := ioutil.ReadAll(fileOperatorGroup)
	operatorGroupTemplate := string(operatorGroup)
	templateOperatorYAML = strings.ReplaceAll(operatorGroupYAML, "operator_group.yaml", "operator_group_"+namespace+"_.yaml")
	operatorGroupString := strings.ReplaceAll(operatorGroupTemplate, "$OPERATOR_NAMESPACE", namespace)
	ioutil.WriteFile(templateOperatorYAML, []byte(operatorGroupString), 0644)
	return
}

func writeSubscription(p Packagemanifest) (templateSubscriptionYAML string) {
	operatorBaseDir := exutil.FixturePath("testdata", "operators")
	subscriptionYAML := filepath.Join(operatorBaseDir, "subscription.yaml")
	fileSubscription, _ := os.Open(subscriptionYAML)
	subscription, _ := ioutil.ReadAll(fileSubscription)
	subscriptionTemplate := string(subscription)

	templateSubscriptionYAML = strings.ReplaceAll(subscriptionYAML, "subscription.yaml", "subscription_"+p.CsvVersion+"_.yaml")
	operatorSubscription := strings.ReplaceAll(subscriptionTemplate, "$OPERATOR_PACKAGE_NAME", p.Name)
	operatorSubscription = strings.ReplaceAll(operatorSubscription, "$OPERATOR_CHANNEL", "\""+p.DefaultChannel+"\"")
	operatorSubscription = strings.ReplaceAll(operatorSubscription, "$OPERATOR_NAMESPACE", p.Namespace)
	operatorSubscription = strings.ReplaceAll(operatorSubscription, "$OPERATOR_SOURCE", p.CatalogSource)
	operatorSubscription = strings.ReplaceAll(operatorSubscription, "$OPERATOR_CATALOG_NAMESPACE", p.CatalogSourceNamespace)
	operatorSubscription = strings.ReplaceAll(operatorSubscription, "$OPERATOR_CURRENT_CSV_VERSION", p.CsvVersion)
	ioutil.WriteFile(templateSubscriptionYAML, []byte(operatorSubscription), 0644)
	e2e.Logf("Subscription: %s", operatorSubscription)
	return
}
func CheckDeployment(p Packagemanifest, oc *exutil.CLI) {
	poolErr := wait.Poll(10*time.Second, 300*time.Second, func() (bool, error) {
		msg, _ := oc.SetNamespace(p.Namespace).AsAdmin().Run("get").Args("csv", p.CsvVersion, "-o=jsonpath={.status.phase}").Output()
		if strings.Contains(msg, "Succeeded") {
			return true, nil
		}
		return false, nil
	})
	if poolErr != nil {
		RemoveOperatorDependencies(p, oc, false)
		g.Fail("Could not obtain CSV:" + p.CsvVersion)
	}
}

func RemoveOperatorDependencies(p Packagemanifest, oc *exutil.CLI, checkDeletion bool) {
	ip, _ := oc.SetNamespace(p.Namespace).AsAdmin().Run("get").Args("sub", p.Name, "-o=jsonpath={.status.installplan.name}").Output()
	e2e.Logf("IP: %s", ip)
	if len(strings.TrimSpace(ip)) > 0 {
		msg, _ := oc.SetNamespace(p.Namespace).AsAdmin().Run("get").Args("installplan", ip, "-o=jsonpath={.spec.clusterServiceVersionNames}").Output()
		msg = strings.ReplaceAll(msg, "[", "")
		msg = strings.ReplaceAll(msg, "]", "")
		e2e.Logf("CSVS: %s", msg)
		csvs := strings.Split(msg, " ")
		for i := range csvs {
			e2e.Logf("CSV_: %s", csvs[i])
			msg, err := oc.SetNamespace(p.Namespace).AsAdmin().Run("delete").Args("csv", csvs[i]).Output()
			if checkDeletion {
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(msg).To(o.ContainSubstring("deleted"))
			}
		}

		subsOutput, _ := oc.SetNamespace(p.Namespace).AsAdmin().Run("get").Args("subs", "-o=jsonpath={range .items[?(.status.installplan.name=='"+ip+"')].metadata}{.name}{' '}").Output()
		e2e.Logf("SUBS OUTPUT: %s", subsOutput)
		if len(strings.TrimSpace(subsOutput)) > 0 {
			subs := strings.Split(subsOutput, " ")
			e2e.Logf("SUBS: %s", subs)
			for i := range subs {
				e2e.Logf("SUB_: %s", subs[i])
				msg, err := oc.SetNamespace(p.Namespace).AsAdmin().Run("delete").Args("subs", subs[i]).Output()
				if checkDeletion {
					o.Expect(err).NotTo(o.HaveOccurred())
					o.Expect(msg).To(o.ContainSubstring("deleted"))
				}
			}
		}
	}
	if p.SupportsSingleNamespace || p.SupportsOwnNamespace {
		RemoveNamespace(p.Namespace, oc)
	}

}

func itemExists(arrayType interface{}, item interface{}) bool {
	arr := reflect.ValueOf(arrayType)

	if arr.Kind() != reflect.Array {
		panic("Invalid data-type")
	}

	for i := 0; i < arr.Len(); i++ {
		if arr.Index(i).Interface() == item {
			return true
		}
	}

	return false
}
