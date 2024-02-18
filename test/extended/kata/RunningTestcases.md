
## Running Openshift Sandboxed Containers (OSC) test cases

All the tests are under the *[sig-kata]* tag.  Default values are hard coded but can be changed (see [with a configmap](#change-the-test-defaults-with-a-configmap)).

Running the tests where you have logged into a cluster:
```
bin/extended-platform-tests run all --dry-run | egrep 'sig-kata' | bin/extended-platform-tests run --timeout 120m -f -
```

This will subscribe to the *Openshift sandbox container Operator* from the *redhat-operators* catalog that exists in the cluster and create *kataconfig* with *kata* runtime.

The OSC test code is part of the https://github.com/openshift/openshift-tests-private repo. From the top directory, `openshift-tests-private`, kata code is in  `test/extended/kata` and the templates are in `test/extended/testdata/kata`.

Tests run using the chosen `runtimeClass` for *kataconfig* and in matching workloads. Tests that do not apply to the `runtimeClass` chosen will be skipped.  There are skips in the code for other reasons as well.
To see all the tests and if the code can skip them:
`egrep 'g.It|g.Skip|skipMessage|msg = fmt.Sprintf' test/extended/kata/kata.go`

### Choosing tests to run
Instead of using `egrep 'sig-kata'`, you can list all the test numbers in the regex.
examples:
- `egrep '66108|43516'`
  - check the CSV version and see if the operator is in the catalog
- `egrep 'sig-kata | egrep -iv '43523|41813|upgrade'`
  - run the full suite, excluding upgrade and the 2 tests that delete kataconfig



### Change the test defaults with a configmap

To override the hard coded defaults, a *configmap* named `osc-config` in the `default` *namespace* is used.

A template `testrun-cm-template.yaml` is used to create a *configmap* with `oc process`.  It is named **osc-config**.  To use the template in the example below, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`

Process:
```
oc process --ignore-unknown-parameters=true -f $L/testrun-cm-template.yaml -p OSCCHANNEL=stable NAME=osc-config NAMESPACE=default  CATSOURCENAME=<your-catalog> ICSPNEEDED=true  RUNTIMECLASSNAME=kata ENABLEPEERPODS=false OPERATORVER=1.4.1-GA  -o yaml > osc-config.yaml

oc apply -f osc-config.yaml
```

Values not specified will be the defaults from the template. The `CATSOURCENAME` could be `redhat-operators` to use the GA version or a custom catalog created as [below](#create-a-catalog-with-the-starting-version).
`OPERATORVER` is compared to the csv's version in test 66108.

### Change the test defaults with environment variables
This was not being used and environment variables have been removed from the code.





## Upgrading
An upgrade should be done in its own testrun.  If it is combined with other tests, the tests will be in random order.  Therefore, you should create your cluster, run the upgrade test by itself and then run the full suite.

When the OLM detects the *catalog* the subscription is using has a new version, an upgrade will happen. Operators are set for automatic upgrade by default.  The subscription can have *Update Approval* set to **Manual** to prevent OLM from doing the upgrade automatically.

During this update, the new operator version will be subscribed to and the current operator will be deleted.  The subscription is otherwise unchanged

To do the upgrade, you will need a catalog where you control the image index.

### Image indexes for catalogs
The OSC build process creates an image index in quay.  You can view the tags in the repo with `skopeo list-tags docker://quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog`

Unreleased versions with have a -number from the Nth build of that version.  ex: *1.5.2-6, 1.5.2-8, 1.6.0-1, 1.6.0-8*.  Past GA versions have been tagged with the GA version.  ex: *1.4.1, 1.5.0, 1.5.1*.  Future GA versions will need to manually or automatically tag the last build without the -build number.

### Create a catalog with the starting version
To use the template example below, set **L** to point to your copy of the git repo and where the templates are.  Ex: `L=~/go/src/github.com/tbuskey/openshift-tests-private/test/extended/testdata/kata`

example: _your-index_ is `quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.1`

```
oc process --ignore-unknown-parameters=true -f /catalogSourceTemplate.yaml -p NAME=_your-catalog_ IMAGEINDEX=_your index_ PUBLISHER=_your name_ DISPLAYNAME=_your name_  > catalog.yaml

oc apply -f catalog.yaml
```

In the operator hub you will see *OpenShift sandboxed containers Operator* with **your name** displayed from your **your-catalog**. When you subscribe to the operator, it will have **your-index** There will also be an operator with **Red Hat** displayed from the **redhat-operators** catalog.


##### example catalogsource created with the template
```
{
    "kind": "List",
    "apiVersion": "v1",
    "metadata": {},
    "items": [
        {
            "apiVersion": "operators.coreos.com/v1alpha1",
            "kind": "CatalogSource",
            "metadata": {
                "name": "your-catalog",
                "namespace": "openshift-marketplace"
            },
            "spec": {
                "displayName": "your name",
                "image": "quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.1",
                "publisher": "your name",
                "sourceType": "grpc"
            }
        }
    ]
}
```

### Create an `osc-config` using the catalog
[Use a configmap](#change-the-test-defaults-with-a-configmap) with _your-catalog_ as the CATSOURCENAME

### Create the `osc-config-upgrade-catalog` configmap
The catalog that osc-config has for CATSOURCENAME will have its IMAGEINDEX changed to the one from the `osc-config-upgrade-catalog` configmap. To create the upgrade config map:

example: _your-next-index_ is `quay.io/openshift_sandboxed_containers/openshift-sandboxed-containers-operator-catalog:1.5.2-8`

```
oc process --ignore-unknown-parameters=true -f testrunUpgradeCatalogImage.yaml -p IMAGEINDEXAFTER=_your-next-index_ > osc-config-upgrade-catalog.yaml

oc apply -f osc-config-upgrade-catalog.yaml
```

### Run the upgrade
`bin/extended-platform-tests run all --dry-run | egrep '70824' | bin/extended-platform-tests run --timeout 120m -f - `

If the operator is not installed, this will install the starting version in `g.BeforeEach()` and then upgrade to the new version.  Otherwise, the existing operator will be upgraded.

### Delete the `osc-config-upgrade-catalog` configmap before testing more
You _can_ run the rest of the tests with this configmap in place but it will slow the run down.  The upgrade test waits to see if the CSV changes and times out with a failure if it does not change. To avoid this, the `osc-config-upgrade-catalog` configmap should be deleted: `oc delete -n default cm osc-config-upgrade-catalog`

### Before running further tests
The `osc-config` configmap should be updated with `operatorVer` set to _your-next-index_'s version so the `66108-Version in operator CSV should match expected version` test passes.

## Changing the *Subscription* and channel for an upgrade
This is not how Openshift does upgrades.  Early on, we used a channel in OSC for each version.  We upgraded by changing the subscription.

Changing the channel is a different process.  The operator only has the `stable` channel after version 1.3.

The "*Longduration-Author:tbuskey-High-53583-upgrade osc operator by changing subscription [Disruptive][Serial]*" testcase.

To run this test, you would install the operator with an older channel. The hard coded settings might do this or you can use the `osc-config`.  This will run all the other test cases with those values.

The channel change uses the `osc-config-upgrade-subscription`  *configmap* created with the `testrun-cm-template.yaml` [as shown](#change-the-test-defaults-with-a-configmap).  If the configmap does not exist, the test will be skipped.


[comment]: # (Spell check with aspell test/extended/kata/RunningTestcases.md )