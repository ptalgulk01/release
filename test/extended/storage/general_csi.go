package storage

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-storage] STORAGE", func() {
	defer g.GinkgoRecover()
	var oc = exutil.NewCLI("storage-general-csi", exutil.KubeConfigPath())

	// aws-csi test suite cloud provider support check
	g.BeforeEach(func() {
		cloudProvider = getCloudProvider(oc)
		generalCsiSupportCheck(cloudProvider)
	})

	// author: pewang@redhat.com
	// OCP-44903 [CSI Driver] [Dynamic PV] [ext4] volumes should store data and allow exec of files on the volume
	g.It("Author:pewang-High-44903-[CSI Driver] [Dynamic PV] [ext4] volumes should store data and allow exec of files on the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			g.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("3. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			g.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("5. Check the deployment's pod mounted volume fstype is ext4 by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "ext4")

			g.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("8. Check the volume mounted on the pod located node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

			g.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			g.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			g.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			g.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			g.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// [CSI Driver] [Dynamic PV] [Filesystem default] volumes should store data and allow exec of files
	g.It("Author:pewang-Critical-24485-[CSI Driver] [Dynamic PV] [Filesystem default] volumes should store data and allow exec of files", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Use the framework created project as default, if use your own, exec the follow code setupProject
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			g.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("%s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			g.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			g.By("# Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// OCP-44911 -[CSI Driver] [Dynamic PV] [Filesystem] could not write into read-only volume
	g.It("Author:pewang-High-44911-[CSI Driver] [Dynamic PV] [Filesystem] could not write into read-only volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod1 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod2 := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pvc.namespace = oc.Namespace()
			pod1.namespace, pod2.namespace = pvc.namespace, pvc.namespace

			g.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create pod1 with the created pvc and wait for the pod ready")
			pod1.create(oc)
			defer pod1.deleteAsAdmin(oc)
			pod1.waitReady(oc)

			g.By("# Check the pod volume could be read and written and write testfile with content 'storage test' to the volume")
			pod1.checkMountedVolumeCouldRW(oc)

			// When the test cluster have multi node in the same az,
			// delete the pod1 could help us test the pod2 maybe schedule to a different node scenario
			// If pod2 schedule to a different node, the pvc bound pv could be attach successfully and the test data also exist
			g.By("# Delete pod1")
			pod1.delete(oc)

			g.By("# Use readOnly parameter create pod2 with the pvc: 'spec.containers[0].volumeMounts[0].readOnly: true' and wait for the pod ready ")
			pod2.createWithReadOnlyVolume(oc)
			defer pod2.deleteAsAdmin(oc)
			pod2.waitReady(oc)

			g.By("# Check the file /mnt/storage/testfile exist in the volume and read its content contains 'storage test' ")
			output, err := pod2.execCommand(oc, "cat "+pod2.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("# Write something to the readOnly mount volume failed")
			output, err = pod2.execCommand(oc, "touch "+pod2.mountPath+"/test"+getRandomString())
			o.Expect(err).Should(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("Read-only file system"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-44910 - [CSI-Driver] [Dynamic PV] [Filesystem default] support mountOptions
	g.It("Author:wduan-High-44910-[CSI Driver] [Dynamic PV] [Filesystem default] support mountOptions", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			mountOption          = []string{"debug", "discard"}
			extraParameters      = map[string]interface{}{
				"allowVolumeExpansion": true,
				"mountOptions":         mountOption,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner in " + cloudProvider + "!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			g.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("3. Create deployment with the created pvc")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			g.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("5. Check the deployment's pod mounted volume contains the mount option by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "debug")
			dep.checkPodMountedVolumeContain(oc, "discard")

			g.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("8. Check the volume mounted contains the mount option by exec mount cmd in the node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "debug")
			checkVolumeMountCmdContain(oc, volName, nodeName, "discard")

			g.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			g.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			g.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			g.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			g.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// OCP-44904 [CSI Driver] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume
	g.It("Author:pewang-High-44904-[CSI Driver] [Dynamic PV] [xfs] volumes should store data and allow exec of files on the volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			g.By("1. Create csi storageclass")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("2. Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("3. Create deployment with the created pvc and wait for the pod ready")
			dep.create(oc)
			defer dep.deleteAsAdmin(oc)

			g.By("4. Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("5. Check the deployment's pod mounted volume fstype is xfs by exec mount cmd in the pod")
			dep.checkPodMountedVolumeContain(oc, "xfs")

			g.By("6. Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("7. Check the deployment's pod mounted volume have the exec right")
			dep.checkPodMountedVolumeHaveExecRight(oc)

			g.By("8. Check the volume mounted on the pod located node")
			volName := pvc.getVolumeName(oc)
			nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

			g.By("9. Scale down the replicas number to 0")
			dep.scaleReplicas(oc, "0")

			g.By("10. Wait for the deployment scale down completed and check nodes has no mounted volume")
			dep.waitReady(oc)
			checkVolumeNotMountOnNode(oc, volName, nodeName)

			g.By("11. Scale up the deployment replicas number to 1")
			dep.scaleReplicas(oc, "1")

			g.By("12. Wait for the deployment scale up completed")
			dep.waitReady(oc)

			g.By("13. After scaled check the deployment's pod mounted volume contents and exec right")
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat /mnt/storage/testfile*")).To(o.ContainSubstring("storage test"))
			o.Expect(execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "/mnt/storage/hello")).To(o.ContainSubstring("Hello OpenShift Storage"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// OCP-47370 -[CSI Driver] [Dynamic PV] [Filesystem] provisioning volume with subpath
	g.It("Author:pewang-High-47370-[CSI Driver] [Dynamic PV] [Filesystem] provisioning volume with subpath", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			podWithSubpathA := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podWithSubpathB := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podWithSubpathC := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			podWithNoneSubpath := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			g.By("# Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create podWithSubpathA, podWithSubpathB, podWithNoneSubpath with the created pvc and wait for the pods ready")
			podWithSubpathA.createWithSubpathVolume(oc, "subpathA")
			defer podWithSubpathA.deleteAsAdmin(oc)
			podWithSubpathA.waitReady(oc)
			// Since the scenario all the test pods comsume the same pvc and scheduler maybe schedule the test pods to different cause flake of "Unable to attach or mount volumes"
			// Patch the test namespace with node-selector schedule the test pods to the same node
			nodeName := getNodeNameByPod(oc, podWithSubpathA.namespace, podWithSubpathA.name)
			patchPath := `{"metadata":{"annotations":{"openshift.io/node-selector":"kubernetes.io/hostname=` + nodeName + `"}}}`
			_, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("namespace", podWithSubpathA.namespace, "-p", patchPath).Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
			// Create podWithSubpathB, podWithNoneSubpath with the same pvc
			podWithSubpathB.createWithSubpathVolume(oc, "subpathB")
			defer podWithSubpathB.deleteAsAdmin(oc)
			podWithNoneSubpath.create(oc)
			defer podWithNoneSubpath.deleteAsAdmin(oc)
			podWithSubpathB.waitReady(oc)
			podWithNoneSubpath.waitReady(oc)

			g.By("# Check the podWithSubpathA's volume could be read, written, exec and podWithSubpathB couldn't see the written content")
			podWithSubpathA.checkMountedVolumeCouldRW(oc)
			podWithSubpathA.checkMountedVolumeHaveExecRight(oc)
			output, err := podWithSubpathB.execCommand(oc, "ls /mnt/storage")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			o.Expect(output).ShouldNot(o.ContainSubstring("testfile"))

			g.By("# Check the podWithNoneSubpath could see both 'subpathA' and 'subpathB' folders with 'container_file_t' label")
			output, err = podWithNoneSubpath.execCommand(oc, "ls -Z /mnt/storage")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("subpathA"))
			o.Expect(output).Should(o.ContainSubstring("subpathB"))
			o.Expect(output).Should(o.ContainSubstring("container_file_t"))

			g.By("# Use the same subpath 'subpathA' create podWithSubpathC and wait for the pod ready")
			podWithSubpathC.createWithSubpathVolume(oc, "subpathA")
			defer podWithSubpathC.deleteAsAdmin(oc)
			podWithSubpathC.waitReady(oc)

			g.By("# Check the subpathA's data still exist not be covered and podWithSubpathC could also see the file content")
			output, err = podWithSubpathC.execCommand(oc, "cat /mnt/storage/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("storage test"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-44905 - [CSI-Driver] [Dynamic PV] [block volume] volumes should store data
	g.It("Author:wduan-Critical-44905-[CSI-Driver] [Dynamic PV] [block volume] volumes should store data", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for raw block volume
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod.namespace, pod.name)

			g.By("Write file to raw block volume")
			pod.writeDataIntoRawBlockVolume(oc)

			g.By("Delete pod")
			pod.deleteAsAdmin(oc)

			g.By("Check the volume umount from the node")
			volName := pvc.getVolumeName(oc)
			checkVolumeDetachedFromNode(oc, volName, nodeName)

			g.By("Create new pod with the pvc and wait for the pod ready")
			pod_new := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
			pod_new.create(oc)
			defer pod_new.deleteAsAdmin(oc)
			pod_new.waitReady(oc)

			g.By("Check the data in the raw block volume")
			pod_new.checkDataInRawBlockVolume(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-46358 - [CSI Driver] [CSI Clone] Clone a pvc with filesystem VolumeMode
	g.It("Author:wduan-Critical-46358-[CSI Driver] [CSI Clone] Clone a pvc with filesystem VolumeMode", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("%s", pvc_ori.scname)
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvc_clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvc_ori.name))
			pod_clone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_clone.name))

			g.By("Create a clone pvc with the preset csi storageclass")
			pvc_clone.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("%s", pvc_ori.scname)
			pvc_clone.capacity = pvc_ori.capacity
			pvc_clone.createWithCloneDataSource(oc)
			defer pvc_clone.deleteAsAdmin(oc)

			g.By("Create pod with the cloned pvc and wait for the pod ready")
			pod_clone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer pod_clone.deleteAsAdmin(oc)
			pod_clone.waitReady(oc)

			g.By("Delete origial pvc will not impact the cloned one")
			pod_ori.deleteAsAdmin(oc)
			pvc_ori.deleteAsAdmin(oc)

			g.By("Check the file exist in cloned volume")
			output, err := pod_clone.execCommand(oc, "cat "+pod_clone.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47224 - [CSI Driver] [CSI Clone] [Filesystem] provisioning volume with pvc data source larger than original volume
	g.It("Author:wduan-High-47224-[CSI Driver] [CSI Clone] [Filesystem] provisioning volume with pvc data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity("1Gi"))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("%s", pvc_ori.scname)
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvc_clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvc_ori.name))
			pod_clone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_clone.name))

			g.By("Create a clone pvc with the preset csi storageclass")
			pvc_clone.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			cloneCapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc_ori.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			cloneCapacityInt64 = cloneCapacityInt64 + getRandomNum(1, 10)
			pvc_clone.capacity = strconv.FormatInt(cloneCapacityInt64, 10) + "Gi"
			pvc_clone.createWithCloneDataSource(oc)
			defer pvc_clone.deleteAsAdmin(oc)

			g.By("Create pod with the cloned pvc and wait for the pod ready")
			pod_clone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer pod_clone.deleteAsAdmin(oc)
			pod_clone.waitReady(oc)

			g.By("Check the cloned pvc size is as expected")
			pvc_clone_size, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvc_clone.name, "-n", pvc_clone.namespace, "-o=jsonpath={.status.capacity.storage}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The pvc.status.capacity.storage is %s", pvc_clone_size)
			o.Expect(pvc_clone_size).To(o.Equal(pvc_clone.capacity))

			g.By("Check the file exist in cloned volume")
			output, err := pod_clone.execCommand(oc, "cat "+pod_clone.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("Check could write more data")
			blockCounts := strconv.FormatInt(cloneCapacityInt64*4*4/5, 10)
			output1, err := pod_clone.execCommand(oc, "/bin/dd  if=/dev/zero of="+pod_clone.mountPath+"/testfile1 bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-46813 - [CSI Driver] [CSI Clone] Clone a pvc with Raw Block VolumeMode
	g.It("Author:wduan-Critical-46813-[CSI Driver][CSI Clone] Clone a pvc with Raw Block VolumeMode", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)

			g.By("Write data to volume")
			pod_ori.writeDataIntoRawBlockVolume(oc)
			pod_ori.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvc_clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvc_ori.name))
			pod_clone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_clone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a clone pvc with the preset csi storageclass")
			pvc_clone.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc_clone.capacity = pvc_ori.capacity
			pvc_clone.createWithCloneDataSource(oc)
			defer pvc_clone.deleteAsAdmin(oc)

			g.By("Create pod with the cloned pvc and wait for the pod ready")
			pod_clone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer pod_clone.deleteAsAdmin(oc)
			pod_clone.waitReady(oc)

			g.By("Check the data exist in cloned volume")
			pod_clone.checkDataInRawBlockVolume(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47225 - [CSI Driver] [CSI Clone] [Raw Block] provisioning volume with pvc data source larger than original volume
	g.It("Author:wduan-High-47225-[CSI Driver] [CSI Clone] [Raw Block] provisioning volume with pvc data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "cinder.csi.openstack.org"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimCapacity("1Gi"))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)

			g.By("Write data to volume")
			pod_ori.writeDataIntoRawBlockVolume(oc)
			pod_ori.execCommand(oc, "sync")

			// Set the resource definition for the clone
			pvc_clone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvc_ori.name))
			pod_clone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_clone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a clone pvc with the preset csi storageclass")
			pvc_clone.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			cloneCapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc_ori.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			cloneCapacityInt64 = cloneCapacityInt64 + getRandomNum(1, 10)
			pvc_clone.capacity = strconv.FormatInt(cloneCapacityInt64, 10) + "Gi"
			pvc_clone.createWithCloneDataSource(oc)
			defer pvc_clone.deleteAsAdmin(oc)

			g.By("Create pod with the cloned pvc and wait for the pod ready")
			pod_clone.createWithNodeSelector(oc, "kubernetes\\.io/hostname", nodeName)
			defer pod_clone.deleteAsAdmin(oc)
			pod_clone.waitReady(oc)

			g.By("Check the cloned pvc size is as expected")
			pvc_clone_size, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pvc", pvc_clone.name, "-n", pvc_clone.namespace, "-o=jsonpath={.status.capacity.storage}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("The pvc.status.capacity.storage is %s", pvc_clone_size)
			o.Expect(pvc_clone_size).To(o.Equal(pvc_clone.capacity))

			g.By("Check the data exist in cloned volume")
			pod_clone.checkDataInRawBlockVolume(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// OCP-44909 [CSI Driver] Volume should mount again after `oc adm drain`
	g.It("Author:pewang-High-44909-[CSI Driver] Volume should mount again after `oc adm drain` [Disruptive]", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir                   = exutil.FixturePath("testdata", "storage")
			pvcTemplate                          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate                   = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners                  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
			schedulableWorkersWithSameAz, azName = getSchedulableWorkersWithSameAz(oc)
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip: Non-supported provisioner!!!")
		}
		if len(schedulableWorkersWithSameAz) == 0 {
			g.Skip("Skip: The test cluster has less than two schedulable workers in each avaiable zone!!!")
		}
		// Set up a specified project share for all the phases
		g.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

			g.By("# Create a pvc with preset csi storageclass")
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create a deployment with the created pvc, node selector and wait for the pod ready")
			if azName == "noneAzCluster" {
				dep.create(oc)
			} else {
				dep.createWithNodeSelector(oc, `topology\.kubernetes\.io\/zone`, azName)
			}
			defer dep.deleteAsAdmin(oc)

			g.By("# Wait for the deployment ready")
			dep.waitReady(oc)

			g.By("# Check the deployment's pod mounted volume can be read and write")
			dep.checkPodMountedVolumeCouldRW(oc)

			g.By("# Run drain cmd to drain the node which the deployment's pod located")
			originNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			drainSpecificNode(oc, originNodeName)
			defer uncordonSpecificNode(oc, originNodeName)

			g.By("# Wait for the deployment become ready again")
			dep.waitReady(oc)

			g.By("# Check the deployment's pod schedule to another ready node")
			newNodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
			o.Expect(originNodeName).NotTo(o.Equal(newNodeName))

			g.By("# Check testdata still in the volume")
			output, err := execCommandInSpecificPod(oc, dep.namespace, dep.getPodList(oc)[0], "cat "+dep.mpath+"/testfile*")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes/#delete
	g.It("Author:pewang-High-44906-[CSI Driver] [Dynamic PV] [Delete reclaimPolicy] volumes should be deleted after the pvc deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Use the framework created project as default, if use your own, exec the follow code setupProject
		g.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassReclaimPolicy("Delete"), setStorageClassVolumeBindingMode("Immediate"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))

			g.By("# Make sure we have a csi storageclass with 'reclaimPolicy: Delete' and 'volumeBindingMode: Immediate'")
			presetStorageClassName := getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("The preset storage class name is: %s", pvc.scname)
			if getReclaimPolicyByStorageClassName(oc, presetStorageClassName) != "delete" || getVolumeBindingModeByStorageClassName(oc, presetStorageClassName) != "immediate" {
				storageClass.create(oc)
				defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
				pvc.scname = storageClass.name
			} else {
				e2e.Logf("Using the preset storageclass: %s", presetStorageClassName)
				pvc.scname = presetStorageClassName
			}

			g.By("# Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Wait for the pvc become to bound")
			pvc.waitStatusAsExpected(oc, "Bound")

			g.By("# Get the volumename, volumeId")
			volumeName := pvc.getVolumeName(oc)
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", volumeName).Execute()
			volumeId := pvc.getVolumeId(oc)
			defer deleteBackendVolumeByVolumeId(oc, volumeId)

			g.By("# Delete the pvc and check the pv is deleted accordingly")
			pvc.delete(oc)
			waitForPersistentVolumeStatusAsExpected(oc, volumeName, "deleted")

			g.By("# Check the volume on backend is deleted")
			getCredentialFromCluster(oc)
			waitVolumeDeletedOnBackend(oc, volumeId)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: pewang@redhat.com
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes/#retain
	g.It("Author:pewang-High-44907-[CSI Driver] [Dynamic PV] [Retain reclaimPolicy] [Static PV] volumes could be re-used after the pvc/pv deletion", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			supportProvisioners  = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Use the framework created project as default, if use your own, exec the follow code setupProject
		g.By("# Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner(provisioner), setStorageClassReclaimPolicy("Retain"))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			newpvc := newPersistentVolumeClaim(setPersistentVolumeClaimStorageClassName(storageClass.name), setPersistentVolumeClaimTemplate(pvcTemplate))
			newpod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(newpvc.name))

			g.By("# Create csi storageclass with 'reclaimPolicy: retain'")
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("# Create a pvc with the csi storageclass")
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("# Create pod with the created pvc and wait for the pod ready")
			pod.create(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			g.By("# Get the volumename, volumeId and pod located node name")
			volumeName := pvc.getVolumeName(oc)
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", volumeName).Execute()
			volumeId := pvc.getVolumeId(oc)
			defer deleteBackendVolumeByVolumeId(oc, volumeId)
			originNodeName := getNodeNameByPod(oc, pod.namespace, pod.name)

			g.By("# Check the pod volume can be read and write")
			pod.checkMountedVolumeCouldRW(oc)

			g.By("# Check the pod volume have the exec right")
			pod.checkMountedVolumeHaveExecRight(oc)

			g.By("# Delete the pod and pvc")
			pod.delete(oc)
			pvc.delete(oc)

			g.By("# Check the PV status become to 'Released' ")
			waitForPersistentVolumeStatusAsExpected(oc, volumeName, "Released")

			g.By("# Delete the PV and check the volume already not mounted on node")
			originpv, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pv", volumeName, "-o", "json").Output()
			debugLogf(originpv)
			o.Expect(err).ShouldNot(o.HaveOccurred())
			_, err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", volumeName).Output()
			o.Expect(err).ShouldNot(o.HaveOccurred())
			waitForPersistentVolumeStatusAsExpected(oc, volumeName, "deleted")
			checkVolumeNotMountOnNode(oc, volumeName, originNodeName)

			g.By("# Check the volume still exists in backend by volumeId")
			getCredentialFromCluster(oc)
			waitVolumeAvaiableOnBackend(oc, volumeId)

			g.By("# Use the retained volume create new pv,pvc,pod and wait for the pod running")
			newPvName := "newpv-" + getRandomString()
			defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", newPvName).Execute()
			createNewPersistVolumeWithRetainVolume(oc, originpv, storageClass.name, newPvName)
			newpvc.capacity = pvc.capacity
			newpvc.createWithSpecifiedPV(oc, newPvName)
			defer newpvc.deleteAsAdmin(oc)
			newpod.create(oc)
			defer newpod.deleteAsAdmin(oc)
			newpod.waitReady(oc)

			g.By("# Check the retained pv's data still exist and have exec right")
			output, err := newpod.execCommand(oc, "cat "+newpod.mountPath+"/testfile")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("storage test"))
			output, err = newpod.execCommand(oc, newpod.mountPath+"/hello")
			o.Expect(err).ShouldNot(o.HaveOccurred())
			o.Expect(output).Should(o.ContainSubstring("Hello OpenShift Storage"))

			g.By("# Delete the pv and check the retained pv delete in backend")
			newpod.delete(oc)
			newpvc.delete(oc)
			err = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pv", newPvName).Execute()
			o.Expect(err).ShouldNot(o.HaveOccurred())
			waitForPersistentVolumeStatusAsExpected(oc, newPvName, "deleted")
			deleteBackendVolumeByVolumeId(oc, volumeId)
			waitVolumeDeletedOnBackend(oc, volumeId)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI Driver] [Dynamic PV] [Filesystem] volumes resize on-line
	g.It("Author:ropatil-Critical-45984-[CSI Driver] [Dynamic PV] [Filesystem default] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Online resize volume
			ResizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI Driver] [Dynamic PV] [Raw Block] volumes resize on-line
	g.It("Author:ropatil-Critical-45985-[CSI Driver] [Dynamic PV] [Raw block] volumes resize on-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Online resize volume
			ResizeOnlineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI Driver] [Dynamic PV] [Filesystem] volumes resize off-line
	g.It("Author:ropatil-Critical-41452-[CSI Driver] [Dynamic PV] [Filesystem default] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Offline resize volume
			ResizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: ropatil@redhat.com
	// [CSI Driver] [Dynamic PV] [Raw block] volumes resize off-line
	g.It("Author:ropatil-Critical-44902-[CSI Driver] [Dynamic PV] [Raw block] volumes resize off-line", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"disk.csi.azure.com", "csi.vsphere.vmware.com"}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate  = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set up a specified project share for all the phases
		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimStorageClassName(getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)))
			dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"), setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"))
			pvc.namespace = oc.Namespace()
			dep.namespace = pvc.namespace

			// Performing the Test Steps for Offline resize volume
			ResizeOfflineCommonTestSteps(oc, pvc, dep, cloudProvider, provisioner)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
	// author: chaoyang@redhat.com
	//[CSI Driver] [Dynamic PV] [Security] CSI volume security testing when privileged is false
	g.It("Author:chaoyang-Critical-44908-[CSI Driver] [Dynamic PV] CSI volume security testing when privileged is false ", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com"}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir  = exutil.FixturePath("testdata", "storage")
			pvcTemplate         = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate         = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			supportProvisioners = sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		)

		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		g.By("0. Create new project for the scenario")
		oc.SetupProject() //create new project

		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

			pvc.namespace = oc.Namespace()
			pod.namespace = pvc.namespace
			g.By("1. Create a pvc with the preset csi storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			e2e.Logf("%s", pvc.scname)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("2. Create pod with the created pvc and wait for the pod ready")
			pod.createWithSecurity(oc)
			defer pod.deleteAsAdmin(oc)
			pod.waitReady(oc)

			g.By("3. Check pod security--uid")
			output_uid, err := pod.execCommandAsAdmin(oc, "id -u")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", output_uid)
			o.Expect(output_uid).To(o.ContainSubstring("1000160000"))

			g.By("4. Check pod security--fsGroup")
			output_gid, err := pod.execCommandAsAdmin(oc, "id -G")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", output_gid)
			o.Expect(output_gid).To(o.ContainSubstring("24680"))

			g.By("5. Check pod security--selinux")
			output_mountPath, err := pod.execCommandAsAdmin(oc, "ls -lZd "+pod.mountPath)
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", output_mountPath)
			o.Expect(output_mountPath).To(o.ContainSubstring("24680"))
			o.Expect(output_mountPath).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			_, err = pod.execCommandAsAdmin(oc, "touch "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			output_testfile, err := pod.execCommandAsAdmin(oc, "ls -lZ "+pod.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf("%s", output_testfile)
			o.Expect(output_testfile).To(o.ContainSubstring("24680"))
			o.Expect(output_testfile).To(o.ContainSubstring("system_u:object_r:container_file_t:s0:c2,c13"))

			_, err = pod.execCommandAsAdmin(oc, "cp /hello "+pod.mountPath)
			o.Expect(err).NotTo(o.HaveOccurred())
			output_execfile, err := pod.execCommandAsAdmin(oc, "cat "+pod.mountPath+"/hello")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output_execfile).To(o.ContainSubstring("Hello OpenShift Storage"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})

	// author: wduan@redhat.com
	// OCP-48911 - [CSI Driver] [fsgroup] should be updated with new defined value when volume attach to another pod
	g.It("Author:wduan-Critical-48911-[CSI Driver] [fsgroup] should be updated with new defined value when volume attach to another pod", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "cinder.csi.openstack.org", "pd.csi.storage.gke.io", "csi.vsphere.vmware.com", "vpc.block.csi.ibm.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir   = exutil.FixturePath("testdata", "storage")
			storageClassTemplate = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate          = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate          = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
		)
		// Set up a specified project share for all the phases
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the scenario
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name))
			pod_a := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			securityContext := map[string]interface{}{
				"fsGroup": 10000,
			}
			extraParameters := map[string]interface{}{
				"jsonPath":        `items.0.spec.`,
				"securityContext": securityContext,
			}

			g.By("Create a pvc with the preset storageclass")
			pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc.create(oc)
			defer pvc.deleteAsAdmin(oc)

			g.By("Create pod_a with the created pvc and wait pod ready")
			pod_a.createWithExtraParameters(oc, extraParameters)
			defer pod_a.deleteAsAdmin(oc)
			pod_a.waitReady(oc)

			g.By("Check the fsgroup of mounted volume and new created file should be 10000")
			pod_a.checkFsgroup(oc, "ls -lZd "+pod_a.mountPath, "10000")
			_, err := pod_a.execCommandAsAdmin(oc, "touch "+pod_a.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			pod_a.checkFsgroup(oc, "ls -lZ "+pod_a.mountPath+"/testfile", "10000")

			g.By("Delete the pod_a")
			pod_a.delete(oc)

			securityContext = map[string]interface{}{
				"fsGroup": 20000,
			}
			extraParameters = map[string]interface{}{
				"jsonPath":        `items.0.spec.`,
				"securityContext": securityContext,
			}

			g.By("Create pod_b with the same pvc and wait pod ready")
			pod_b := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))
			pod_b.createWithExtraParameters(oc, extraParameters)
			defer pod_b.deleteAsAdmin(oc)
			pod_b.waitReady(oc)

			g.By("Check the fsgroup of mounted volume, existing file and new created file should be 20000")
			pod_b.checkFsgroup(oc, "ls -lZd "+pod_b.mountPath, "20000")
			pod_b.checkFsgroup(oc, "ls -lZ "+pod_b.mountPath+"/testfile", "20000")
			_, err = pod_b.execCommandAsAdmin(oc, "touch "+pod_b.mountPath+"/testfile-new")
			o.Expect(err).NotTo(o.HaveOccurred())
			pod_b.checkFsgroup(oc, "ls -lZ "+pod_b.mountPath+"/testfile-new", "20000")

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47879 - [CSI Driver] [Snapshot] [Filesystem default] provisioning should provision storage with snapshot data source and restore it succesffully
	g.It("Author:wduan-Critical-47879-[CSI Driver] [Snapshot] [Filesystem default] provisioning should provision storage with snapshot data source and restore it succesffully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name))

			g.By("Create a restored pvc with the preset csi storageclass")
			pvc_restore.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
			pvc_restore.capacity = pvc_ori.capacity
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the file exist in restored volume")
			output, err := pod_restore.execCommand(oc, "cat "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			pod_restore.checkMountedVolumeCouldRW(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47930 - [CSI Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source and restore it succesffully
	g.It("Author:wduan-Critical-47930-[CSI Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source and restore it succesffully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Check fstype")
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)
			volName := pvc_ori.getVolumeName(oc)
			checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name))

			g.By("Create a restored pvc with the preset csi storageclass")
			pvc_restore.scname = storageClass.name
			pvc_restore.capacity = pvc_ori.capacity
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the file exist in restored volume")
			output, err := pod_restore.execCommand(oc, "cat "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			pod_restore.checkMountedVolumeCouldRW(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: wduan@redhat.com
	// OCP-47931 - [CSI Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source and restore it succesffully
	g.It("Author:wduan-Critical-47931-[CSI Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source and restore it succesffully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}

		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			// Set the resource definition for the original
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("Create a pvc with the preset csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Check fstype")
			nodeName := getNodeNameByPod(oc, pod_ori.namespace, pod_ori.name)
			volName := pvc_ori.getVolumeName(oc)
			checkVolumeMountCmdContain(oc, volName, nodeName, "xfs")

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name))

			g.By("Create a restored pvc with the preset csi storageclass")
			pvc_restore.scname = storageClass.name
			pvc_restore.capacity = pvc_ori.capacity
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the file exist in restored volume")
			output, err := pod_restore.execCommand(oc, "cat "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))
			pod_restore.checkMountedVolumeCouldRW(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})
	// author: chaoyang@redhat.com
	// OCP-48723 - [CSI Driver] [Snapshot] [Block] provisioning should provision storage with snapshot data source and restore it succesffully
	g.It("Author:chaoyang-Critical-48723-[CSI Driver] [Snapshot] [block] provisioning should provision storage with snapshot data source and restore it succesffully", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			g.By("Create a pvc with the csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Write file to raw block volume")
			pod_ori.writeDataIntoRawBlockVolume(oc)
			pod_ori.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a restored pvc with the csi storageclass")
			pvc_restore.scname = storageClass.name
			pvc_restore.capacity = pvc_ori.capacity
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the data in the raw block volume")
			pod_restore.checkDataInRawBlockVolume(oc)

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")

		}
	})
	//author: chaoyang@redhat.com
	//OCP-48913 - [CSI Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source larger than original volume
	g.It("Author:chaoyang-Medium-48913-[CSI Driver] [Snapshot] [Filesystem ext4] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))

			g.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'ext4'")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("Create a pvc with the csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name))

			g.By("Create a restored pvc with the created csi storageclass")
			pvc_restore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc_ori.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvc_restore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the file exist in restored volume")
			output, err := pod_restore.execCommand(oc, "cat "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := pod_restore.execCommand(oc, "/bin/dd  if=/dev/zero of="+pod_restore.mountPath+"/testfile1 bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	//author: chaoyang@redhat.com
	//OCP-48933 - [CSI Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source larger than original volume
	g.It("Author:chaoyang-Medium-48933-[CSI Driver] [Snapshot] [Filesystem xfs] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")

			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "xfs",
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name))

			g.By("Create a csi storageclass with parameter 'csi.storage.k8s.io/fstype': 'xfs'")
			storageClass.provisioner = provisioner
			storageClass.createWithExtraParameters(oc, extraParameters)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.

			g.By("Create a pvc with the csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)

			g.By("Write file to volume")
			pod_ori.checkMountedVolumeCouldRW(oc)
			pod_ori.execCommand(oc, "sync")

			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name))

			g.By("Create a restored pvc with the created csi storageclass")
			pvc_restore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc_ori.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvc_restore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)
			g.By("Check the file exist in restored volume")
			output, err := pod_restore.execCommand(oc, "cat "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("storage test"))

			g.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			//blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := pod_restore.execCommand(oc, "fallocate -l "+fmt.Sprintf("%d", restoreVolInt64)+"G "+pod_restore.mountPath+"/testfile")
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}
	})

	// author: chaoyang@redhat.com
	//OCP-48934 - [CSI Driver] [Snapshot] [Raw Block] provisioning should provision storage with snapshot data source larger than original volume
	g.It("Author:chaoyang-Medium-48934-[CSI Driver] [Snapshot] [Raw Block] provisioning should provision storage with snapshot data source larger than original volume", func() {
		// Define the test scenario support provisioners
		scenarioSupportProvisioners := []string{"ebs.csi.aws.com", "disk.csi.azure.com", "pd.csi.storage.gke.io", "diskplugin.csi.alibabacloud.com"}
		supportProvisioners := sliceIntersect(scenarioSupportProvisioners, getSupportProvisionersByCloudProvider(cloudProvider))
		if len(supportProvisioners) == 0 {
			g.Skip("Skip for scenario non-supported provisioner!!!")
		}
		// Set the resource template for the scenario
		var (
			storageTeamBaseDir     = exutil.FixturePath("testdata", "storage")
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate            = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			volumesnapshotTemplate = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
		)
		g.By("Create new project for the scenario")
		oc.SetupProject() //create new project
		for _, provisioner := range supportProvisioners {
			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase start" + "******")
			storageClass := newStorageClass(setStorageClassTemplate(storageClassTemplate))
			pvc_ori := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
			pod_ori := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_ori.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			storageClass.provisioner = provisioner
			storageClass.create(oc)
			defer storageClass.deleteAsAdmin(oc) // ensure the storageclass is deleted whether the case exist normally or not.
			g.By("Create a pvc with the csi storageclass")
			pvc_ori.scname = storageClass.name
			pvc_ori.create(oc)
			defer pvc_ori.deleteAsAdmin(oc)

			g.By("Create pod with the created pvc and wait for the pod ready")
			pod_ori.create(oc)
			defer pod_ori.deleteAsAdmin(oc)
			pod_ori.waitReady(oc)
			g.By("Write file to raw block volume")
			pod_ori.writeDataIntoRawBlockVolume(oc)
			pod_ori.execCommand(oc, "sync")

			// Create volumesnapshot with pre-defined volumesnapshotclass
			g.By("Create volumesnapshot and wait for ready_to_use")
			preset_vscname := getPresetVolumesnapshotClassNameByProvisioner(cloudProvider, provisioner)
			volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvc_ori.name), setVolumeSnapshotVscname(preset_vscname))
			volumesnapshot.create(oc)
			defer volumesnapshot.delete(oc)
			volumesnapshot.waitReadyToUse(oc)

			// Set the resource definition for the restore
			pvc_restore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
			pod_restore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc_restore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

			g.By("Create a restored pvc with the csi storageclass")
			pvc_restore.scname = storageClass.name
			oricapacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc_ori.capacity, "Gi"), 10, 64)
			o.Expect(err).To(o.Not(o.HaveOccurred()))
			restorecapacityInt64 := oricapacityInt64 + getRandomNum(3, 8)
			pvc_restore.capacity = strconv.FormatInt(restorecapacityInt64, 10) + "Gi"
			pvc_restore.capacity = pvc_ori.capacity
			pvc_restore.createWithSnapshotDataSource(oc)
			defer pvc_restore.deleteAsAdmin(oc)

			g.By("Create pod with the restored pvc and wait for the pod ready")
			pod_restore.create(oc)
			defer pod_restore.deleteAsAdmin(oc)
			pod_restore.waitReady(oc)

			g.By("Check the data in the raw block volume")
			pod_restore.checkDataInRawBlockVolume(oc)

			g.By("Check could write more data")
			restoreVolInt64 := oricapacityInt64 + 2
			blockCounts := strconv.FormatInt(restoreVolInt64*4*4/5, 10)
			output1, err := pod_restore.execCommand(oc, "/bin/dd  if=/dev/null of="+pod_restore.mountPath+" bs=256M count="+blockCounts)
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output1).NotTo(o.ContainSubstring("No space left on device"))

			g.By("******" + cloudProvider + " csi driver: \"" + provisioner + "\" test phase finished" + "******")
		}

	})
})

