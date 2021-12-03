package mco

import (
	"fmt"
	"strings"

	g "github.com/onsi/ginkgo"
	"github.com/onsi/gomega/types"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type ocGetter struct {
	oc        *exutil.CLI
	kind      string
	namespace string
	name      string
}

// Template helps to create resources using openshift templates
type Template struct {
	oc           *exutil.CLI
	templateFile string
}

// Resource will provide the functionality to hanlde general openshift resources
type Resource struct {
	ocGetter
}

// getCommonParams returns the params that are necessary for all commands involving this object
// It returns these 3 params (or 2 if the object is not namespaced): {kind} {resourcename} ({-n} {namespace} only if namespaced)
func (r *ocGetter) getCommonParams() []string {
	params := []string{r.kind}
	if r.name != "" {
		params = append(params, r.name)
	}

	if r.namespace != "" {
		params = append([]string{"-n", r.namespace}, params...)
	}

	return params
}

// Get uses the CLI to retrieve the return value for this jsonpath
func (r *ocGetter) Get(jsonPath string, extraParams ...string) (string, error) {
	params := r.getCommonParams()

	params = append(params, extraParams...)

	params = append(params, []string{"-o", fmt.Sprintf("jsonpath=%s", jsonPath)}...)

	result, err := r.oc.WithoutNamespace().Run("get").Args(params...).Output()

	return result, err
}

// GetSafe uses the CLI to retrieve the return value for this jsonpath, if the resource does not exist, it returns the defaut value
func (r *ocGetter) GetSafe(jsonPath string, defaultValue string, extraParams ...string) string {
	ret, err := r.Get(jsonPath, extraParams...)
	if err != nil {
		return defaultValue
	}

	return ret
}

// GetOrFail uses the CLI to retrieve the return value for this jsonpath, if the resource does not exist, it fails the test
func (r *ocGetter) GetOrFail(jsonPath string, extraParams ...string) string {
	ret, err := r.Get(jsonPath, extraParams...)
	if err != nil {
		e2e.Failf("%v", err)
	}

	return ret
}

// PollValue returns a function suitable to be used with the gomega Eventually/Consistently checks
func (r *ocGetter) Poll(jsonPath string) func() string {
	return func() string {
		ret, _ := r.Get(jsonPath)
		return ret
	}
}

// NewResource constructs a Resource struct for a not-namespaced resource
func NewResource(oc *exutil.CLI, kind string, name string) *Resource {
	return &Resource{ocGetter: ocGetter{oc, kind, "", name}}
}

// NewNamespacedResource constructs a Resource struct for a namespaced resource
func NewNamespacedResource(oc *exutil.CLI, kind string, namespace string, name string) *Resource {
	return &Resource{ocGetter: ocGetter{oc, kind, namespace, name}}
}

// Delete removes the resource from openshift cluster
func (r *Resource) Delete() error {
	params := r.getCommonParams()

	_, err := r.oc.WithoutNamespace().Run("delete").Args(params...).Output()
	if err != nil {
		e2e.Logf("%v", err)
	}

	return err
}

// Exists returns true if the resource exists and false if not
func (r *Resource) Exists() bool {
	_, err := r.Get("{.}")
	return err == nil
}

// String implements the Stringer interface
func (r *Resource) String() string {
	return fmt.Sprintf("<Kind: %s, Name: %s, Namespace: %s>", r.kind, r.name, r.namespace)
}

// Patch patches the resource using the given patch type
// The following patches are exactly the same patch but using different types, 'merge' and 'json'
// --type merge -p '{"spec": {"selector": {"app": "frommergepatch"}}}'
// --type json  -p '[{ "op": "replace", "path": "/spec/selector/app", "value": "fromjsonpatch"}]'
func (r *Resource) Patch(patchType string, patch string) error {
	params := r.getCommonParams()

	params = append(params, []string{"--type", patchType, "-p", patch}...)

	_, err := r.oc.WithoutNamespace().Run("patch").Args(params...).Output()
	if err != nil {
		e2e.Logf("%v", err)
	}

	return err
}

// NewMCOTemplate creates a new template using the MCO fixture directory as the base path of the template file
func NewMCOTemplate(oc *exutil.CLI, fileName string) *Template {
	return &Template{oc: oc, templateFile: generateTemplateAbsolutePath(fileName)}
}

// SetTemplate sets the template file that will be used to create this resource
func (t *Template) SetTemplate(template string) {
	t.templateFile = template
}

// Create the resources defined in the template file
// The template will be created using oc with no namespace (-n NAMESPACE) argument. So if we want to
// create a namespaced resource we need to add the NAMESPACE parameter to the template and
// provide the "-p NAMESPACE" argument to this function.
func (t *Template) Create(parameters ...string) error {
	if t.templateFile == "" {
		return fmt.Errorf("There is no template configured")
	}

	allParams := []string{"--ignore-unknown-parameters=true", "-f", t.templateFile}
	allParams = append(allParams, parameters...)

	exutil.CreateClusterResourceFromTemplate(t.oc, allParams...)

	return nil
}

// ResourceList provides the functionality to handle lists of openshift resources
type ResourceList struct {
	ocGetter
	extraParams []string
}

// NewResourceList constructs a ResourceList struct for not-namespaced resources
func NewResourceList(oc *exutil.CLI, kind string) *ResourceList {
	return &ResourceList{ocGetter{oc.AsAdmin(), kind, "", ""}, []string{}}
}

// NewNamespacedResourceList constructs a ResourceList struct for namespaced resources
func NewNamespacedResourceList(oc *exutil.CLI, kind string, namespace string) *ResourceList {
	return &ResourceList{ocGetter{oc.AsAdmin(), kind, namespace, ""}, []string{}}
}

// SortByTimestamp will configure the list to be sorted by creation timestamp
func (l *ResourceList) SortByTimestamp() {
	l.extraParams = append(l.extraParams, "--sort-by=metadata.creationTimestamp")
}

// ByLabel will use the given label to filter the list
func (l *ResourceList) ByLabel(label string) {
	l.extraParams = append(l.extraParams, fmt.Sprintf("--selector=%s", label))
}

// GetAll returns a list of Resource structs with the resources found in this list
func (l ResourceList) GetAll() ([]Resource, error) {
	allItemsNames, err := l.Get("{.items[*].metadata.name}", l.extraParams...)
	if err != nil {
		e2e.Failf("%v", err)
	}
	allNames := strings.Split(strings.Trim(allItemsNames, " "), " ")

	allResources := []Resource{}
	for _, name := range allNames {
		if name != "" {
			newResource := Resource{ocGetter: ocGetter{l.oc, l.kind, l.namespace, name}}
			allResources = append(allResources, newResource)
		}
	}

	return allResources, nil
}

// Exist returns a gomega matcher that checks if a resource exists or not
func Exist() types.GomegaMatcher {
	return &existMatcher{}
}

type existMatcher struct {
}

func (matcher *existMatcher) Match(actual interface{}) (success bool, err error) {
	resource, ok := actual.(*Resource)
	if !ok {
		return false, fmt.Errorf("Exist matcher expects a Resource in case %v", g.CurrentGinkgoTestDescription().TestText)
	}

	return resource.Exists(), nil
}

func (matcher *existMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected resource\n\t%s\nto exist", actual)
}

func (matcher *existMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected resource\n\t%s\nnot to exist", actual)
}
