// NOTE: This test suite currently only support SNO env & rely on some pre-defined steps in CI pipeline which includes,
//        1. Installing LVMS operator
//        2. Adding blank disk/device to worker node to be consumed by LVMCluster
//        3. Create resources like OperatorGroup, Subscription, etc. to configure LVMS operator
//        4. Create LVMCLuster resource with single volumeGroup named as 'vg1', mutliple VGs could be added in future
//      Also, these tests are utilizing preset lvms storageClass="lvms-vg1", volumeSnapshotClassName="lvms-vg1"

package storage

import (
	"math"
	"path/filepath"
	"regexp"
	"strconv"
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
		oc                 = exutil.NewCLI("storage-lvms", exutil.KubeConfigPath())
		storageTeamBaseDir string
		storageLvmsBaseDir string
	)

	g.BeforeEach(func() {
		checkLvmsOperatorInstalled(oc)
		storageTeamBaseDir = exutil.FixturePath("testdata", "storage")
		storageLvmsBaseDir = exutil.FixturePath("testdata", "storage", "lvms")
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61425-[LVMS] [Filesystem] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit
	g.It("Author:rdeore-Critical-61425-[LVMS] [Filesystem] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			thinPoolName       = "thin-pool-1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("2Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentNamespace(oc.Namespace()))

		exutil.By("#. Get thin pool size and over provision limit")
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)

		exutil.By("#. Check PVC can re-size beyond thinpool size, but within overprovisioning limit")
		targetCapactiyInt64 := getRandomNum(int64(thinPoolSize+1), int64(thinPoolSize+10))
		resizeLvmsVolume(oc, pvc, dep, targetCapactiyInt64)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61433-[LVMS] [Block] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit
	g.It("Author:rdeore-Critical-61433-[LVMS] [Block] [WaitForFirstConsumer] PVC resize on LVM cluster beyond thinpool size, but within over-provisioning limit", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			thinPoolName       = "thin-pool-1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("2Gi"), setPersistentVolumeClaimNamespace(oc.Namespace()), setPersistentVolumeClaimVolumemode("Block"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"),
			setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"), setDeploymentNamespace(oc.Namespace()))
		dep.namespace = pvc.namespace

		exutil.By("#. Get thin pool size and over provision limit")
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)

		exutil.By("#. Check PVC can re-size beyond thinpool size, but within overprovisioning rate")
		targetCapactiyInt64 := getRandomNum(int64(thinPoolSize+1), int64(thinPoolSize+10))
		resizeLvmsVolume(oc, pvc, dep, targetCapactiyInt64)
	})

	// author: rdeore@redhat.com
	// OCP-61585-[LVMS] [Filesystem] [Clone] a pvc with the same capacity should be successful
	g.It("Author:rdeore-Critical-61585-[LVMS] [Filesystem] [Clone] a pvc with the same capacity should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Delete origial pvc will not impact the cloned one")
		deleteSpecifiedResource(oc, "pod", podOri.name, podOri.namespace)
		deleteSpecifiedResource(oc, "pvc", pvcOri.name, pvcOri.namespace)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkMountedVolumeDataExist(oc, true)
	})

	// author: rdeore@redhat.com
	// OCP-61586-[LVMS] [Block] Clone a pvc with Block VolumeMode
	g.It("Author:rdeore-Critical-61586-[LVMS] [Block] Clone a pvc with Block VolumeMode", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Delete origial pvc will not impact the cloned one")
		deleteSpecifiedResource(oc, "pod", podOri.name, podOri.namespace)
		deleteSpecifiedResource(oc, "pvc", pvcOri.name, pvcOri.namespace)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkDataInRawBlockVolume(oc)
	})

	// author: rdeore@redhat.com
	// OCP-61863-[LVMS] [Filesystem] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written
	g.It("Author:rdeore-Critical-61863-[LVMS] [Filesystem] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
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
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkMountedVolumeDataExist(oc, true)

		// Check original pod and restored pod are deployed on same worker node, when cluster is multi-node
		if !exutil.IsSNOCluster(oc) {
			exutil.By("Check original pod and restored pod are deployed on same worker node")
			o.Expect(getNodeNameByPod(oc, podOri.namespace, podOri.name) == getNodeNameByPod(oc, podRestore.namespace, podRestore.name)).To(o.BeTrue())
		}
	})

	// author: rdeore@redhat.com
	// OCP-61894-[LVMS] [Block] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written
	g.It("Author:rdeore-Critical-61894-[LVMS] [Block] [Snapshot] should restore volume with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkDataInRawBlockVolume(oc)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61814-[LVMS] [Filesystem] [Clone] a pvc larger than disk size should be successful
	g.It("Author:rdeore-Critical-61814-[LVMS] [Filesystem] [Clone] a pvc larger than disk size should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			thinPoolName     = "thin-pool-1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass and capacity bigger than disk size")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Check clone volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcClone.name, pvcClone.namespace, thinPoolSize)

		exutil.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkMountedVolumeDataExist(oc, true)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61828-[LVMS] [Block] [Clone] a pvc larger than disk size should be successful
	g.It("Author:rdeore-Critical-61828-[LVMS] [Block] [Clone] a pvc larger than disk size should be successful", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			thinPoolName     = "thin-pool-1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Set the resource definition for the clone
		pvcClone := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(pvcOri.name))
		podClone := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcClone.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a clone pvc with the lvms storageclass")
		pvcClone.scname = storageClassName
		pvcClone.capacity = pvcOri.capacity
		pvcClone.createWithCloneDataSource(oc)
		defer pvcClone.deleteAsAdmin(oc)

		exutil.By("Create pod with the cloned pvc and wait for the pod ready")
		podClone.create(oc)
		defer podClone.deleteAsAdmin(oc)
		podClone.waitReady(oc)

		exutil.By("Check clone volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcClone.name, pvcClone.namespace, thinPoolSize)

		exutil.By("Delete origial pvc will not impact the cloned one")
		podOri.deleteAsAdmin(oc)
		pvcOri.deleteAsAdmin(oc)

		exutil.By("Check the file exist in cloned volume")
		podClone.checkDataInRawBlockVolume(oc)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61997-[LVMS] [Filesystem] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written
	g.It("Author:rdeore-Critical-61997-[LVMS] [Filesystem] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			thinPoolName            = "thin-pool-1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass and capacity bigger than disk size")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.checkMountedVolumeCouldRW(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check restored volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcRestore.name, pvcRestore.namespace, thinPoolSize)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkMountedVolumeDataExist(oc, true)
	})

	// NOTE: In this test case, we are testing volume provisioning beyond total disk size, it's only specific to LVMS operator
	//       as it supports over-provisioning, unlike other CSI drivers
	// author: rdeore@redhat.com
	// OCP-61998-[LVMS] [Block] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written
	g.It("Author:rdeore-Critical-61998-[LVMS] [Block] [Snapshot] should restore volume larger than disk size with snapshot dataSource successfully and the volume could be read and written", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate             = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate             = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumesnapshotTemplate  = filepath.Join(storageTeamBaseDir, "volumesnapshot-template.yaml")
			volumeGroup             = "vg1"
			thinPoolName            = "thin-pool-1"
			storageClassName        = "lvms-" + volumeGroup
			volumeSnapshotClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject() //create new project

		// Set the resource definition for the original
		pvcOri := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"))
		podOri := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcOri.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))
		thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
		pvcCapacity := strconv.FormatInt(int64(thinPoolSize)+getRandomNum(2, 10), 10) + "Gi"

		exutil.By("Create a pvc with the lvms csi storageclass")
		pvcOri.scname = storageClassName
		pvcOri.capacity = pvcCapacity
		pvcOri.create(oc)
		defer pvcOri.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and wait for the pod ready")
		podOri.create(oc)
		defer podOri.deleteAsAdmin(oc)
		podOri.waitReady(oc)

		exutil.By("Check volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcOri.name, pvcOri.namespace, thinPoolSize)

		exutil.By("Write file to volume")
		podOri.writeDataIntoRawBlockVolume(oc)
		podOri.execCommand(oc, "sync")

		// Create volumesnapshot with pre-defined volumesnapshotclass
		exutil.By("Create volumesnapshot and wait for ready_to_use")
		volumesnapshot := newVolumeSnapshot(setVolumeSnapshotTemplate(volumesnapshotTemplate), setVolumeSnapshotSourcepvcname(pvcOri.name), setVolumeSnapshotVscname(volumeSnapshotClassName))
		volumesnapshot.create(oc)
		defer volumesnapshot.delete(oc)
		volumesnapshot.waitReadyToUse(oc)

		// Set the resource definition for the restore
		pvcRestore := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimVolumemode("Block"), setPersistentVolumeClaimDataSourceName(volumesnapshot.name))
		podRestore := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvcRestore.name), setPodVolumeType("volumeDevices"), setPodPathType("devicePath"), setPodMountPath("/dev/dblock"))

		exutil.By("Create a restored pvc with the lvms storageclass")
		pvcRestore.scname = storageClassName
		pvcRestore.capacity = pvcOri.capacity
		pvcRestore.createWithSnapshotDataSource(oc)
		defer pvcRestore.deleteAsAdmin(oc)

		exutil.By("Create pod with the restored pvc and wait for the pod ready")
		podRestore.create(oc)
		defer podRestore.deleteAsAdmin(oc)
		podRestore.waitReady(oc)

		exutil.By("Check restored volume size is bigger than disk size")
		checkVolumeBiggerThanDisk(oc, pvcRestore.name, pvcRestore.namespace, thinPoolSize)

		exutil.By("Check the file exist in restored volume")
		podRestore.checkDataInRawBlockVolume(oc)
	})

	// author: rdeore@redhat.com
	// OCP-66321-[LVMS] [Filesystem] [ext4] provision a PVC with fsType:'ext4'
	g.It("Author:rdeore-High-66321-[LVMS] [Filesystem] [ext4] provision a PVC with fsType:'ext4'", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate            = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate     = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			storageClassTemplate   = filepath.Join(storageTeamBaseDir, "storageclass-template.yaml")
			storageClass           = newStorageClass(setStorageClassTemplate(storageClassTemplate), setStorageClassProvisioner("topolvm.io"))
			volumeGroup            = "vg1"
			storageClassParameters = map[string]string{
				"csi.storage.k8s.io/fstype": "ext4",
				"topolvm.io/device-class":   volumeGroup,
			}
			extraParameters = map[string]interface{}{
				"parameters":           storageClassParameters,
				"allowVolumeExpansion": true,
			}
		)

		exutil.By("#. Create a new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Create a new lvms storageclass")
		storageClass.createWithExtraParameters(oc, extraParameters)
		defer storageClass.deleteAsAdmin(oc)

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClass.name),
			setPersistentVolumeClaimCapacity("2Gi"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name))

		exutil.By("#. Create a pvc with the lvms storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Check the deployment's pod mounted volume fstype is ext4 by exec mount cmd in the pod")
		dep.checkPodMountedVolumeContain(oc, "ext4")

		exutil.By("#. Check the deployment's pod mounted volume can be read and write")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Check the deployment's pod mounted volume have the exec right")
		dep.checkPodMountedVolumeHaveExecRight(oc)

		exutil.By("#. Check the fsType of volume mounted on the pod located node")
		volName := pvc.getVolumeName(oc)
		nodeName := getNodeNameByPod(oc, dep.namespace, dep.getPodList(oc)[0])
		checkVolumeMountCmdContain(oc, volName, nodeName, "ext4")
	})

	// author: rdeore@redhat.com
	// OCP-66320-[LVMS] Pre-defined CSI Storageclass should get re-created automatically after deleting
	g.It("Author:rdeore-High-66320-[LVMS] Pre-defined CSI Storageclass should get re-created automatically after deleting [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
			storageClass     = newStorageClass()
		)

		exutil.By("#. Check lvms storageclass exists on cluster")
		if !isSpecifiedResourceExist(oc, "sc/"+storageClassName, "") {
			g.Skip("Skipped: the cluster does not have storage-class: " + storageClassName)
		}

		exutil.By("#. Copy pre-defined lvms CSI storageclass configuration in JSON format")
		originSC, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("sc", storageClassName, "-o", "json").Output()
		debugLogf(originSC)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing lvms storageClass")
		o.Expect(oc.WithoutNamespace().AsAdmin().Run("delete").Args("sc", storageClassName).Execute()).ShouldNot(o.HaveOccurred())
		defer func() {
			if !isSpecifiedResourceExist(oc, "sc/"+storageClassName, "") {
				storageClass.createWithExportJSON(oc, originSC, storageClassName)
			}
		}()

		exutil.By("#. Check deleted lvms storageClass is re-created automatically")
		o.Eventually(func() bool {
			return isSpecifiedResourceExist(oc, "sc/"+storageClassName, "")
		}, 30*time.Second, 5*time.Second).Should(o.BeTrue())
	})

	// author: rdeore@redhat.com
	// OCP-66322-[LVMS] Show status column for lvmCluster and show warning event for 'Not Enough Storage capacity' directly from PVC
	g.It("Author:rdeore-High-66322-[LVMS] Show status column for lvmCluster and show warning event for 'Not Enough Storage capacity' directly from PVC", func() {
		// Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
			thinPoolName     = "thin-pool-1"
		)

		exutil.By("Check lvmCluster status is shown in 'oc get' output")
		lvmClusterStatus, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("lvmcluster", "-n", "openshift-storage").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())
		o.Expect(lvmClusterStatus).To(o.ContainSubstring("Ready"))

		exutil.By("Create new project for the scenario")
		oc.SetupProject()

		// Set the resource definitions
		pvcCapacity := strconv.FormatInt(int64(getOverProvisionLimitByVolumeGroup(oc, volumeGroup, thinPoolName))+getRandomNum(10, 20), 10) + "Gi"
		e2e.Logf("PVC capacity in Gi: %s", pvcCapacity)
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimCapacity(pvcCapacity))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("Create a pvc with the pre-defined lvms csi storageclass")
		pvc.scname = storageClassName
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create pod with the created pvc and check status is Pending")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.checkStatusConsistently(oc, "Pending", 30)

		exutil.By("Check warning event is generated for a pvc resource")
		waitResourceSpecifiedEventsOccurred(oc, pvc.namespace, pvc.name, "NotEnoughCapacity", "Requested storage ("+pvc.capacity+") is greater than available capacity on any node")
	})

	// author: rdeore@redhat.com
	// OCP-66764-[LVMS] Show warning event for 'Removed Claim Reference' directly from PV
	g.It("Author:rdeore-High-66764-[LVMS] Show warning event for 'Removed Claim Reference' directly from PV", func() {
		// Set the resource template for the scenario
		var (
			pvcTemplate      = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate      = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			volumeGroup      = "vg1"
			storageClassName = "lvms-" + volumeGroup
		)

		exutil.By("Create new project for the scenario")
		oc.SetupProject()

		// Set the resource definitions
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("Create a pvc with the pre-defined lvms csi storageclass")
		pvc.scname = storageClassName
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("Create pod with the pvc and wait for pod to be ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("Remove claim reference from pv bound to pvc")
		pvName := pvc.getVolumeName(oc)
		pvPatch := `{"spec":{"claimRef": null}}`
		patchResourceAsAdmin(oc, "", "pv/"+pvName, pvPatch, "merge")
		defer deleteSpecifiedResource(oc.AsAdmin(), "logicalvolume", pvName, "")
		defer deleteSpecifiedResource(oc.AsAdmin(), "pv", pvName, "")

		exutil.By("Check warning event is generated for a pv resource")
		waitResourceSpecifiedEventsOccurred(oc, "default", pvName, "ClaimReferenceRemoved",
			"Claim reference has been removed. This PV is no longer dynamically managed by LVM Storage and will need to be cleaned up manually")

		exutil.By("Delete Pod and Pvc to clean-up the pv automatically by lvms operator")
		deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)
	})

	// author: rdeore@redhat.com
	// OCP-67001-[LVMS] Check deviceSelector logic works with combination of one valid device Path and two optionalPaths
	g.It("Author:rdeore-High-67001-[LVMS] Check deviceSelector logic works with combination of one valid device Path and two optionalPaths [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-paths-template.yaml")
			volumeGroup        = "vg1"
		)

		if exutil.IsSNOCluster(oc) {
			g.Skip("Skipped: test case is only applicable to multi-node/SNO with additional worker-node cluster")
		}

		exutil.By("#. Get list of available block devices/disks attached to all worker ndoes")
		freeDiskNameCountMap := getListOfFreeDisksFromWorkerNodes(oc)
		if len(freeDiskNameCountMap) < 2 { // this test requires atleast 2 unique disks, 1 for mandatoryDevicePath and 1 for optionalDevicePath
			g.Skip("Skipped: Cluster's Worker nodes does not have minimum required free block devices/disks attached")
		}
		workerNodeCount := len(getWorkersList(oc))
		var mandatoryDisk string
		var optionalDisk string
		isDiskFound := false
		for diskName, count := range freeDiskNameCountMap {
			if count == int64(workerNodeCount) { // mandatory disk with same name should be present on all worker nodes as per LVMS requriement
				mandatoryDisk = diskName
				isDiskFound = true
				delete(freeDiskNameCountMap, diskName)
				break
			}
		}
		if !isDiskFound { // If all Worker nodes doesn't have 1 disk with same name, skip the test scenario
			g.Skip("Skipped: All Worker nodes does not have a free block device/disk with same name attached")
		}
		for diskName := range freeDiskNameCountMap {
			optionalDisk = diskName
			break
		}

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		debugLogf(originLVMJSON)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()

		exutil.By("#. Create a new LVMCluster resource with paths and optionalPaths")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterPaths([]string{"/dev/" + mandatoryDisk}),
			setLvmClusterOptionalPaths([]string{"/dev/" + optionalDisk, "/dev/invalid-path"}))
		lvmCluster.create(oc)
		defer lvmCluster.deleteLVMClusterSafely(oc) // If new lvmCluster creation fails, need to remove finalizers if any
		lvmCluster.waitReady(oc)

		exutil.By("#. Check LVMS CSI storage capacity equals backend devices/disks total size")
		pathsDiskTotalSize := getTotalDiskSizeOnAllWorkers(oc, "/dev/"+mandatoryDisk)
		optionalPathsDiskTotalSize := getTotalDiskSizeOnAllWorkers(oc, "/dev/"+optionalDisk)
		ratio, sizePercent := getOverProvisionRatioAndSizePercentByVolumeGroup(oc, "vg1")
		expectedStorageCapacity := sizePercent * (pathsDiskTotalSize + optionalPathsDiskTotalSize) / 100
		e2e.Logf("EXPECTED USABLE STORAGE CAPACITY: %d", expectedStorageCapacity)
		currentLvmStorageCapacity := lvmCluster.getCurrentTotalLvmStorageCapacityByStorageClass(oc, "lvms-vg1")
		actualStorageCapacity := (currentLvmStorageCapacity / ratio) / 1024 // Get size in Gi
		e2e.Logf("ACTUAL USABLE STORAGE CAPACITY: %d", actualStorageCapacity)
		storageDiff := float64(expectedStorageCapacity - actualStorageCapacity)
		absDiff := math.Abs(storageDiff)
		o.Expect(int(absDiff) < 2).To(o.BeTrue()) // there is always a difference of 1 Gi between backend disk size and usable size

		exutil.By("#. Create a new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("#. Create a pvc with the pre-set lvms csi storageclass")
		pvc.scname = "lvms-" + volumeGroup
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("#. Write file to volume")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("Delete Pod and PVC")
		deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("Delete newly created LVMCluster resource")
		lvmCluster.deleteLVMClusterSafely(oc)

		exutil.By("#. Create original LVMCluster resource")
		originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
		originLvmCluster.waitReady(oc)
	})

	// author: rdeore@redhat.com
	// OCP-67002-[LVMS] Check deviceSelector logic works with only optional paths
	g.It("Author:rdeore-High-67002-[LVMS] Check deviceSelector logic works with only optional paths [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-paths-template.yaml")
			volumeGroup        = "vg1"
		)

		if exutil.IsSNOCluster(oc) {
			g.Skip("Skipped: test case is only applicable to multi-node/SNO with additional worker-node cluster")
		}

		exutil.By("#. Get list of available block devices/disks attached to all worker ndoes")
		freeDiskNameCountMap := getListOfFreeDisksFromWorkerNodes(oc)
		if len(freeDiskNameCountMap) < 1 { // this test requires atleast 1 unique disk for optional Device Path
			g.Skip("Skipped: Cluster's Worker nodes does not have minimum required free block devices/disks attached")
		}
		workerNodeCount := len(getWorkersList(oc))
		var optionalDisk string
		isDiskFound := false
		for diskName, count := range freeDiskNameCountMap {
			if count == int64(workerNodeCount) { // mandatory disk with same name should be present on all worker nodes as per LVMS requriement
				optionalDisk = diskName
				isDiskFound = true
				delete(freeDiskNameCountMap, diskName)
				break
			}
		}
		if !isDiskFound { // If all Worker nodes doesn't have 1 disk with same name, skip the test scenario
			g.Skip("Skipped: All Worker nodes does not have a free block device/disk with same name attached")
		}

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		debugLogf(originLVMJSON)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")

		exutil.By("#. Create a new LVMCluster resource with optional paths")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterOptionalPaths([]string{"/dev/" + optionalDisk, "/dev/invalid-path"}))
		defer lvmCluster.deleteLVMClusterSafely(oc) // If new lvmCluster creation fails, need to remove finalizers if any
		lvmCluster.createWithoutMandatoryPaths(oc)
		lvmCluster.waitReady(oc)

		exutil.By("#. Check LVMS CSI storage capacity equals backend devices/disks total size")
		optionalPathsDiskTotalSize := getTotalDiskSizeOnAllWorkers(oc, "/dev/"+optionalDisk)
		ratio, sizePercent := getOverProvisionRatioAndSizePercentByVolumeGroup(oc, "vg1")
		expectedStorageCapacity := sizePercent * optionalPathsDiskTotalSize / 100
		e2e.Logf("EXPECTED USABLE STORAGE CAPACITY: %d", expectedStorageCapacity)
		currentLvmStorageCapacity := lvmCluster.getCurrentTotalLvmStorageCapacityByStorageClass(oc, "lvms-vg1")
		actualStorageCapacity := (currentLvmStorageCapacity / ratio) / 1024 // Get size in Gi
		e2e.Logf("ACTUAL USABLE STORAGE CAPACITY: %d", actualStorageCapacity)
		storageDiff := float64(expectedStorageCapacity - actualStorageCapacity)
		absDiff := math.Abs(storageDiff)
		o.Expect(int(absDiff) < 2).To(o.BeTrue()) // there is always a difference of 1 Gi between backend disk size and usable size

		exutil.By("#. Create a new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("#. Create a pvc with the pre-set lvms csi storageclass")
		pvc.scname = "lvms-" + volumeGroup
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("#. Write file to volume")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("Delete Pod and PVC")
		deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("Delete newly created LVMCluster resource")
		lvmCluster.deleteLVMClusterSafely(oc)
	})

	// author: rdeore@redhat.com
	// OCP-67003-[LVMS] Check deviceSelector logic shows error when only optionalPaths are used which are invalid device paths
	g.It("Author:rdeore-High-67003-[LVMS] Check deviceSelector logic shows error when only optionalPaths are used which are invalid device paths [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-paths-template.yaml")
		)

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		debugLogf(originLVMJSON)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")

		exutil.By("#. Create a new LVMCluster resource with invalid optional paths")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterOptionalPaths([]string{"/dev/invalid-path1", "/dev/invalid-path2"}))
		defer lvmCluster.deleteLVMClusterSafely(oc) // If new lvmCluster creation fails, need to remove finalizers if any
		lvmCluster.createWithoutMandatoryPaths(oc)

		exutil.By("#. Check LVMCluster state is 'Failed' with proper error reason")
		lvmCluster.getLvmClusterStatus(oc)
		o.Eventually(func() string {
			lvmClusterState, _ := lvmCluster.getLvmClusterStatus(oc)
			return lvmClusterState
		}, 120*time.Second, 5*time.Second).Should(o.Equal("Failed"))
		errMsg := "at least 1 valid device is required if DeviceSelector paths or optionalPaths are specified"
		errorDesc := lvmCluster.describeLvmCluster(oc)
		e2e.Logf("LVMCluster resource description: " + errorDesc)
		o.Expect(strings.Contains(strings.ToLower(errorDesc), strings.ToLower(errMsg))).To(o.BeTrue())

		exutil.By("Delete newly created LVMCluster resource")
		lvmCluster.deleteLVMClusterSafely(oc)
	})

	// author: rdeore@redhat.com
	// OCP-67004-[LVMS] Check deviceSelector logic shows error when identical device path is used in both paths and optionalPaths
	g.It("Author:rdeore-High-67004-[LVMS] Check deviceSelector logic shows error when identical device path is used in both paths and optionalPaths [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-paths-template.yaml")
		)

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		debugLogf(originLVMJSON)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")

		exutil.By("#. Attempt creating a new LVMCluster resource with identical mandatory and optional device paths")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterPaths([]string{"/dev/diskpath-1"}),
			setLvmClusterOptionalPaths([]string{"/dev/diskpath-1", "/dev/diskpath-2"}))
		defer lvmCluster.deleteLVMClusterSafely(oc)
		errorMsg, _ := lvmCluster.createToExpectError(oc)
		e2e.Logf("LVMCluster creation error: " + errorMsg)

		exutil.By("#. Check LVMCluster creation failed with proper error reason")
		expectedErrorSubStr := "error: optional device path /dev/diskpath-1 is specified at multiple places in deviceClass " + lvmCluster.deviceClassName
		o.Expect(strings.ToLower(errorMsg)).To(o.ContainSubstring(strings.ToLower(expectedErrorSubStr)))
	})

	// OCP-69191-[LVMS] [Filesystem] Support provisioning less than 1Gi size PV and re-size
	g.It("Author:rdeore-Critical-69191-[LVMS] [Filesystem] Support provisioning less than 1Gi size PV and re-size", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("14Mi"), setPersistentVolumeClaimNamespace(oc.Namespace()))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentNamespace(oc.Namespace()))

		exutil.By("#. Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Write data in pod mounted volume")
		dep.checkPodMountedVolumeCouldRW(oc)

		exutil.By("#. Resize PVC storage capacity to a value bigger than previous value and less than 1Gi")
		pvcSizeInt64, _ := strconv.ParseInt(pvc.capacity, 10, 64)
		newPvcSizeInt64 := getRandomNum(pvcSizeInt64+50, pvcSizeInt64+1000)
		newPvcSize := strconv.FormatInt(newPvcSizeInt64, 10) + "Mi"
		pvc.resizeAndCheckDataIntegrity(oc, dep, newPvcSize)

		exutil.By("#. Resize PVC storage capacity to a value bigger than 1Gi")
		pvc.resizeAndCheckDataIntegrity(oc, dep, "2Gi")
	})

	// OCP-69753-[LVMS] [Block] Support provisioning less than 1Gi size PV and re-size
	g.It("Author:rdeore-Critical-69753-[LVMS] [Block] Support provisioning less than 1Gi size PV and re-size", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			deploymentTemplate = filepath.Join(storageTeamBaseDir, "dep-template.yaml")
			volumeGroup        = "vg1"
			storageClassName   = "lvms-" + volumeGroup
		)

		exutil.By("#. Create new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate), setPersistentVolumeClaimStorageClassName(storageClassName),
			setPersistentVolumeClaimCapacity("14Mi"), setPersistentVolumeClaimNamespace(oc.Namespace()), setPersistentVolumeClaimVolumemode("Block"))
		dep := newDeployment(setDeploymentTemplate(deploymentTemplate), setDeploymentPVCName(pvc.name), setDeploymentVolumeType("volumeDevices"),
			setDeploymentVolumeTypePath("devicePath"), setDeploymentMountpath("/dev/dblock"), setDeploymentNamespace(oc.Namespace()))

		exutil.By("#. Create a pvc with the csi storageclass")
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
		dep.create(oc)
		defer dep.deleteAsAdmin(oc)

		exutil.By("#. Wait for the deployment ready")
		dep.waitReady(oc)

		exutil.By("#. Write data in pod mounted volume")
		dep.writeDataBlockType(oc)

		exutil.By("#. Resize PVC storage capacity to a value bigger than previous value and less than 1Gi")
		pvcSizeInt64, _ := strconv.ParseInt(pvc.capacity, 10, 64)
		newPvcSizeInt64 := getRandomNum(pvcSizeInt64+50, pvcSizeInt64+1000)
		newPvcSize := strconv.FormatInt(newPvcSizeInt64, 10) + "Mi"
		pvc.resizeAndCheckDataIntegrity(oc, dep, newPvcSize)

		exutil.By("#. Resize PVC storage capacity to a value bigger than 1Gi")
		pvc.resizeAndCheckDataIntegrity(oc, dep, "2Gi")
	})

	// author: rdeore@redhat.com
	// OCP-69611-[LVMS] Check optionalPaths work as expected with nodeSelector on multi-node OCP cluster
	g.It("Author:rdeore-High-69611-[LVMS] Check optionalPaths work as expected with nodeSelector on multi-node OCP cluster [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-optional-paths-template.yaml")
			volumeGroup        = "vg1"
		)

		if exutil.IsSNOCluster(oc) {
			g.Skip("Skipped: test case is only applicable to multi-node/SNO with additional worker-node cluster")
		}

		exutil.By("#. Get list of available block devices/disks attached to all worker ndoes")
		freeDiskNameCountMap := getListOfFreeDisksFromWorkerNodes(oc)
		if len(freeDiskNameCountMap) < 1 { // this test requires atleast 1 unique disk for optional Device Path
			g.Skip("Skipped: Cluster's Worker nodes does not have minimum two required free block devices/disks attached")
		}
		workerNodeList := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		workerNodeCount := len(workerNodeList)
		var optionalDisk string
		isDiskFound := false
		for diskName, count := range freeDiskNameCountMap {
			if count == int64(workerNodeCount) { // optional disk with same device-path should be present on all worker nodes as per LVMS requriement
				optionalDisk = diskName
				isDiskFound = true
				break
			}
		}
		if !isDiskFound { // If all worker nodes doesn't have atleast one disk with same device-path, skip the test scenario
			g.Skip("Skipped: All Worker nodes does not have a free block device/disk attached with same device-path")
		}

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")

		exutil.By("#. Create a new LVMCluster resource with node-selector and optional paths")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterPaths([]string{""}),
			setLvmClusterOptionalPaths([]string{"/dev/" + optionalDisk, "/dev/invalid-path"}))
		defer lvmCluster.deleteLVMClusterSafely(oc) // If new lvmCluster creation fails, need to remove finalizers if present
		lvmCluster.createWithNodeSelector(oc, "kubernetes.io/hostname", "In", []string{workerNodeList[0].name, workerNodeList[1].name})
		lvmCluster.waitReady(oc)

		exutil.By("#. Check LVMCluster CR definition has entry for only two worker nodes")
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].status.deviceClassStatuses[0].nodeStatus[*].node}").Output()
		workerNodesInUse := strings.Split(output, " ")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(workerNodesInUse) == 2).To(o.BeTrue())
		matchedWorkers := sliceIntersect([]string{workerNodeList[0].name, workerNodeList[1].name}, workerNodesInUse)
		o.Expect(len(matchedWorkers) == 2).To(o.BeTrue())

		exutil.By("#. Check there are exactly two pods with component name 'vg-manager' and 'topolvm-node' in LVMS namespace")
		vgManagerPodList, err := getPodsListByLabel(oc.AsAdmin(), "openshift-storage", "app.kubernetes.io/component=vg-manager")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(vgManagerPodList) == 2).To(o.BeTrue())
		topoLvmNodePodList, err := getPodsListByLabel(oc.AsAdmin(), "openshift-storage", "app.kubernetes.io/component=topolvm-node")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(topoLvmNodePodList) == 2).To(o.BeTrue())

		exutil.By("#. Create a new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("#. Create a pvc with the pre-set lvms csi storageclass")
		pvc.scname = "lvms-" + volumeGroup
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("#. Write file to volume")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("Delete Pod and PVC")
		deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("Delete newly created LVMCluster resource")
		lvmCluster.deleteLVMClusterSafely(oc)
	})

	// author: rdeore@redhat.com
	// OCP-69772-[LVMS] Check LVMS operator should work with user created RAID volume as devicePath
	g.It("Author:rdeore-High-69772-[LVMS] Check LVMS operator should work with user created RAID volume as devicePath [Disruptive]", func() {
		//Set the resource template for the scenario
		var (
			pvcTemplate        = filepath.Join(storageTeamBaseDir, "pvc-template.yaml")
			podTemplate        = filepath.Join(storageTeamBaseDir, "pod-template.yaml")
			lvmClusterTemplate = filepath.Join(storageLvmsBaseDir, "lvmcluster-with-paths-template.yaml")
			volumeGroup        = "vg1"
		)
		workerNodeList := getSchedulableLinuxWorkers(getAllNodesInfo(oc))
		workerNodeCount := len(workerNodeList)

		exutil.By("#. Check all worker nodes have at least two additional block devices/disks attached")
		freeDisksCountMap := getLVMSUsableDiskCountFromWorkerNodes(oc)
		if len(freeDisksCountMap) != workerNodeCount { // test case requires all worker nodes to have additional disks/block devices attached
			g.Skip("Skipped: Cluster's worker nodes does not have minimum required free block devices/disks attached")
		}
		for _, diskCount := range freeDisksCountMap {
			if diskCount < 2 { // atleast two additional disks/block devices should be present on all worker nodes
				g.Skip("Skipped: Cluster's worker nodes does not have minimum required two free block devices/disks attached")
			}
		}

		exutil.By("#. Copy and save existing LVMCluster configuration in JSON format")
		lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		originLvmCluster := newLvmCluster(setLvmClusterName(lvmClusterName), setLvmClusterNamespace("openshift-storage"))
		originLVMJSON, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", originLvmCluster.name, "-n", "openshift-storage", "-o", "json").Output()
		debugLogf(originLVMJSON)
		o.Expect(err).ShouldNot(o.HaveOccurred())

		exutil.By("#. Delete existing LVMCluster resource")
		deleteSpecifiedResource(oc.AsAdmin(), "lvmcluster", originLvmCluster.name, "openshift-storage")
		defer func() {
			if !isSpecifiedResourceExist(oc, "lvmcluster/"+originLvmCluster.name, "openshift-storage") {
				originLvmCluster.createWithExportJSON(oc, originLVMJSON, originLvmCluster.name)
			}
			originLvmCluster.waitReady(oc)
		}()

		exutil.By("#. Create a RAID disk on each worker node")
		raidDiskName := "md69772"
		defer func() {
			for _, workerNode := range workerNodeList {
				removeRAIDLevelDisk(oc, workerNode.name, raidDiskName)
			}
		}()
		for _, workerNode := range workerNodeList {
			createRAIDLevel1Disk(oc, workerNode.name, raidDiskName)
		}

		exutil.By("#. Create a new LVMCluster resource using RAID disk as a mandatory path")
		lvmCluster := newLvmCluster(setLvmClustertemplate(lvmClusterTemplate), setLvmClusterPaths([]string{"/dev/" + raidDiskName}))
		lvmCluster.createWithoutOptionalPaths(oc)
		defer lvmCluster.deleteLVMClusterSafely(oc) // If new lvmCluster creation fails, need to remove finalizers if any
		lvmCluster.waitReady(oc)

		exutil.By("#. Create a new project for the scenario")
		oc.SetupProject()

		exutil.By("#. Define storage resources")
		pvc := newPersistentVolumeClaim(setPersistentVolumeClaimTemplate(pvcTemplate))
		pod := newPod(setPodTemplate(podTemplate), setPodPersistentVolumeClaim(pvc.name))

		exutil.By("#. Create a pvc with the pre-set lvms csi storageclass")
		pvc.scname = "lvms-" + volumeGroup
		pvc.create(oc)
		defer pvc.deleteAsAdmin(oc)

		exutil.By("#. Create pod with the created pvc and wait for the pod ready")
		pod.create(oc)
		defer pod.deleteAsAdmin(oc)
		pod.waitReady(oc)

		exutil.By("#. Write file to volume")
		pod.checkMountedVolumeCouldRW(oc)

		exutil.By("#. Delete Pod and PVC")
		deleteSpecifiedResource(oc, "pod", pod.name, pod.namespace)
		deleteSpecifiedResource(oc, "pvc", pvc.name, pvc.namespace)

		exutil.By("#. Delete newly created LVMCluster resource")
		lvmCluster.deleteLVMClusterSafely(oc)
	})
})