// Performing test steps for Online Volume Resizing
func ResizeOnlineCommonTestSteps(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, cloudProvider string, provisioner string) {
	// Set up a specified project share for all the phases
	g.By("1. Create a pvc with the preset csi storageclass")
	pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
	e2e.Logf("%s", pvc.scname)
	pvc.create(oc)
	defer pvc.deleteAsAdmin(oc)

	g.By("2. Create deployment with the created pvc and wait for the pod ready")
	dep.create(oc)
	defer dep.deleteAsAdmin(oc)

	g.By("3. Wait for the deployment ready")
	dep.waitReady(oc)

	g.By("4. Check the pvc status to Bound")
	o.Expect(getPersistentVolumeClaimStatus(oc, pvc.namespace, pvc.name)).To(o.Equal("Bound"))

	g.By("5. Write data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeCouldRW(oc)
	} else {
		dep.writeDataBlockType(oc)
	}

	g.By("6. Apply the patch to Resize the pvc volume")
	capacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	capacityInt64 = capacityInt64 + getRandomNum(1, 10)
	expandedCapactiy := strconv.FormatInt(capacityInt64, 10) + "Gi"
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapactiy)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapactiy

	g.By("7. Waiting for the pvc capacity update sucessfully")
	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)
	pvc.waitResizeSuccess(oc, pvc.capacity)

	g.By("8. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.getPodMountedVolumeData(oc)
		// After volume expand write 80% data of the new capacity should succeed
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+fmt.Sprintf("%.2f", float64(capacityInt64)*0.8)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No space left on device"))
		// Continue write 30% data of the new capacity should fail of "No space left on device"
		msg, err = execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+fmt.Sprintf("%.2f", float64(capacityInt64)*0.3)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("No space left on device"))
	} else {
		// Since fallocate doesn't support raw block write and dd cmd write big file is too slow, just check origin data intact
		dep.checkDataBlockType(oc)
		dep.writeDataBlockType(oc)
	}
}

