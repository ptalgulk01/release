package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	"github.com/tidwall/gjson"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var (
		oc                               = exutil.NewCLI("storage-operators", exutil.KubeConfigPath())
		cloudProviderSupportProvisioners []string
	)

	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		// Function to check optional enabled capabilities
		checkOptionalCapability(oc, "Storage")
		cloudProviderSupportProvisioners = getSupportProvisionersByCloudProvider(oc)
	})

	// author: wduan@redhat.com
	// OCP-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity
	g.It("ROSA-OSD_CCS-ARO-Author:wduan-High-66532-[CSI-Driver-Operator] Check Azure-Disk and Azure-File CSI-Driver-Operator configuration on manual mode with Azure Workload Identity", func() {

		// Check only on Azure cluster with manual credentialsMode
		credentialsMode, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cloudcredentials/cluster", "-o=jsonpath={.spec.credentialsMode}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		serviceAccountIssuer, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("authentication/cluster", "-o=jsonpath={.spec.serviceAccountIssuer}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		// Temporarily fix by checking serviceAccountIssuer
		if cloudProvider != "azure" || credentialsMode != "Manual" || serviceAccountIssuer == "" {
			g.Skip("This case is only applicable for Azure cluster with Manual credentials mode, skipped")
		}

		// Check the azure_federated_token_file is present in azure-disk-credentials/azure-file-credentials secret, while azure_client_secret is not present in secret.
		secrets := []string{"azure-disk-credentials", "azure-file-credentials"}
		for _, secret := range secrets {
			e2e.Logf("Checking secret: %s", secret)
			secretData, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-csi-drivers", "secret", secret, "-o=jsonpath={.data}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(strings.Contains(secretData, "azure_federated_token_file")).To(o.BeTrue())
			o.Expect(strings.Contains(secretData, "azure_client_secret")).NotTo(o.BeTrue())
		}

		// Check the --enable-azure-workload-identity=true in controller definition
		deployments := []string{"azure-disk-csi-driver-controller", "azure-file-csi-driver-controller"}
		for _, deployment := range deployments {
			e2e.Logf("Checking deployment: %s", deployment)
			args, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-cluster-csi-drivers", "deployment", deployment, "-o=jsonpath={.spec.template.spec.initContainers[?(@.name==\"azure-inject-credentials\")].args}}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(args).To(o.ContainSubstring("enable-azure-workload-identity=true"))
		}

	})

	// author: pewang@redhat.com
	// OCP-64793-[CSI-Driver-Operator] should restart driver controller Pods if CA certificates are updated
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-ROSA-OSD_CCS-ARO-Author:pewang-High-64793-[CSI-Driver-Operator] should restart driver controller Pods if CA certificates are updated [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "pd.csi.storage.gke.io", "disk.csi.azure.com", "file.csi.azure.com", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			csiOperatorNs       = "openshift-cluster-csi-drivers"
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		type operatorAndCert struct {
			metricsCertSecret string
			driverOperator    deployment
		}

		var myTester = map[string][]operatorAndCert{
			"ebs.csi.aws.com":              {{"aws-ebs-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("aws-ebs-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=aws-ebs-csi-driver-controller"))}},
			"efs.csi.aws.com":              {{"aws-efs-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("aws-efs-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=aws-efs-csi-driver-controller"))}},
			"disk.csi.azure.com":           {{"azure-disk-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("azure-disk-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=azure-disk-csi-driver-controller"))}},
			"file.csi.azure.com":           {{"azure-file-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("azure-file-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=azure-file-csi-driver-controller"))}},
			"pd.csi.storage.gke.io":        {{"gcp-pd-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("gcp-pd-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=gcp-pd-csi-driver-controller"))}},
			"filestore.csi.storage.gke.io": {{"gcp-filestore-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("gcp-filestore-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=gcp-filestore-csi-driver-controller"))}},
			"csi.vsphere.vmware.com": {{"vmware-vsphere-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("vmware-vsphere-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=vmware-vsphere-csi-driver-controller"))},
				{"vmware-vsphere-csi-driver-webhook-secret", newDeployment(setDeploymentName("vmware-vsphere-csi-driver-webhook"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=vmware-vsphere-csi-driver-webhook"))}},
			"csi.sharedresource.openshift.io": {{"shared-resource-csi-driver-webhook-serving-cert", newDeployment(setDeploymentName("shared-resource-csi-driver-webhook"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("name=shared-resource-csi-driver-webhook"))},
				{"shared-resource-csi-driver-node-metrics-serving-cert", newDeployment(setDeploymentName("shared-resource-csi-driver-node"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=shared-resource-csi-driver-node"))}},
			"diskplugin.csi.alibabacloud.com": {{"alibaba-disk-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("alibaba-disk-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=alibaba-disk-csi-driver-controller"))}},

			// The follow provisioners covered by other teams not our CI, only define them but not add to test list, will add to test list when it is needed
			"cinder.csi.openstack.org":  {{"openstack-cinder-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("openstack-cinder-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=openstack-cinder-csi-driver-controller"))}},
			"manila.csi.openstack.org ": {{"manila-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("openstack-manila-csi-controllerplugin"), setDeploymentNamespace("openshift-manila-csi-driver"), setDeploymentApplabel("app=openstack-manila-csi-controllerplugin"))}},
			"powervs.csi.ibm.com":       {{"ibm-powervs-block-csi-driver-controller-metrics-serving-cert", newDeployment(setDeploymentName("ibm-powervs-block-csi-driver-controller"), setDeploymentNamespace(csiOperatorNs), setDeploymentApplabel("app=ibm-powervs-block-csi-driver-controller"))}},
		}

		// Currently only sharedresource csi driver(available for all platforms) is still TP in 4.14, it will auto installed on TechPreviewNoUpgrade clusters
		if checkCSIDriverInstalled(oc, []string{"csi.sharedresource.openshift.io"}) {
			supportProvisioners = append(supportProvisioners, "csi.sharedresource.openshift.io")
		}

		for _, provisioner = range supportProvisioners {
			func() {

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

				// Make sure the cluster storage operator recover healthy again whether the case passed or failed
				defer waitCSOhealthy(oc.AsAdmin())

				for i := 0; i < len(myTester[provisioner]); i++ {

					// The shared-resource-csi-driver-node-metrics-serving-cert is used by shared-resource-csi-driver-node daemonset
					if provisioner == "csi.sharedresource.openshift.io" && myTester[provisioner][i].metricsCertSecret == "shared-resource-csi-driver-node-metrics-serving-cert" {
						exutil.By("# Get the origin shared-resource csi driver node pod name")
						csiDriverNode := newDaemonSet(setDsName("shared-resource-csi-driver-node"), setDsNamespace(csiOperatorNs), setDsApplabel("app=shared-resource-csi-driver-node"))
						metricsCert := myTester[provisioner][i].metricsCertSecret
						resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("ds", csiDriverNode.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
						o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

						exutil.By("# Delete the metrics-serving-cert secret and wait csi driver node pods ready again ")
						// The secret will added back by the service-ca-operator
						o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", csiOperatorNs, "secret/"+metricsCert).Execute()).NotTo(o.HaveOccurred())

						o.Eventually(func() string {
							resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("ds", csiDriverNode.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
							o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
							return resourceVersionNew
						}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

						csiDriverNode.waitReady(oc.AsAdmin())
					} else {
						exutil.By("# Get the origin csi driver controller pod name")
						csiDriverController := myTester[provisioner][i].driverOperator
						metricsCert := myTester[provisioner][i].metricsCertSecret
						csiDriverController.replicasno = csiDriverController.getReplicasNum(oc.AsAdmin())
						originPodList := csiDriverController.getPodList(oc.AsAdmin())
						resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiDriverController.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
						o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

						exutil.By("# Delete the metrics-serving-cert secret and wait csi driver controller ready again ")
						// The secret will added back by the service-ca-operator
						o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", csiOperatorNs, "secret/"+metricsCert).Execute()).NotTo(o.HaveOccurred())

						o.Eventually(func() string {
							resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiDriverController.name, "-n", csiOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
							o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
							return resourceVersionNew
						}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

						csiDriverController.waitReady(oc.AsAdmin())
						waitCSOhealthy(oc.AsAdmin())
						newPodList := csiDriverController.getPodList(oc.AsAdmin())

						exutil.By("# Check pods are different with original pods")
						o.Expect(len(sliceIntersect(originPodList, newPodList))).Should(o.Equal(0))
					}

				}

				if provisioner != "csi.sharedresource.openshift.io" {

					exutil.By("# Create new project verify")
					oc.SetupProject()

					pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)))
					pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

					exutil.By("# Create a pvc with the preset csi storageclass")
					pvc.create(oc)
					defer pvc.deleteAsAdmin(oc)

					exutil.By("# Create pod with the created pvc and wait for the pod ready")
					pod.create(oc)
					defer pod.deleteAsAdmin(oc)
					pod.waitReady(oc)

				}

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

			}()

		}
	})

	// author: pewang@redhat.com
	// https://issues.redhat.com/browse/STOR-1443
	// OCP-66529-[Cluster-CSI-Snapshot-Controller-Operator] should restart webhook Pods if csi-snapshot-webhook-secret changed [Disruptive]
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-Author:pewang-High-66529-[Cluster-CSI-Snapshot-Controller-Operator] should restart webhook Pods if csi-snapshot-webhook-secret changed [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "filestore.csi.storage.gke.io"}
		// Set the resource template for the scenario
		var (
			supportProvisioners         = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
			ccscOperatorNs              = "openshift-cluster-storage-operator"
			webhookSecretName           = "csi-snapshot-webhook-secret"
			csiSnapshotWebhook          = newDeployment(setDeploymentName("csi-snapshot-webhook"), setDeploymentNamespace(ccscOperatorNs), setDeploymentApplabel("app=csi-snapshot-webhook"))
			storageTeamBaseDir          = exutil.FixturePath("testdata", "storage")
			pvcTemplate                 = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate                 = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			pvcOri                      = newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podOri                      = newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
			volumesnapshotTemplate      = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeSnapshotClassTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshotclass-template.yaml")
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Skip if CSISnapshot CO is not enabled
		if !isEnabledCapability(oc, "CSISnapshot") {
			g.Skip("Skip for CSISnapshot capability is not enabled on the test cluster!")
		}
		if strSliceContains(cloudProviderSupportProvisioners, "csi.vsphere.vmware.com") {
			mo := newMonitor(oc.AsAdmin())
			vcenterVersion, getvCenterVersionErr := mo.getSpecifiedMetricValue("vsphere_vcenter_info", `data.result.0.metric.version`)
			o.Expect(getvCenterVersionErr).NotTo(o.HaveOccurred())
			esxiVersion, getEsxiVersionErr := mo.getSpecifiedMetricValue("vsphere_esxi_version_total", `data.result.0.metric.version`)
			o.Expect(getEsxiVersionErr).NotTo(o.HaveOccurred())
			// Snapshot feature on vSphere needs both vCenter version and Esxi version at least 7.0.3
			if !versionIsAbove(vcenterVersion, "7.0.2") || !versionIsAbove(esxiVersion, "7.0.2") {
				g.Skip("Skip for the test cluster vCenter version \"" + vcenterVersion + "\" not support snapshot!!!")
			}
		}

		exutil.By("# Create new project verify")
		oc.SetupProject()

		for _, provisioner = range supportProvisioners {
			func() {

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")

				// Make sure the cluster storage operator recover healthy again whether the case passed or failed
				defer waitCSOhealthy(oc.AsAdmin())

				for _, provisioner = range supportProvisioners {

					exutil.By("# Get the origin csi-snapshot-webhook pod name")
					csiSnapshotWebhook.replicasno = csiSnapshotWebhook.getReplicasNum(oc.AsAdmin())
					originPodList := csiSnapshotWebhook.getPodList(oc.AsAdmin())
					resourceVersionOri, resourceVersionOriErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiSnapshotWebhook.name, "-n", ccscOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
					o.Expect(resourceVersionOriErr).ShouldNot(o.HaveOccurred())

					exutil.By("# Delete the csi-snapshot-webhook-secret secret and wait snapshot-webhook ready again ")
					// The secret will added back by the service-ca-operator
					o.Expect(oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ccscOperatorNs, "secret/"+webhookSecretName).Execute()).NotTo(o.HaveOccurred())

					o.Eventually(func() string {
						resourceVersionNew, resourceVersionNewErr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", csiSnapshotWebhook.name, "-n", ccscOperatorNs, "-o=jsonpath={.metadata.resourceVersion}").Output()
						o.Expect(resourceVersionNewErr).ShouldNot(o.HaveOccurred())
						return resourceVersionNew
					}, 120*time.Second, 5*time.Second).ShouldNot(o.Equal(resourceVersionOri))

					csiSnapshotWebhook.waitReady(oc.AsAdmin())
					waitCSOhealthy(oc.AsAdmin())
					newPodList := csiSnapshotWebhook.getPodList(oc.AsAdmin())

					exutil.By("# Check pods are different with original pods")
					o.Expect(len(sliceIntersect(originPodList, newPodList))).Should(o.Equal(0))

				}

				// Check after the webhook Pods restarted the snapshot function should be still worked well
				exutil.By("Create a pvc with the preset csi storageclass")
				pvcOri.scname = getPresetStorageClassNameByProvisioner(oc, cloudProvider, provisioner)
				pvcOri.create(oc)
				defer pvcOri.deleteAsAdmin(oc)

				exutil.By("Create pod with the created pvc and wait for the pod ready")
				podOri.create(oc)
				defer podOri.deleteAsAdmin(oc)
				podOri.waitReady(oc)

				exutil.By("Write file to volume")
				podOri.checkMountedVolumeCouldRW(oc)
				podOri.execCommand(oc, "sync")

				// Create volumesnapshot with pre-defined volumesnapshotclass
				exutil.By("Create volumesnapshot and wait for ready_to_use")
				var presetVscName string
				if provisioner == "filestore.csi.storage.gke.io" {
					volumesnapshotClass := newVolumeSnapshotClass(setVolumeSnapshotClassTemplate(volumeSnapshotClassTemplate), setVolumeSnapshotClassDriver(provisioner), setVolumeSnapshotDeletionpolicy("Delete"))
					volumesnapshotClass.create(oc)
					defer volumesnapshotClass.deleteAsAdmin(oc)
					presetVscName = volumesnapshotClass.name

				} else {
					presetVscName = getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
				}
				volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(presetVscName))
				volumesnapshot.create(oc)
				defer volumesnapshot.delete(oc)
				volumesnapshot.waitReadyToUse(oc)

				exutil.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

			}()

		}
	})

	// author: wduan@redhat.com
	// OCP-70338-[CSI-Driver-Operator] TLSSecurityProfile setting for Kube RBAC cipher suites
	g.It("NonHyperShiftHOST-ROSA-OSD_CCS-ARO-NonPreRelease-Longduration-Author:wduan-Medium-70338-[CSI-Driver-Operator] TLSSecurityProfile setting for Kube RBAC cipher suites. [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "efs.csi.aws.com", "disk.csi.azure.com", "file.csi.azure.com", "pd.csi.storage.gke.io", "filestore.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "cinder.csi.openstack.org"}
		var (
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, cloudProviderSupportProvisioners)
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		type TLSSecurityProfile struct {
			profileType     string
			patchCmd        string
			expectedCipher  string
			expectedVersion string
		}
		// In 4.15 cluster, there is no tlsSecurityProfile defined in apiserver/cluster, it will use the same config with Intermediate mode as below
		// So test case will first check if storage components follow the default setting
		var TLSProfileDefault TLSSecurityProfile = TLSSecurityProfile{
			profileType:     "default",
			patchCmd:        `[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value":}]`,
			expectedCipher:  `["TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384","TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"]`,
			expectedVersion: "VersionTLS12",
		}

		// In this test, will change to custom type to check storage components follow the change
		var TLSProfileCustom TLSSecurityProfile = TLSSecurityProfile{
			profileType:     "custom",
			patchCmd:        `[{"op": "add", "path": "/spec/tlsSecurityProfile", "value":{"custom":{"ciphers":["ECDHE-ECDSA-CHACHA20-POLY1305","ECDHE-RSA-CHACHA20-POLY1305","ECDHE-RSA-AES128-GCM-SHA256","ECDHE-ECDSA-AES128-GCM-SHA256"],"minTLSVersion":"VersionTLS11"},"type":"Custom"}}]`,
			expectedCipher:  `["TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256","TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256","TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"]`,
			expectedVersion: "VersionTLS11",
		}

		// Get origin TLSSecurityProfile in apiserver/cluster for restore
		savedTLSSecurityProfile, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("apiserver/cluster", "-o=jsonpath={.spec.tlsSecurityProfile}").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		// Check the default setting
		for _, provisioner = range supportProvisioners {
			exutil.By("Checking " + cloudProvider + " csi driver: \"" + provisioner + "\" with default setting")
			// Check TLSSecurityProfile with default setting
			verifyTLSInCSIDriver(oc, provisioner, TLSProfileDefault.expectedCipher, TLSProfileDefault.expectedVersion)
			replacer := strings.NewReplacer("[", "", "]", "", `"`, "")
			expectedCipher := replacer.Replace(TLSProfileDefault.expectedCipher)
			verifyTLSInCSIController(oc, provisioner, expectedCipher, TLSProfileDefault.expectedVersion)
		}

		// Apply new TLSSecurityProfile and check
		exutil.By("Patching the apiserver with ciphers type : " + TLSProfileCustom.profileType)
		exeErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", "-p", TLSProfileCustom.patchCmd).Execute()
		o.Expect(exeErr).NotTo(o.HaveOccurred())
		defer func() {
			exutil.By("Restoring apiserver/cluster's ciphers")
			patchCmdRestore := fmt.Sprintf(`--patch=[{"op": "replace", "path": "/spec/tlsSecurityProfile", "value":%s}]`, savedTLSSecurityProfile)
			exeErr := oc.AsAdmin().WithoutNamespace().Run("patch").Args("apiserver/cluster", "--type=json", patchCmdRestore).Execute()
			o.Expect(exeErr).ShouldNot(o.HaveOccurred())
			// wait 1 min to let co delect the changes and wait the key Cluster Operator in healthy status
			// Check clusterversion/version doesn't work
			// Todo: We might need a general function to wait cluster healthy before exiting disruptive test,
			// but the point is do we need to check/wait all CO? Will it cause longer duration for test?
			// Or we need a CO snapshot and at least make sure not worse than original status?
			time.Sleep(60 * time.Second)
			waitCOHealthy(oc, "storage", 120)
			waitCOHealthy(oc, "kube-apiserver", 900)
			// The kube-apiserver is almost the last CO to recover, but still check other and usually it will not wait additional time
			waitCOHealthy(oc, "authentication", 120)
			waitCOHealthy(oc, "etcd", 120)
		}()

		for _, provisioner = range supportProvisioners {
			exutil.By("Checking " + cloudProvider + " csi driver: \"" + provisioner + "\" with new setting")
			verifyTLSInCSIDriver(oc, provisioner, TLSProfileCustom.expectedCipher, TLSProfileCustom.expectedVersion)
			// The outputs from the apiserver and container args are different
			replacer := strings.NewReplacer("[", "", "]", "", `"`, "")
			expectedCipher := replacer.Replace(TLSProfileCustom.expectedCipher)
			verifyTLSInCSIController(oc, provisioner, expectedCipher, TLSProfileCustom.expectedVersion)
		}
	})

})

func verifyTLSInCSIDriver(oc *exutil.CLI, provisioner string, expectedCipher string, expectedVersion string) {
	o.Eventually(func() []string {
		cipherInDriver, operr := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercsidriver", provisioner, "-o=jsonpath={.spec.observedConfig.targetcsiconfig.servingInfo.cipherSuites}").Output()
		o.Expect(operr).ShouldNot(o.HaveOccurred())
		versionInDriver, operr := oc.WithoutNamespace().AsAdmin().Run("get").Args("clustercsidriver", provisioner, "-o=jsonpath={.spec.observedConfig.targetcsiconfig.servingInfo.minTLSVersion}").Output()
		o.Expect(operr).ShouldNot(o.HaveOccurred())
		return []string{cipherInDriver, versionInDriver}
	}, 120*time.Second, 5*time.Second).Should(o.Equal([]string{expectedCipher, expectedVersion}))
}

func verifyTLSInCSIController(oc *exutil.CLI, provisioner string, expectedCipher string, expectedVersion string) {
	// Drivers controller deployment name
	var (
		CSIDriverController = map[string]string{
			"ebs.csi.aws.com":              "aws-ebs-csi-driver-controller",
			"efs.csi.aws.com":              "aws-efs-csi-driver-controller",
			"disk.csi.azure.com":           "azure-disk-csi-driver-controller",
			"file.csi.azure.com":           "azure-file-csi-driver-controller",
			"pd.csi.storage.gke.io":        "gcp-pd-csi-driver-controller",
			"filestore.csi.storage.gke.io": "gcp-filestore-csi-driver-controller",
			"csi.vsphere.vmware.com":       "vmware-vsphere-csi-driver-controller",
			"vpc.block.csi.ibm.io":         "ibm-vpc-block-csi-controller",
			"cinder.csi.openstack.org":     "openstack-cinder-csi-driver-controller",
		}
		// All tested CSI Driver located in the "openshift-cluster-csi-drivers" namespace
		CSIDriverNS string = "openshift-cluster-csi-drivers"
		cipher      string
		version     string
	)
	o.Eventually(func() []string {
		output, operr := oc.WithoutNamespace().AsAdmin().Run("get").Args("deployment", CSIDriverController[provisioner], "-n", CSIDriverNS, "-o=jsonpath={.spec.template.spec}").Output()
		o.Expect(operr).ShouldNot(o.HaveOccurred())
		argsList := gjson.Get(output, "containers.#(name%\"*kube-rbac-proxy*\")#.args").Array()
		for _, args := range argsList {
			for _, arg := range args.Array() {
				if strings.HasPrefix(arg.String(), "--tls-cipher-suites=") {
					cipher = strings.TrimPrefix(arg.String(), "--tls-cipher-suites=")

				}
				if strings.HasPrefix(arg.String(), "--tls-min-version=") {
					version = strings.TrimPrefix(arg.String(), "--tls-min-version=")
				}
			}
		}
		return []string{cipher, version}
	}, 120*time.Second, 5*time.Second).Should(o.Equal([]string{expectedCipher, expectedVersion}))
}