func checkVolumeBiggerThanDisk(oc *exutil.CLI, pvcName string, pvcNamespace string, thinPoolSize int) {
	pvSize, _err := getVolSizeFromPvc(oc, pvcName, pvcNamespace)
	o.Expect(_err).NotTo(o.HaveOccurred())
	regexForNumbersOnly := regexp.MustCompile("[0-9]+")
	pvSizeVal := regexForNumbersOnly.FindAllString(pvSize, -1)[0]
	pvSizeNum, err := strconv.Atoi(pvSizeVal)
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Persistent volume Size in Gi: %d", pvSizeNum)
	o.Expect(pvSizeNum > thinPoolSize).Should(o.BeTrue())
}

func checkLvmsOperatorInstalled(oc *exutil.CLI) {
	csiDriver, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("csidriver", "topolvm.io").Output()
	if err != nil || strings.Contains(csiDriver, "not found") {
		g.Skip("LVMS Operator is not installed on the running OCP cluster")
	}
	lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(lvmClusterName).NotTo(o.BeEmpty())
	o.Eventually(func() string {
		lvmClusterState, err := getLvmClusterState(oc.AsAdmin(), "openshift-storage", lvmClusterName)
		o.Expect(err).NotTo(o.HaveOccurred())
		return lvmClusterState
	}, 30*time.Second, 5*time.Second).Should(o.Equal("Ready"))
}