// Test steps for Offline Volume Resizing
// E.g. Expand a Persistent Volume in Offline Mode (vmware doc)
// https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-90082E1C-DC01-4610-ABA2-6A4E97C18CBC.html?hWord=N4IghgNiBcIKIA8AOYB2ATABGTA1A9hAK4C2ApiAL5A
func ResizeOfflineCommonTestSteps(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, cloudProvider string, provisioner string) {
	// Set up a specified project share for all the phases
	g.By("1. Create a pvc with the preset csi storageclass")
	pvc.scname = getPresetStorageClassNameByProvisioner(cloudProvider, provisioner)
	e2e.Logf("%s", pvc.scname)
	pvc.create(oc)
	defer pvc.deleteAsAdmin(oc)

	g.By("2. Create deployment with the created pvc and wait for the pod ready")
	dep.create(oc)
	defer dep.deleteAsAdmin(oc)

	g.By("3. Wait for the deployment ready")
	dep.waitReady(oc)

	g.By("4. Check the pvc status to Bound")
	o.Expect(getPersistentVolumeClaimStatus(oc, pvc.namespace, pvc.name)).To(o.Equal("Bound"))

	g.By("5. Write data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeCouldRW(oc)
	} else {
		dep.writeDataBlockType(oc)
	}

	g.By("6. Get the volume mounted on the pod located node and Scale down the replicas number to 0")
	volName := pvc.getVolumeName(oc)
	nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
	dep.scaleReplicas(oc, "0")

	g.By("7. Wait for the deployment scale down completed and check nodes has no mounted volume")
	dep.waitReady(oc)
	// Offline resize need the volume is detached from the node and when resize completely then comsume the volume
	checkVolumeDetachedFromNode(oc, volName, nodeName)

	g.By("8. Apply the patch to Resize the pvc volume")
	capacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	capacityInt64 = capacityInt64 + getRandomNum(1, 10)
	expandedCapactiy := strconv.FormatInt(capacityInt64, 10) + "Gi"
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapactiy)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapactiy

	g.By("9. Check the pvc resizing status type and wait for the backend volume resized")
	if dep.typepath == "mountPath" {
		getPersistentVolumeClaimStatusMatch(oc, dep.namespace, pvc.name, "FileSystemResizePending")
	} else {
		getPersistentVolumeClaimStatusType(oc, dep.namespace, dep.pvcname)
	}

	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)

	g.By("10. Scale up the replicas number to 1")
	dep.scaleReplicas(oc, "1")

	g.By("11. Get the pod status by label Running")
	dep.waitReady(oc)

	g.By("12. Waiting for the pvc size update sucessfully")
	pvc.waitResizeSuccess(oc, pvc.capacity)

	g.By("13. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.getPodMountedVolumeData(oc)
		// After volume expand write 80% data of the new capacity should succeed
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+fmt.Sprintf("%.2f", float64(capacityInt64)*0.8)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No space left on device"))
		// Continue write 30% data of the new capacity should fail of "No space left on device"
		msg, err = execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+fmt.Sprintf("%.2f", float64(capacityInt64)*0.3)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("No space left on device"))
	} else {
		// Since fallocate doesn't support raw block write and dd cmd write big file is too slow, just check origin data intact
		dep.checkDataBlockType(oc)
		dep.writeDataBlockType(oc)
	}
}
