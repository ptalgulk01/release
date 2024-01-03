package clusterinfrastructure

import (
	"path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
)

var _ = g.Describe("[sig-cluster-lifecycle] Cluster_Infrastructure", func() {
	defer g.GinkgoRecover()
	var (
		oc                         = exutil.NewCLI("capi-machines", exutil.KubeConfigPath())
		iaasPlatform               string
		clusterID                  string
		region                     string
		host                       string
		profile                    string
		instanceType               string
		zone                       string
		ami                        string
		subnetName                 string
		subnetID                   string
		sgName                     string
		image                      string
		machineType                string
		network                    string
		subnetwork                 string
		capiBaseDir                string
		clusterTemplate            string
		awsClusterTemplate         string
		awsMachineTemplateTemplate string
		gcpClusterTemplate         string
		gcpMachineTemplateTemplate string
		capiMachinesetAWSTemplate  string
		capiMachinesetgcpTemplate  string
		err                        error
		cluster                    clusterDescription
		awscluster                 awsClusterDescription
		awsMachineTemplate         awsMachineTemplateDescription
		gcpcluster                 gcpClusterDescription
		gcpMachineTemplate         gcpMachineTemplateDescription
		capiMachineSetAWS          capiMachineSetAWSDescription
		capiMachineSetgcp          capiMachineSetgcpDescription
		clusterNotInCapi           clusterDescriptionNotInCapi
	)

	g.BeforeEach(func() {
		exutil.SkipForSNOCluster(oc)
		exutil.SkipConditionally(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		switch iaasPlatform {
		case "aws":
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.aws.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := exutil.GetRandomMachineSetName(oc)
			profile, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.iamInstanceProfile.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			instanceType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.instanceType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.placement.availabilityZone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			ami, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.ami.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.subnet.id}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			sgName, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.securityGroups[0].filters[0].values[0]}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		case "gcp":
			region, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.gcp.region}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			randomMachinesetName := exutil.GetRandomMachineSetName(oc)
			zone, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.zone}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			image, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.disks[0].image}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			machineType, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.machineType}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			network, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkInterfaces[0].network}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			subnetwork, err = oc.AsAdmin().WithoutNamespace().Run("get").Args(mapiMachineset, randomMachinesetName, "-n", machineAPINamespace, "-o=jsonpath={.spec.template.spec.providerSpec.value.networkInterfaces[0].subnetwork}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
		default:
			g.Skip("IAAS platform is " + iaasPlatform + " which is NOT supported cluster api ...")
		}
		clusterID, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		apiServerInternalURI, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.apiServerInternalURI}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		start := strings.Index(apiServerInternalURI, "://")
		end := strings.LastIndex(apiServerInternalURI, ":")
		host = apiServerInternalURI[start+3 : end]

		capiBaseDir = exutil.FixturePath("testdata", "clusterinfrastructure", "capi")
		clusterTemplate = filepath.Join(capiBaseDir, "cluster.yaml")
		awsClusterTemplate = filepath.Join(capiBaseDir, "awscluster.yaml")
		if subnetName != "" {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws.yaml")
		} else {
			awsMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-aws-id.yaml")
		}
		gcpClusterTemplate = filepath.Join(capiBaseDir, "gcpcluster.yaml")
		gcpMachineTemplateTemplate = filepath.Join(capiBaseDir, "machinetemplate-gcp.yaml")
		capiMachinesetAWSTemplate = filepath.Join(capiBaseDir, "machinesetaws.yaml")
		capiMachinesetgcpTemplate = filepath.Join(capiBaseDir, "machinesetgcp.yaml")
		cluster = clusterDescription{
			name:     clusterID,
			template: clusterTemplate,
		}
		clusterNotInCapi = clusterDescriptionNotInCapi{
			name:      clusterID,
			namespace: "openshift-machine-api",
			template:  clusterTemplate,
		}
		awscluster = awsClusterDescription{
			name:     clusterID,
			region:   region,
			host:     host,
			template: awsClusterTemplate,
		}
		gcpcluster = gcpClusterDescription{
			name:     clusterID,
			region:   region,
			host:     host,
			network:  network,
			template: gcpClusterTemplate,
		}
		awsMachineTemplate = awsMachineTemplateDescription{
			name:         "aws-machinetemplate",
			profile:      profile,
			instanceType: instanceType,
			zone:         zone,
			ami:          ami,
			subnetName:   subnetName,
			sgName:       sgName,
			subnetID:     subnetID,
			template:     awsMachineTemplateTemplate,
		}
		gcpMachineTemplate = gcpMachineTemplateDescription{
			name:        "gcp-machinetemplate",
			region:      region,
			image:       image,
			machineType: machineType,
			subnetwork:  subnetwork,
			clusterID:   clusterID,
			template:    gcpMachineTemplateTemplate,
		}
		capiMachineSetAWS = capiMachineSetAWSDescription{
			name:        "capi-machineset",
			clusterName: clusterID,
			template:    capiMachinesetAWSTemplate,
			replicas:    1,
		}
		capiMachineSetgcp = capiMachineSetgcpDescription{
			name:          "capi-machineset-gcp",
			clusterName:   clusterID,
			template:      capiMachinesetgcpTemplate,
			failureDomain: zone,
			replicas:      1,
		}

		switch iaasPlatform {
		case "aws":
			cluster.kind = "AWSCluster"
			clusterNotInCapi.kind = "AWSCluster"
			capiMachineSetAWS.kind = "AWSMachineTemplate"
			capiMachineSetAWS.machineTemplateName = awsMachineTemplate.name
		case "gcp":
			cluster.kind = "GCPCluster"
			clusterNotInCapi.kind = "GCPCluster"
			capiMachineSetgcp.kind = "GCPMachineTemplate"
			capiMachineSetgcp.machineTemplateName = gcpMachineTemplate.name
			capiMachineSetgcp.failureDomain = zone

		default:
			g.Skip("IAAS platform is " + iaasPlatform + " which is NOT supported cluster api ...")
		}
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-High-51071-[CAPI] Create machineset with CAPI on aws [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		cluster.createCluster(oc)
		defer awscluster.deleteAWSCluster(oc)
		awscluster.createAWSCluster(oc)
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)

		capiMachineSetAWS.name = "capi-machineset-51071"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})

	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-High-53100-[CAPI] Create machineset with CAPI on gcp [Disruptive][Slow]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "gcp")
		skipForCAPINotExist(oc)

		g.By("Create capi machineset")
		cluster.createCluster(oc)
		defer gcpcluster.deleteGCPCluster(oc)
		gcpcluster.createGCPCluster(oc)

		defer gcpMachineTemplate.deleteGCPMachineTemplate(oc)
		gcpMachineTemplate.createGCPMachineTemplate(oc)

		capiMachineSetgcp.name = "capi-machineset-53100"
		defer waitForCapiMachinesDisappergcp(oc, capiMachineSetgcp.name)
		defer capiMachineSetgcp.deleteCapiMachineSetgcp(oc)
		capiMachineSetgcp.createCapiMachineSetgcp(oc)

	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-Author:zhsun-medium-55205-[CAPI] Webhook validations for CAPI [Disruptive]", func() {
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")
		skipForCAPINotExist(oc)

		g.By("Shouldn't allow to create/update cluster with invalid kind")
		clusters, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("cluster", "-n", clusterAPINamespace, "-o=jsonpath={.items[*].metadata.name}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if len(clusters) == 0 {
			cluster.createCluster(oc)
		}

		clusterKind, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args("cluster", cluster.name, "-n", clusterAPINamespace, "-p", `{"spec":{"infrastructureRef":{"kind":"invalid"}}}`, "--type=merge").Output()
		o.Expect(clusterKind).To(o.ContainSubstring("invalid"))

		g.By("Shouldn't allow to delete cluster")
		clusterDelete, _ := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", cluster.name, "-n", clusterAPINamespace).Output()
		o.Expect(clusterDelete).To(o.ContainSubstring("deletion of cluster is not allowed"))
	})

	// author: miyadav@redhat.com
	g.It("NonHyperShiftHOST-Author:miyadav-high-69188-[CAPI] cluster object can be deleted in non-cluster-api namespace [Disruptive]", func() {
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws", "gcp")
		skipForCAPINotExist(oc)
		iaasPlatform = exutil.CheckPlatform(oc)
		g.By("Create cluster object in namespace other than openshift-cluster-api")
		defer oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", clusterNotInCapi.name, "-n", clusterNotInCapi.namespace).Execute()
		clusterNotInCapi.createClusterNotInCapiNamespace(oc)
		g.By("Deleting cluster object in namespace other than openshift-cluster-api, should be successful")
		err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("cluster", clusterNotInCapi.name, "-n", clusterNotInCapi.namespace).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: zhsun@redhat.com
	g.It("NonHyperShiftHOST-NonPreRelease-Longduration-Author:zhsun-Medium-62928-[CAPI] Enable IMDSv2 on existing worker machines via machine set [Disruptive][Slow]", func() {
		g.By("Check if cluster api on this platform is supported")
		exutil.SkipConditionally(oc)
		exutil.SkipTestIfSupportedPlatformNotMatched(oc, "aws")
		skipForCAPINotExist(oc)

		g.By("Create cluster, awscluster, awsmachinetemplate")
		cluster.createCluster(oc)
		defer awscluster.deleteAWSCluster(oc)
		awscluster.createAWSCluster(oc)
		defer awsMachineTemplate.deleteAWSMachineTemplate(oc)
		awsMachineTemplate.createAWSMachineTemplate(oc)
		err := oc.AsAdmin().WithoutNamespace().Run("patch").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-p", `{"spec":{"template":{"spec":{"instanceMetadataOptions":{"httpEndpoint":"enabled","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":"disabled"}}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check machineTemplate with httpTokens: required")
		out, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("awsmachinetemplate", capiMachineSetAWS.machineTemplateName, "-n", clusterAPINamespace, "-o=jsonpath={.items[0].spec.template.spec.httpTokens}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).Should(o.Equal("required"))

		g.By("Create capi machineset with IMDSv2")
		capiMachineSetAWS.name = "capi-machineset-62928"
		defer waitForCapiMachinesDisapper(oc, capiMachineSetAWS.name)
		defer capiMachineSetAWS.deleteCapiMachineSet(oc)
		capiMachineSetAWS.createCapiMachineSet(oc)
	})
})