// Get the current state of LVM Cluster
func getLvmClusterState(oc *exutil.CLI, namespace string, lvmClusterName string) (string, error) {
	lvmCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o", "json").Output()
	lvmClusterState := gjson.Get(lvmCluster, "items.#(metadata.name="+lvmClusterName+").status.state").String()
	e2e.Logf("The current LVM Cluster state is %q", lvmClusterState)
	return lvmClusterState, err
}

// Get the total thinPoolSize for given volumeGroup from all available worker nodes on the cluster
func getThinPoolSizeByVolumeGroup(oc *exutil.CLI, volumeGroup string, thinPoolName string) int {
	cmd := "lvs --units g 2> /dev/null | grep " + volumeGroup + " | awk '{if ($1 == \"" + thinPoolName + "\") print $4;}'"
	workerNodes := getWorkersList(oc)
	var totalThinPoolSize int = 0
	for _, workerName := range workerNodes { // Search all worker nodes to fetch thin-pool-size by VG
		output, err := execCommandInSpecificNode(oc, workerName, cmd)
		o.Expect(err).NotTo(o.HaveOccurred())
		regexForNumbersOnly := regexp.MustCompile("[0-9.]+")
		sizeVal := regexForNumbersOnly.FindAllString(output, -1)[0]
		sizeNum := strings.Split(sizeVal, ".")
		thinPoolSize, err := strconv.Atoi(sizeNum[0])
		o.Expect(err).NotTo(o.HaveOccurred())
		totalThinPoolSize = totalThinPoolSize + thinPoolSize
	}
	e2e.Logf("Total thin Pool size in Gi from backend nodes: %d", totalThinPoolSize)
	return totalThinPoolSize
}

