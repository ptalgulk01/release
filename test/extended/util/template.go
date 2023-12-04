package util

import (
	"fmt"
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// ApplyClusterResourceFromTemplateWithError apply the changes to the cluster resource and return error if happned.
// For ex: ApplyClusterResourceFromTemplateWithError(oc, "--ignore-unknown-parameters=true", "-f", "TEMPLATE LOCATION")
func ApplyClusterResourceFromTemplateWithError(oc *CLI, parameters ...string) error {
	return resourceFromTemplate(oc, false, true, "", parameters...)
}

// ApplyClusterResourceFromTemplate apply the changes to the cluster resource.
// For ex: ApplyClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", "TEMPLATE LOCATION")
func ApplyClusterResourceFromTemplate(oc *CLI, parameters ...string) {
	resourceFromTemplate(oc, false, false, "", parameters...)
}

// ApplyNsResourceFromTemplate apply changes to the ns resource.
// No need to add a namespace parameter in the template file as it can be provided as a function argument.
// For ex: ApplyNsResourceFromTemplate(oc, "NAMESPACE", "--ignore-unknown-parameters=true", "-f", "TEMPLATE LOCATION")
func ApplyNsResourceFromTemplate(oc *CLI, namespace string, parameters ...string) {
	resourceFromTemplate(oc, false, false, namespace, parameters...)
}

// CreateClusterResourceFromTemplate create resource from the template.
// For ex: CreateClusterResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", "TEMPLATE LOCATION")
func CreateClusterResourceFromTemplate(oc *CLI, parameters ...string) {
	resourceFromTemplate(oc, true, false, "", parameters...)
}

// CreateNsResourceFromTemplate create ns resource from the template.
// No need to add a namespace parameter in the template file as it can be provided as a function argument.
// For ex: CreateNsResourceFromTemplate(oc, "NAMESPACE", "--ignore-unknown-parameters=true", "-f", "TEMPLATE LOCATION")
func CreateNsResourceFromTemplate(oc *CLI, namespace string, parameters ...string) {
	resourceFromTemplate(oc, true, false, namespace, parameters...)
}

func resourceFromTemplate(oc *CLI, create bool, returnError bool, namespace string, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		fileName := GetRandomString() + "config.json"
		stdout, _, err := oc.AsAdmin().Run("process").Args(parameters...).OutputsToFiles(fileName)
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}

		configFile = stdout
		return true, nil
	})
	if returnError && err != nil {
		return fmt.Errorf("fail to process %v", parameters)
	}
	AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)

	var resourceErr error
	if create {
		if namespace != "" {
			resourceErr = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile, "-n", namespace).Execute()
		} else {
			resourceErr = oc.AsAdmin().WithoutNamespace().Run("create").Args("-f", configFile).Execute()
		}
	} else {
		if namespace != "" {
			resourceErr = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile, "-n", namespace).Execute()
		} else {
			resourceErr = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
		}
	}
	if returnError && resourceErr != nil {
		return fmt.Errorf("fail to create/apply resource %v", resourceErr)
	}
	AssertWaitPollNoErr(resourceErr, fmt.Sprintf("fail to create/apply resource %v", resourceErr))
	return nil
}

// GetRandomString to create random string
func GetRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[seed.Intn(len(chars))]
	}
	return string(buffer)
}

// ApplyResourceFromTemplateWithNonAdminUser to as normal user to create resource from template
func ApplyResourceFromTemplateWithNonAdminUser(oc *CLI, parameters ...string) error {
	var configFile string
	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(GetRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})
	AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))

	e2e.Logf("the file of resource is %s", configFile)
	return oc.WithoutNamespace().Run("apply").Args("-f", configFile).Execute()
}

// ProcessTemplate process template given file path and parameters
func ProcessTemplate(oc *CLI, parameters ...string) string {
	var configFile string

	err := wait.Poll(3*time.Second, 15*time.Second, func() (bool, error) {
		output, err := oc.Run("process").Args(parameters...).OutputToFile(GetRandomString() + "config.json")
		if err != nil {
			e2e.Logf("the err:%v, and try next round", err)
			return false, nil
		}
		configFile = output
		return true, nil
	})

	AssertWaitPollNoErr(err, fmt.Sprintf("fail to process %v", parameters))
	e2e.Logf("the file of resource is %s", configFile)
	return configFile
}