// Get OverProvision Ratio value and Size Percent value from lvmCluster config
func getOverProvisionRatioAndSizePercentByVolumeGroup(oc *exutil.CLI, volumeGroup string) (int, int) {
	lvmCluster, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o", "json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	lvmClusterName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("lvmcluster", "-n", "openshift-storage", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	overProvisionRatio := gjson.Get(lvmCluster, "items.#(metadata.name="+lvmClusterName+").spec.storage.deviceClasses.#(name="+volumeGroup+").thinPoolConfig.overprovisionRatio")
	overProvisionRatioStr := overProvisionRatio.String()
	o.Expect(overProvisionRatioStr).NotTo(o.BeEmpty())
	e2e.Logf("Over-Provision Ratio: %s", overProvisionRatioStr)
	opRatio, err := strconv.Atoi(strings.TrimSpace(overProvisionRatioStr))
	o.Expect(err).NotTo(o.HaveOccurred())
	sizePercent := gjson.Get(lvmCluster, "items.#(metadata.name="+lvmClusterName+").spec.storage.deviceClasses.#(name="+volumeGroup+").thinPoolConfig.sizePercent")
	sizePercentStr := sizePercent.String()
	o.Expect(sizePercentStr).NotTo(o.BeEmpty())
	e2e.Logf("Size-percent: %s", sizePercentStr)
	sizePercentNum, err := strconv.Atoi(strings.TrimSpace(sizePercentStr))
	o.Expect(err).NotTo(o.HaveOccurred())
	return opRatio, sizePercentNum
}

func getOverProvisionLimitByVolumeGroup(oc *exutil.CLI, volumeGroup string, thinPoolName string) int {
	thinPoolSize := getThinPoolSizeByVolumeGroup(oc, volumeGroup, thinPoolName)
	opRatio, _ := getOverProvisionRatioAndSizePercentByVolumeGroup(oc, volumeGroup)
	limit := thinPoolSize * opRatio
	e2e.Logf("Over-Provisioning Limit in Gi: %d", limit)
	return limit
}

// Performing test steps for LVMS PVC volume Resizing
func resizeLvmsVolume(oc *exutil.CLI, pvc persistentVolumeClaim, dep deployment, expandedCapactiyInt64 int64) {
	// Set up a specified project share for all the phases
	exutil.By("#. Create a pvc with the csi storageclass")
	pvc.create(oc)
	defer pvc.deleteAsAdmin(oc)

	exutil.By("#. Create deployment with the created pvc and wait for the pod ready")
	dep.create(oc)
	defer dep.deleteAsAdmin(oc)

	exutil.By("#. Wait for the deployment ready")
	dep.waitReady(oc)

	exutil.By("#. Write data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeCouldRW(oc)
	} else {
		dep.writeDataBlockType(oc)
	}

	exutil.By("#. Apply the patch to Resize the pvc volume")
	capacityInt64, err := strconv.ParseInt(strings.TrimRight(pvc.capacity, "Gi"), 10, 64)
	o.Expect(err).NotTo(o.HaveOccurred())
	expandedCapactiy := strconv.FormatInt(expandedCapactiyInt64, 10) + "Gi"
	o.Expect(applyVolumeResizePatch(oc, pvc.name, pvc.namespace, expandedCapactiy)).To(o.ContainSubstring("patched"))
	pvc.capacity = expandedCapactiy

	exutil.By("#. Waiting for the pvc capacity update sucessfully")
	waitPVVolSizeToGetResized(oc, pvc.namespace, pvc.name, pvc.capacity)
	pvc.waitResizeSuccess(oc, pvc.capacity)

	exutil.By("#. Check origin data intact and write new data in pod")
	if dep.typepath == "mountPath" {
		dep.checkPodMountedVolumeDataExist(oc, true)
		// After volume expand write data more than the old capacity should succeed
		msg, err := execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(capacityInt64+1, 10)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).NotTo(o.ContainSubstring("No space left on device"))
		// Continue write data more than new capacity should fail of "No space left on device"
		msg, err = execCommandInSpecificPod(oc, pvc.namespace, dep.getPodList(oc)[0], "fallocate -l "+strconv.FormatInt(expandedCapactiyInt64-capacityInt64, 10)+"G "+dep.mpath+"/"+getRandomString()+" ||true")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(msg).To(o.ContainSubstring("No space left on device"))
	} else {
		// Since fallocate doesn't support raw block write and dd cmd write big file is too slow, just check origin data intact
		dep.checkDataBlockType(oc)
		dep.writeDataBlockType(oc)
	}
}
