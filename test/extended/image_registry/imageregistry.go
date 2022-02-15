package image_registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	clusterinfra "github.com/openshift/openshift-tests-private/test/extended/util/clusterinfrastructure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

var _ = g.Describe("[sig-imageregistry] Image_Registry", func() {
	defer g.GinkgoRecover()
	var (
		oc                   = exutil.NewCLI("default-image-registry", exutil.KubeConfigPath())
		bcName               = "rails-postgresql-example"
		bcNameOne            = fmt.Sprintf("%s-1", bcName)
		errInfo              = "http.response.status=404"
		logInfo              = `Unsupported value: "abc": supported values: "", "Normal", "Debug", "Trace", "TraceAll"`
		updatePolicy         = `"maxSurge":0,"maxUnavailable":"10%"`
		monitoringns         = "openshift-monitoring"
		promPod              = "prometheus-k8s-0"
		patchAuthUrl         = `"authURL":"invalid"`
		patchRegion          = `"regionName":"invaild"`
		patchDomain          = `"domain":"invaild"`
		patchDomainId        = `"domainID":"invalid"`
		patchTenantId        = `"tenantID":"invalid"`
		authErrInfo          = `Get "invalid/": unsupported`
		regionErrInfo        = "No suitable endpoint could be found"
		domainErrInfo        = "Failed to authenticate provider client"
		domainIdErrInfo      = "You must provide exactly one of DomainID or DomainName"
		tenantIdErrInfo      = "Authentication failed"
		queryCredentialMode  = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1/query?query=cco_credentials_mode"
		imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
		requireRules         = "requiredDuringSchedulingIgnoredDuringExecution"
		preRules             = "preferredDuringSchedulingIgnoredDuringExecution"
	)
	// author: wewang@redhat.com
	g.It("Author:wewang-High-39027-Check AWS secret and access key with an OpenShift installed in a regular way", func() {
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform")
		}
		g.By("Check AWS secret and access key inside image registry pod")
		result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cat", "/var/run/secrets/cloud/credentials").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("aws_access_key_id"))
		o.Expect(result).To(o.ContainSubstring("aws_secret_access_key"))
		g.By("Check installer-cloud-credentials secret")
		credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/installer-cloud-credentials", "-n", "openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		fmt.Sprintf("credentials is %s", credentials)
		sDec, err := base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			fmt.Printf("Error decoding string: %s ", err.Error())
		}
		o.Expect(sDec).To(o.ContainSubstring("aws_access_key_id"))
		o.Expect(sDec).To(o.ContainSubstring("aws_secret_access_key"))
		g.By("push/pull image to registry")
		oc.SetupProject()
		err = oc.Run("import-image").Args("myimage", "--from", "busybox", "--confirm", "--reference-policy=local").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("new-app").Args("rails-postgresql-example").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), bcNameOne, nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(bcName, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-34992-Add logLevel to registry config object with invalid value", func() {
		g.By("Change spec.loglevel with invalid values")
		out, _ := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"logLevel":"abc"}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring(logInfo))
		g.By("Change spec.operatorLogLevel with invalid values")
		out, _ = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"operatorLogLevel":"abc"}}`, "--type=merge").Output()
		o.Expect(out).To(o.ContainSubstring(logInfo))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Critial-24262-Image registry operator can read/overlap gloabl proxy setting [Disruptive]", func() {
		var (
			buildFile = filepath.Join(imageRegistryBaseDir, "inputimage.yaml")
			buildsrc  = bcSource{
				outname:   "inputimage",
				namespace: "",
				name:      "imagesourcebuildconfig",
				template:  buildFile,
			}
		)

		g.By("Check if it's a proxy cluster")
		output, _ := oc.WithoutNamespace().AsAdmin().Run("get").Args("proxy/cluster", "-o=jsonpath={.spec}").Output()
		if !strings.Contains(output, "httpProxy") {
			g.Skip("Skip for non-proxy platform")
		}
		g.By("Start a build and pull image from internal registry")
		oc.SetupProject()
		buildsrc.namespace = oc.Namespace()
		g.By("Create buildconfig")
		buildsrc.create(oc)
		g.By("starting a build to output internal imagestream")
		err := oc.Run("start-build").Args(buildsrc.outname).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), fmt.Sprintf("%s-1", buildsrc.outname), nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildsrc.outname, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("starting a build using internal registry image")
		err = oc.Run("start-build").Args(buildsrc.name).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), buildsrc.name+"-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(buildsrc.name, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Set wrong proxy to imageregistry cluster")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"proxy":{"http": "http://test:3128","https":"http://test:3128","noProxy":"test.no-proxy.com"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Remove proxy for imageregistry cluster")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec": {"proxy": null}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
			result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "env").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(result).NotTo(o.ContainSubstring("HTTP_PROXY=http://test:3128"))
			o.Expect(result).NotTo(o.ContainSubstring("HTTPS_PROXY=http://test:3128"))
		}()
		recoverRegistryDefaultPods(oc)
		result, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "env").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("HTTP_PROXY=http://test:3128"))
		o.Expect(result).To(o.ContainSubstring("HTTPS_PROXY=http://test:3128"))
		g.By("starting  a build again and waiting for failure")
		br, err := exutil.StartBuildAndWait(oc, buildsrc.name)
		o.Expect(err).NotTo(o.HaveOccurred())
		br.AssertFailure()
		g.By("expecting the build logs to indicate the image was rejected")
		buildLog, err := br.LogsNoTimestamp()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(buildLog).To(o.MatchRegexp("[Ee]rror.*initializing source docker://image-registry.openshift-image-registry.svc:5000"))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Critial-22893-PodAntiAffinity should work for image registry pod", func() {
		g.Skip("According devel comments: https://bugzilla.redhat.com/show_bug.cgi?id=2014940, still not work,when find a solution, will enable it")
		g.By("Check platforms")
		platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		platforms := map[string]bool{
			"AWS":       true,
			"Azure":     true,
			"GCP":       true,
			"OpenStack": true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}

		var numi, numj int
		g.By("Add podAntiAffinity in image registry config")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":{"podAntiAffinity":{"preferredDuringSchedulingIgnoredDuringExecution":[{"podAffinityTerm":{"topologyKey":"kubernetes.io/hostname"},"weight":100}]}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"affinity":null}}`, "--type=merge").Execute()

		g.By("Set image registry replica to 3")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":3}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Set image registry replica to 2")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
		}()

		g.By("Confirm 3 pods scaled up")
		err = wait.Poll(1*time.Minute, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if len(podList.Items) != 3 {
				e2e.Logf("Continue to next round")
				return false, nil
			} else {
				for _, pod := range podList.Items {
					if pod.Status.Phase != corev1.PodRunning {
						e2e.Logf("Continue to next round")
						return false, nil
					}
				}
				return true, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Image registry pod list is not 3")

		g.By("At least 2 pods in different nodes")
		_, numj = comparePodHostIp(oc)
		o.Expect(numj >= 2).To(o.BeTrue())

		g.By("Set image registry replica to 4")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":4}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Set image registry replica to 2")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":2}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
		}()

		g.By("Confirm 4 pods scaled up")
		err = wait.Poll(50*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if len(podList.Items) != 4 {
				e2e.Logf("Continue to next round")
				return false, nil
			} else {
				for _, pod := range podList.Items {
					if pod.Status.Phase != corev1.PodRunning {
						e2e.Logf("Continue to next round")
						return false, nil
					}
				}
				return true, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Image registry pod list is not 4")

		g.By("Check 2 pods in the same node")
		numi, _ = comparePodHostIp(oc)
		o.Expect(numi >= 1).To(o.BeTrue())
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Low-43669-Update openshift-image-registry/node-ca DaemonSet using maxUnavailable", func() {
		g.By("Check node-ca updatepolicy")
		out := getResource(oc, asAdmin, withoutNamespace, "daemonset/node-ca", "-n", "openshift-image-registry", "-o=jsonpath={.spec.updateStrategy.rollingUpdate}")
		o.Expect(out).To(o.ContainSubstring(updatePolicy))
	})

	// author: xiuwang@redhat.com
	g.It("DisconnectedOnly-Author:xiuwang-High-43715-Image registry pullthough should support pull image from the mirror registry with auth via imagecontentsourcepolicy", func() {
		g.By("Check the imagestream imported with digest id using pullthrough policy")
		out := getResource(oc, asAdmin, withoutNamespace, "is/jenkins", "-n", "openshift", "-o=jsonpath={.spec.tags[0]['from.name', 'referencePolicy.type']}")
		o.Expect(out).To(o.ContainSubstring("Local"))
		o.Expect(out).To(o.ContainSubstring("@sha256"))

		g.By("Create a pod using the imagestream")
		oc.SetupProject()
		err := oc.Run("new-app").Args("jenkins-ephemeral").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		err = wait.Poll(25*time.Second, 1*time.Minute, func() (bool, error) {
			podList, err := oc.KubeClient().CoreV1().Pods(oc.Namespace()).List(metav1.ListOptions{LabelSelector: "deploymentconfig=jenkins"})
			o.Expect(err).NotTo(o.HaveOccurred())
			if len(podList.Items) == 1 {
				return true, nil
			}
			return false, nil

		})
		exutil.AssertWaitPollNoErr(err, "Pulling image via icsp is failed")
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-27961-Create imagestreamtag with insufficient permissions [Disruptive]", func() {
		var (
			roleFile = filepath.Join(imageRegistryBaseDir, "role.yaml")
			rolesrc  = authRole{
				namespace: "",
				rolename:  "tag-bug-role",
				template:  roleFile,
			}
		)
		g.By("Import an image")
		oc.SetupProject()
		err := oc.Run("import-image").Args("test-img", "--from", "registry.access.redhat.com/rhel7", "--confirm").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create role with insufficient permissions")
		rolesrc.namespace = oc.Namespace()
		rolesrc.create(oc)
		err = oc.Run("create").Args("sa", "tag-bug-sa").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().Run("policy").Args("add-role-to-user", "view", "-z", "tag-bug-sa", "--role-namespace", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().Run("policy").Args("remove-role-from-user", "view", "tag-bug-sa", "--role-namespace", oc.Namespace()).Execute()
		out, _ := oc.Run("get").Args("sa", "tag-bug-sa", "-o=jsonpath={.secrets[0].name}", "-n", oc.Namespace()).Output()
		token, _ := oc.Run("get").Args("secret/"+out, "-o", `jsonpath={.data.\.dockercfg}`).Output()
		sDec, err := base64.StdEncoding.DecodeString(token)
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("config").Args("set-credentials", "tag-bug-sa", fmt.Sprintf("--token=%s", sDec)).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defuser, err := oc.Run("config").Args("get-users").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err = oc.Run("config").Args("current-context").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("config").Args("set-context", out, "--user=tag-bug-sa").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.Run("config").Args("set-context", out, "--user="+defuser).Execute()

		g.By("Create imagestreamtag with insufficient permissions")
		err = oc.AsAdmin().Run("tag").Args("test-img:latest", "test-img:v1").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check if new imagestreamtag created")
		out = getResource(oc, true, withoutNamespace, "istag", "-n", oc.Namespace())
		o.Expect(out).To(o.ContainSubstring("test-img:latest"))
		o.Expect(out).To(o.ContainSubstring("test-img:v1"))
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Medium-43664-Check ServiceMonitor of registry which will not hotloop CVO", func() {
		g.By("Check the servicemonitor of openshift-image-registry")
		out := getResource(oc, asAdmin, withoutNamespace, "servicemonitor", "-n", "openshift-image-registry", "-o=jsonpath={.items[1].spec.selector.matchLabels.name}")
		o.Expect(out).To(o.ContainSubstring("image-registry-operator"))

		g.By("Check CVO not hotloop due to registry")
		masterlogs, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("node-logs", "--role", "master", "--path=kube-apiserver/audit.log", "--raw").OutputToFile("audit.log")
		o.Expect(err).NotTo(o.HaveOccurred())

		result, err := exec.Command("bash", "-c", "cat "+masterlogs+" | grep verb.*update.*resource.*servicemonitors").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).NotTo(o.ContainSubstring("image-registry"))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-27985-Image with invalid resource name can be pruned", func() {
		g.By("Config image registry to emptydir")
		defer recoverRegistryStorageConfig(oc)
		defer recoverRegistryDefaultReplicas(oc)
		configureRegistryStorageToEmptyDir(oc)

		g.By("Import image to internal registry")
		oc.SetupProject()
		var invalidInfo = "Invalid image name foo/bar/" + oc.Namespace() + "/ruby-hello-world"
		err := oc.Run("new-build").Args("openshift/ruby:latest~https://github.com/openshift/ruby-hello-world.git").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "ruby-hello-world-1", nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs("ruby-hello-world", oc)
		}
		exutil.AssertWaitPollNoErr(err, "build is not complete")

		g.By("Add system:image-pruner role to system:serviceaccount:openshift-image-registry:registry")
		err = oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "add-cluster-role-to-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer oc.AsAdmin().WithoutNamespace().Run("adm").Args("policy", "remove-cluster-role-from-user", "system:image-pruner", "system:serviceaccount:openshift-image-registry:registry").Execute()

		g.By("Check invaild image source can be pruned")
		err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "mkdir", "-p", "/registry/docker/registry/v2/repositories/foo/bar").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cp", "-r", "/registry/docker/registry/v2/repositories/"+oc.Namespace(), "/registry/docker/registry/v2/repositories/foo/bar").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		out, err := oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "/bin/bash", "-c", "/usr/bin/dockerregistry -prune=check").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(out).To(o.ContainSubstring(invalidInfo))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-41414-There are 2 replicas for image registry on HighAvailable workers S3/Azure/GCS/Swift storage [Flaky]", func() {
		g.By("Check image registry pod")
		platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		switch platformtype {
		case "AWS", "Azure", "GCP", "OpenStack":
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			o.Expect(len(podList.Items)).To(o.Equal(2))
			oc.SetupProject()
			err := oc.Run("new-build").Args("-D", "FROM quay.io/openshifttest/busybox@sha256:afe605d272837ce1732f390966166c2afff5391208ddd57de10942748694049d", "--to=test-41414").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), "test-41414-1", nil, nil, nil)
			if err != nil {
				exutil.DumpBuildLogs("test-41414", oc)
			}
			exutil.AssertWaitPollNoErr(err, "build is not complete")
		default:
			g.Skip("Skip for other clusters!")
		}
	})

	//author: xiuwang@redhat.com
	g.It("Author:xiuwang-Critial-34680-Image registry storage cannot be removed if set to Unamanaged when image registry is set to Removed [Disruptive]", func() {
		g.By("Get registry storage info")
		var storageinfo1, storageinfo2, storageinfo3 string
		storageinfo1 = restoreRegistryStorageConfig(oc)
		g.By("Set image registry storage to Unmanaged, image registry operator to Removed")
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed","storage":{"managementState":"Unmanaged"}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defer func() {
			g.By("Recover image registry change")
			err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed","storage":{"managementState":"Managed"}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
		}()
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			podList, err1 := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if err1 != nil {
				e2e.Logf("Error listing pods: %v", err)
				return false, nil
			}
			if len(podList.Items) != 0 {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is not removed")
		storageinfo2 = restoreRegistryStorageConfig(oc)
		if strings.Compare(storageinfo1, storageinfo2) != 0 {
			e2e.Failf("Image stroage has changed")
		}
		g.By("Set image registry operator to Managed again")
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			podList, err1 := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if err1 != nil {
				e2e.Logf("Error listing pods: %v", err)
				return false, nil
			}
			if len(podList.Items) == 0 {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is not recovered")
		storageinfo3 = restoreRegistryStorageConfig(oc)
		if strings.Compare(storageinfo1, storageinfo3) != 0 {
			e2e.Failf("Image stroage has changed")
		}
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Critical-21593-Check registry status by changing managementState for image-registry [Disruptive]", func() {
		g.By("Check platforms")
		platformtype, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.spec.platformSpec.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		platforms := map[string]bool{
			"AWS":       true,
			"Azure":     true,
			"GCP":       true,
			"OpenStack": true,
		}
		if !platforms[platformtype] {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Change managementSet from Managed -> Removed")
		defer func() {
			g.By("Set image registry cluster Managed")
			oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
			recoverRegistryDefaultPods(oc)
		}()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check image-registry pods are removed")
		checkRegistrypodsRemoved(oc)

		g.By("Change managementSet from Removed to Managed")
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Managed"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		recoverRegistryDefaultPods(oc)

		g.By("Change managementSet from Managed to Unmanaged")
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Unmanaged"}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Update replicas to 1")
		defer oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas": 2}}`, "--type=merge").Execute()
		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas": 1}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check image registry pods are still 2")
		podList, err := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(podList.Items)).Should(o.Equal(2))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-High-45952-Imported imagestreams should success in deploymentconfig", func() {
		var (
			statefulsetFile = filepath.Join(imageRegistryBaseDir, "statefulset.yaml")
			statefulsetsrc  = staSource{
				namespace: "",
				name:      "example-statefulset",
				template:  statefulsetFile,
			}
		)
		g.By("Import an image stream and set image-lookup")
		oc.SetupProject()
		err := oc.Run("import-image").Args("registry.access.redhat.com/ubi8/ubi", "--scheduled", "--confirm", "--reference-policy=local").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "ubi", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.Run("set").Args("image-lookup", "ubi").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create the initial statefulset")
		statefulsetsrc.namespace = oc.Namespace()
		g.By("Create statefulset")
		statefulsetsrc.create(oc)
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)

		g.By("Reapply the sample yaml")
		applyResourceFromTemplate(oc, "--ignore-unknown-parameters=true", "-f", statefulsetsrc.template, "-p", "NAME="+statefulsetsrc.name, "NAMESPACE="+statefulsetsrc.namespace)
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)

		g.By("setting a trigger, pods are still running")
		err = oc.Run("set").Args("triggers", "statefulset/example-statefulset", "--from-image=ubi:latest", "--containers", "example-statefulset").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Check the pods are running")
		checkPodsRunningWithLabel(oc, oc.Namespace(), "app=example-statefulset", 3)
		interReg := "image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/ubi"
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("pods", "-o=jsonpath={.items[*].spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(interReg))
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-39028-Check aws secret and access key with an openShift installed with an STS credential", func() {
		g.By("Check platforms")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform")
		}
		g.By("Check if the cluster is with STS credential")
		token, err := oc.AsAdmin().WithoutNamespace().Run("sa").Args("-n", "openshift-monitoring", "get-token", "prometheus-k8s").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		result, err := getBearerTokenURLViaPod(monitoringns, promPod, queryCredentialMode, token)
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(result, "manualpodidentity") {
			g.Skip("Skip for the aws cluster without STS credential")
		}

		g.By("Check role_arn/web_identity_token_file inside image registry pod")
		result, err = oc.AsAdmin().WithoutNamespace().Run("rsh").Args("-n", "openshift-image-registry", "deployment.apps/image-registry", "cat", "/var/run/secrets/cloud/credentials").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(result).To(o.ContainSubstring("role_arn"))
		o.Expect(result).To(o.ContainSubstring("web_identity_token_file"))

		g.By("Check installer-cloud-credentials secret")
		credentials, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("secret/installer-cloud-credentials", "-n", "openshift-image-registry", "-o=jsonpath={.data.credentials}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		sDec, _ := base64.StdEncoding.DecodeString(credentials)
		if !strings.Contains(string(sDec), "role_arn") {
			e2e.Failf("credentials does not contain role_arn")
		}
		if !strings.Contains(string(sDec), "web_identity_token_file") {
			e2e.Failf("credentials does not contain web_identity_token_file")
		}

		g.By("push/pull image to registry")
		oc.SetupProject()
		ns_39028 := oc.Namespace()
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("myimage", "-n", ns_39028, "--from", "busybox", "--confirm", "--reference-policy=local").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.WithoutNamespace().AsAdmin().Run("new-app").Args("rails-postgresql-example", "-n", ns_39028).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("waiting for build to finish")
		err = exutil.WaitForABuild(oc.BuildClient().BuildV1().Builds(oc.Namespace()), bcNameOne, nil, nil, nil)
		if err != nil {
			exutil.DumpBuildLogs(bcName, oc)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	//author: xiuwang@redhat.com
	g.It("NonPreRelease-Author:xiuwang-High-45540-Registry should fall back to secondary ImageContentSourcePolicy Mirror [Disruptive]", func() {
		var (
			icspFile = filepath.Join(imageRegistryBaseDir, "icsp-multi-mirrors.yaml")
			icspsrc  = icspSource{
				name:     "image-policy-fake",
				template: icspFile,
			}
		)
		g.By("Create imagecontentsourcepolicy with multiple mirrors")
		defer icspsrc.delete(oc)
		icspsrc.create(oc)

		g.By("Get all nodes list")
		nodeList, err := exutil.GetAllNodes(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check registry configs in all nodes")
		err = wait.Poll(25*time.Second, 2*time.Minute, func() (bool, error) {
			for _, nodeName := range nodeList {
				output, err := exutil.DebugNodeWithChroot(oc, nodeName, "bash", "-c", "cat /etc/containers/registries.conf | grep fake.rhcloud.com")
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(output, "fake.rhcloud.com") {
					e2e.Logf("Continue to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "registry configs are not changed")

		g.By("Create a pod to check pulling issue")
		oc.SetupProject()
		err = exutil.WaitForAnImageStreamTag(oc, "openshift", "cli", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("deployment", "cli-test", "--image", "image-registry.openshift-image-registry.svc:5000/openshift/cli:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get project events")
		err = wait.Poll(10*time.Second, 1*time.Minute, func() (bool, error) {
			events, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("events", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(events, `Successfully pulled image "image-registry.openshift-image-registry.svc:5000/openshift/cli:latest"`) {
				e2e.Logf("Continue to next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image pulls failed")
	})

	// author: wewang@redhat.com
	g.It("Author:wewang-Medium-23583-Registry should not try to pullthrough himself by any name [Serial]", func() {
		g.By("Create additional routes by populating spec.Routes with additional routes")
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":false}}`, "--type=merge").Execute()
		err := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":true}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		defroute := getRegistryDefaultRoute(oc)
		userroute := strings.Replace(defroute, "default", "extra", 1)
		patchInfo := fmt.Sprintf("{\"spec\":{\"routes\":[{\"hostname\": \"%s\", \"name\":\"extra-image-registry\", \"secretName\":\"\"}]}}", userroute)
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"routes":null}}`, "--type=merge").Execute()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", patchInfo, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Get token from secret")
		oc.SetupProject()
		token, err := oc.WithoutNamespace().AsAdmin().Run("serviceaccounts").Args("get-token", "builder", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a secret for user-defined route")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "mysecret", "--docker-server="+userroute, "--docker-username="+oc.Username(), "--docker-password="+token, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Import an image")
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("myimage", "--from=quay.io/openshifttest/busybox@sha256:afe605d272837ce1732f390966166c2afff5391208ddd57de10942748694049d", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Tag the image point to itself address")
		err = oc.WithoutNamespace().AsAdmin().Run("tag").Args(userroute+"/"+oc.Namespace()+"/myimage", "myimage:test", "--insecure=true", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "myimage", "test")
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Check import successfully")
		err = wait.Poll(5*time.Second, 20*time.Second, func() (bool, error) {
			successInfo := userroute + "/" + oc.Namespace() + "/myimage@sha256"
			output, err := oc.WithoutNamespace().AsAdmin().Run("describe").Args("is", "myimage", "-n", oc.Namespace()).Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if o.Expect(output).To(o.ContainSubstring(successInfo)) {
				return true, nil
			} else {
				e2e.Logf("Continue to next round")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Import failed"))
		g.By("Get blobs from the default registry")
		getUrl := "curl -Lks -u \"" + oc.Username() + ":" + token + "\" -I HEAD https://" + defroute + "/v2/" + oc.Namespace() + "/myimage@sha256:0000000000000000000000000000000000000000000000000000000000000000"
		curlOutput, err := exec.Command("bash", "-c", getUrl).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(curlOutput)).To(o.ContainSubstring("404 Not Found"))
		podsOfImageRegistry := []corev1.Pod{}
		podsOfImageRegistry = ListPodStartingWith("image-registry", oc, "openshift-image-registry")
		if len(podsOfImageRegistry) == 0 {
			e2e.Failf("Error retrieving logs")
		}
		foundErrLog := false
		foundErrLog = DePodLogs(podsOfImageRegistry, oc, errInfo)
		o.Expect(foundErrLog).To(o.BeTrue())
	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Longduration-Author:jitli-ConnectedOnly-VMonly-Medium-33051-Images can be imported from an insecure registry without 'insecure: true' if it is in insecureRegistries in image.config/cluster [Disruptive]", func() {

		g.By("import image from an insecure registry directly without --insecure=true")
		output, err := oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image-33051", "--from=registry.access.redhat.com/rhel7").Output()
		o.Expect(err).To(o.HaveOccurred())
		if err != nil {
			e2e.Logf(output)
		}

		g.By("Create route")
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":false}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"defaultRoute":true}}`, "--type=merge").Output()
		if err != nil {
			e2e.Logf(output)
		}
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		g.By("Get server host")
		host, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("route", "-n", "openshift-image-registry", "default-route", "-o=jsonpath={.spec.host}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf(host)

		g.By("Get token from secret")
		oc.SetupProject()
		token, err := oc.WithoutNamespace().AsAdmin().Run("serviceaccounts").Args("get-token", "builder", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Create a secret for user-defined route")
		err = oc.WithoutNamespace().AsAdmin().Run("create").Args("secret", "docker-registry", "secret33051", "--docker-server="+host, "--docker-username="+oc.Username(), "--docker-password="+token, "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Add the insecure registry to images.config.openshift.io cluster")
		defer oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"registrySources": null}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"registrySources": {"insecureRegistries": ["`+host+`"]}}}`, "--type=merge").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		g.By("registries.conf gets updated")
		workNode, _ := exutil.GetFirstWorkerNode(oc)
		e2e.Logf(workNode)
		err = wait.Poll(30*time.Second, 6*time.Minute, func() (bool, error) {
			registriesstatus, _ := exutil.DebugNodeWithChroot(oc, workNode, "bash", "-c", "cat /etc/containers/registries.conf | grep default-route-openshift-image-registry.apps")
			if strings.Contains(registriesstatus, "default-route-openshift-image-registry.apps") {
				e2e.Logf("registries.conf updated")
				return true, nil
			} else {
				e2e.Logf("registries.conf not update")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "registries.conf not update")

		g.By("Tag the image")
		output, err = oc.WithoutNamespace().AsAdmin().Run("tag").Args(host+"/openshift/ruby:latest", "ruby:33051", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Tag ruby:33051 set"))

		g.By("Add docker.io to blockedRegistries list")
		defer oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"additionalTrustedCA": null,"registrySources": null}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("images.config.openshift.io/cluster", "-p", `{"spec": {"additionalTrustedCA": {"name": ""},"registrySources": {"blockedRegistries": ["docker.io"]}}}`, "--type=merge").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		g.By("registries.conf gets updated")
		err = wait.Poll(30*time.Second, 6*time.Minute, func() (bool, error) {
			registriesstatus, _ := exutil.DebugNodeWithChroot(oc, workNode, "bash", "-c", "cat /etc/containers/registries.conf | grep \"docker.io\"")
			if strings.Contains(registriesstatus, "location = \"docker.io\"") {
				e2e.Logf("registries.conf updated")
				return true, nil
			} else {
				e2e.Logf("registries.conf not update")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "registries.conf not contains docker.io")

		g.By("Import an image from docker.io")
		output, err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image2-33051", "--from=docker.io/centos/ruby-22-centos7", "--confirm=true").Output()
		e2e.Logf(output)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("error: Import failed (Forbidden): forbidden: registry docker.io blocked"))

	})

	// author: wewang@redhat.com
	g.It("NonPreRelease-Author:wewang-Critical-24838-Registry OpenStack Storage test with invalid settings [Disruptive]", func() {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "OpenStack") {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Set a different container in registry config")
		oricontainer, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("configs.imageregistry/cluster", "-o=jsonpath={.spec.storage.swift.container}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		newcontainer := strings.Replace(oricontainer, "image", "images", 1)
		defer func() {
			err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{"container": "`+oricontainer+`"}}}}`, "--type=merge").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			recoverRegistryDefaultPods(oc)
		}()
		err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"swift":{"container": "`+newcontainer+`"}}}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		recoverRegistryDefaultPods(oc)

		g.By("Set invalid authURL in image registry crd")
		foundErrLog := false
		foundErrLog = setImageregistryConfigs(oc, patchAuthUrl, authErrInfo)
		o.Expect(foundErrLog).To(o.BeTrue())

		g.By("Set invalid regionName")
		foundErrLog = false
		foundErrLog = setImageregistryConfigs(oc, patchRegion, regionErrInfo)
		o.Expect(foundErrLog).To(o.BeTrue())

		g.By("Set invalid domain")
		foundErrLog = false
		foundErrLog = setImageregistryConfigs(oc, patchDomain, domainErrInfo)
		o.Expect(foundErrLog).To(o.BeTrue())

		g.By("Set invalid domainID")
		foundErrLog = false
		foundErrLog = setImageregistryConfigs(oc, patchDomainId, domainIdErrInfo)
		o.Expect(foundErrLog).To(o.BeTrue())

		g.By("Set invalid tenantID")
		foundErrLog = false
		foundErrLog = setImageregistryConfigs(oc, patchTenantId, tenantIdErrInfo)
		o.Expect(foundErrLog).To(o.BeTrue())
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Critical-47274-Image registry works with OSS storage on alibaba cloud", func() {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "AlibabaCloud") {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Check OSS storage")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.oss}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bucket"))
		o.Expect(output).To(o.ContainSubstring(`"endpointAccessibility":"Internal"`))
		o.Expect(output).To(o.ContainSubstring("region"))
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.conditions[?(@.type==\"StorageEncrypted\")].message}{.status.conditions[?(@.type==\"StorageEncrypted\")].status}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("Default AES256 encryption was successfully enabled on the OSS bucketTrue"))

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Check if registry works well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-47274", oc.Namespace())

		g.By("Check if registry interact with OSS used the internal endpoint")
		output, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=30s", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("internal.aliyuncs.com"))

	})

	// author: xiuwang@redhat.com
	g.It("NonPreRelease-Author:xiuwang-Medium-47342-Configure image registry works with OSS parameters [Disruptive]", func() {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "AlibabaCloud") {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Configure OSS with Public endpoint")
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"endpointAccessibility":null}}}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"endpointAccessibility":"Public"}}}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is degraded")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-47342", oc.Namespace())
		output, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=1m", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).NotTo(o.ContainSubstring("internal.aliyuncs.com"))

		g.By("Configure registry to use KMS encryption type")
		defer oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"encryption":null}}}}`, "--type=merge").Execute()
		output, err = oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"storage":{"oss":{"encryption":{"method":"KMS","kms":{"keyID":"invalidid"}}}}}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = wait.PollImmediate(10*time.Second, 2*time.Minute, func() (bool, error) {
			output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image", "cluster", "-o=jsonpath={.status.conditions[?(@.type==\"StorageEncrypted\")].message}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if !strings.Contains(output, "Default KMS encryption was successfully enabled on the OSS bucket") {
				e2e.Logf("wait for next round")
				return false, nil
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Default encryption can't be changed")
		br, err := exutil.StartBuildAndWait(oc, "test-47342")
		o.Expect(err).NotTo(o.HaveOccurred())
		br.AssertFailure()
		output, err = oc.WithoutNamespace().AsAdmin().Run("logs").Args("deploy/image-registry", "--since=1m", "-n", "openshift-image-registry").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("The specified parameter KMS keyId is not valid"))
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Critical-45345-Image registry works with ibmcos storage on IBM cloud", func() {
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "IBMCloud") {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Check ibmcos storage")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.ibmcos}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bucket"))
		o.Expect(output).To(o.ContainSubstring("location"))
		o.Expect(output).To(o.ContainSubstring("resourceGroupName"))
		o.Expect(output).To(o.ContainSubstring("resourceKeyCRN"))
		o.Expect(output).To(o.ContainSubstring("serviceInstanceCRN"))

		g.By("Check if registry operator degraded")
		registryDegrade := checkRegistryDegraded(oc)
		if registryDegrade {
			e2e.Failf("Image registry is degraded")
		}

		g.By("Check if registry works well")
		oc.SetupProject()
		checkRegistryFunctionFine(oc, "test-45345", oc.Namespace())
	})

	// author: jitli@redhat.com
	g.It("Author:jitli-ConnectedOnly-Medium-41398-Users providing custom AWS tags are set with bucket creation [Disruptive]", func() {

		g.By("Check platforms")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform")
		}
		g.By("Check the cluster is with resourceTags")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.aws}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "resourceTags") {
			g.Skip("Skip for no resourceTags")
		}
		g.By("Get bucket name")
		bucket, err := oc.AsAdmin().Run("get").Args("config.image", "-o=jsonpath={..spec.storage.s3.bucket}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(bucket).NotTo(o.BeEmpty())

		g.By("Check the tags")
		aws := getAWSClient(oc)
		tag, err := awsGetBucketTagging(aws, bucket)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(tag)).To(o.ContainSubstring("customTag"))
		o.Expect(string(tag)).To(o.ContainSubstring("installer-qe"))

		g.By("Removed managementState")
		defer func() {
			status, err := oc.AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.spec.managementState}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if status != "Managed" {
				e2e.Logf("recover config.image cluster is Managed")
				output, err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Managed"}}`, "--type=merge").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(string(output)).To(o.ContainSubstring("patched"))
			} else {
				e2e.Logf("config.image cluster is Managed")
			}
		}()
		output, err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Removed"}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		g.By("Check bucket has been deleted")
		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			tag, err = awsGetBucketTagging(aws, bucket)
			if err != nil && strings.Contains(tag, "The specified bucket does not exist") {
				return true, nil
			} else {
				e2e.Logf("bucket still exist, go next round")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "the bucket isn't been deleted")

		g.By("Managed managementState")
		output, err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"managementState": "Managed"}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("patched"))

		g.By("Get new bucket name and check")
		err = wait.Poll(10*time.Second, 2*time.Minute, func() (bool, error) {
			bucket, _ = oc.AsAdmin().Run("get").Args("config.image", "-o=jsonpath={..spec.storage.s3.bucket}").Output()
			if strings.Compare(bucket, "") != 0 {
				return true, nil
			} else {
				e2e.Logf("not update")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Can't get bucket")

		tag, err = awsGetBucketTagging(aws, bucket)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(tag)).To(o.ContainSubstring("customTag"))
		o.Expect(string(tag)).To(o.ContainSubstring("installer-qe"))
	})

	// author: tbuskey@redhat.com
	g.It("Author:tbuskey-High-22056-Registry operator configure prometheus metric gathering", func() {
		var (
			authHeader         string
			after              = make(map[string]int)
			before             = make(map[string]int)
			data               PrometheusImageregistryQueryHttp
			err                error
			fails              = 0
			failItems          = ""
			l                  int
			msg                string
			prometheusUrl      = "https://prometheus-k8s.openshift-monitoring.svc:9091/api/v1"
			prometheusUrlQuery string
			query              string
			token              string
			metrics            = []string{"imageregistry_http_request_duration_seconds_count",
				"imageregistry_http_request_size_bytes_count",
				"imageregistry_http_request_size_bytes_sum",
				"imageregistry_http_response_size_bytes_count",
				"imageregistry_http_response_size_bytes_sum",
				"imageregistry_http_request_size_bytes_count",
				"imageregistry_http_request_size_bytes_sum",
				"imageregistry_http_requests_total",
				"imageregistry_http_response_size_bytes_count",
				"imageregistry_http_response_size_bytes_sum"}
		)

		g.By("Get Prometheus token")
		token, err = oc.AsAdmin().WithoutNamespace().Run("sa").Args("-n", "openshift-monitoring", "get-token", "prometheus-k8s").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(token).NotTo(o.BeEmpty())
		authHeader = fmt.Sprintf("Authorization: Bearer %v", token)

		g.By("Collect metrics at start")
		for _, query = range metrics {
			prometheusUrlQuery = fmt.Sprintf("%v/query?query=%v", prometheusUrl, query)
			msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--", "curl", "-k", "-H", authHeader, prometheusUrlQuery).Outputs()
			o.Expect(msg).NotTo(o.BeEmpty())
			json.Unmarshal([]byte(msg), &data)
			l = len(data.Data.Result) - 1
			before[query], err = strconv.Atoi(data.Data.Result[l].Value[1].(string))
			// e2e.Logf("query %v ==  %v", query, before[query])
		}
		g.By("pause to get next metrics")
		time.Sleep(60 * time.Second)

		g.By("Collect metrics again")
		for _, query = range metrics {
			prometheusUrlQuery = fmt.Sprintf("%v/query?query=%v", prometheusUrl, query)
			msg, _, err = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-monitoring", "-c", "prometheus", "prometheus-k8s-0", "-i", "--", "curl", "-k", "-H", authHeader, prometheusUrlQuery).Outputs()
			o.Expect(msg).NotTo(o.BeEmpty())
			json.Unmarshal([]byte(msg), &data)
			l = len(data.Data.Result) - 1
			after[query], err = strconv.Atoi(data.Data.Result[l].Value[1].(string))
			// e2e.Logf("query %v ==  %v", query, before[query])
		}

		g.By("results")
		for _, query = range metrics {
			msg = "."
			if before[query] > after[query] {
				fails++
				failItems = fmt.Sprintf("%v%v ", failItems, query)
			}
			e2e.Logf("%v -> %v %v", before[query], after[query], query)
			// need to test & compare
		}
		if fails != 0 {
			e2e.Failf("\nFAIL: %v metrics decreasesd: %v\n\n", fails, failItems)
		}

		g.By("Success")
	})

	// author: xiuwang@redhat.com
	g.It("Author:xiuwang-Medium-47933-DeploymentConfigs template should respect resolve-names annotation", func() {
		var (
			imageRegistryBaseDir = exutil.FixturePath("testdata", "image_registry")
			podFile              = filepath.Join(imageRegistryBaseDir, "dc-template.yaml")
			podsrc               = podSource{
				name:      "mydc",
				namespace: "",
				image:     "myis",
				template:  podFile,
			}
		)

		g.By("Use source imagestream to create dc")
		oc.SetupProject()
		err := oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "myis:latest", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), podsrc.image, "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		podsrc.namespace = oc.Namespace()
		podsrc.create(oc)
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("deploymentconfig/mydc", "-o=jsonpath={..spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("quay.io/openshifttest/busybox"))

		g.By("Use pullthrough imagestream to create dc")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", "myis:latest", "--reference-policy=local", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), podsrc.image, "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
		podsrc.create(oc)
		output, err = oc.AsAdmin().WithoutNamespace().Run("get").Args("deploymentconfig/mydc", "-o=jsonpath={..spec.template.spec.containers[*].image}", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("image-registry.openshift-image-registry.svc:5000/" + oc.Namespace() + "/" + podsrc.image))
	})

	g.It("NonPreRelease-Author:xiuwang-VMonly-Critical-43260-Image registry pod could report to processing after openshift-apiserver reports unconnect quickly[Disruptive][Slow]", func() {
		firstMaster, err := exutil.GetFirstMasterNode(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		clusterID, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("infrastructure", "cluster", "-o=jsonpath={.status.infrastructureName}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		if clusterinfra.CheckPlatform(oc) == "none" && strings.HasPrefix(firstMaster, "master") && !strings.HasPrefix(firstMaster, clusterID) && !strings.HasPrefix(firstMaster, "internal") {
			defer oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"tolerations":[]}}`, "--type=merge").Output()
			output, err := oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"tolerations":[{"effect":"NoSchedule","key":"node-role.kubernetes.io/master","operator":"Exists"}]}}`, "--type=merge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			e2e.Logf(output)
			newCheck("expect", asAdmin, withoutNamespace, contain, "Running", ok, []string{"pods", "-n", "openshift-image-registry", "-l", "docker-registry=default"}).check(oc)
			names, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", "openshift-image-registry", "-l", "docker-registry=default", "-o", "name").Output()
			if err != nil {
				e2e.Failf("Fail to get the image-registry pods' name")
			}
			podNames := strings.Split(names, "\n")
			privateKeyPath := "/root/openshift-qe.pem"
			var nodeNames []string

			for _, podName := range podNames {
				e2e.Logf("get the node name of pod name: %s", podName)
				nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", "openshift-image-registry", podName, "-o=jsonpath={.spec.nodeName}").Output()
				e2e.Logf("node name: %s", nodeName)
				if err != nil {
					e2e.Failf("Fail to get the node name")
				}
				nodeNames = append(nodeNames, nodeName)
			}

			for _, nodeName := range nodeNames {

				e2e.Logf("stop crio service of node: %s", nodeName)
				defer exec.Command("bash", "-c", "ssh -o StrictHostKeyChecking=no -i "+privateKeyPath+" core@"+nodeName+" sudo systemctl start crio").CombinedOutput()
				defer exec.Command("bash", "-c", "ssh -o StrictHostKeyChecking=no -i "+privateKeyPath+" core@"+nodeName+" sudo systemctl start kubelet").CombinedOutput()
				output, _ := exec.Command("bash", "-c", "ssh -o StrictHostKeyChecking=no -i "+privateKeyPath+" core@"+nodeName+" sudo systemctl stop crio").CombinedOutput()
				e2e.Logf("stop crio command result : %s", output)
				o.Expect(err).NotTo(o.HaveOccurred())
				e2e.Logf("stop service of node: %s", nodeName)
				output, _ = exec.Command("bash", "-c", "ssh -o StrictHostKeyChecking=no -i "+privateKeyPath+" core@"+nodeName+" sudo systemctl stop kubelet").CombinedOutput()
				e2e.Logf("stop kubelet command result : %s", output)
				o.Expect(err).NotTo(o.HaveOccurred())
				newCheck("expect", asAdmin, withoutNamespace, contain, "NodeStatusUnknown", ok, []string{"node", nodeName, "-o=jsonpath={.status.conditions..reason}"}).check(oc)
			}
			newCheck("expect", asAdmin, withoutNamespace, contain, "True", ok, []string{"co", "image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Progressing\")].status}"}).check(oc)
			err = wait.Poll(10*time.Second, 330*time.Second, func() (bool, error) {
				res, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", "image-registry", "-o=jsonpath={.status.conditions[?(@.type==\"Available\")].status}").Output()
				if strings.Contains(res, "True") {
					return true, nil
				}
				e2e.Logf(" Available command result : %s", res)
				return false, nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		}
		e2e.Logf("Only baremetal platform supported for the test")
	})

	g.It("NonPreRelease-VMonly-Author:xiuwang-Medium-48045-Update global pull secret for additional private registries[Disruptive]", func() {
		g.By("Setup a private registry")
		oc.SetupProject()
		var regUser, regPass = "testuser", getRandomString()
		tempDataDir, err := extractPullSecret(oc)
		defer os.RemoveAll(tempDataDir)
		o.Expect(err).NotTo(o.HaveOccurred())
		originAuth := filepath.Join(tempDataDir, ".dockerconfigjson")
		htpasswdFile, err := generateHtpasswdFile(tempDataDir, regUser, regPass)
		defer os.RemoveAll(htpasswdFile)
		o.Expect(err).NotTo(o.HaveOccurred())
		regRoute := setSecureRegistryEnableAuth(oc, oc.Namespace(), "myregistry", htpasswdFile)

		g.By("Push image to private registry")
		newAuthFile, err := appendPullSecretAuth(originAuth, regRoute, regUser, regPass)
		o.Expect(err).NotTo(o.HaveOccurred())
		myimage := regRoute + "/" + oc.Namespace() + "/myimage:latest"
		err = oc.AsAdmin().WithoutNamespace().Run("image").Args("mirror", "quay.io/openshifttest/busybox@sha256:c5439d7db88ab5423999530349d327b04279ad3161d7596d2126dfb5b02bfd1f", myimage, "--insecure", "-a", newAuthFile, "--keep-manifest-list=true", "--filter-by-os=.*").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Make sure the image can't be pulled without auth")
		output, err := oc.AsAdmin().WithoutNamespace().Run("import-image").Args("firstis:latest", "--from="+myimage, "--reference-policy=local", "--insecure", "--confirm", "-n", oc.Namespace()).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(string(output)).To(o.ContainSubstring("Unauthorized"))

		g.By("Update pull secret")
		updatePullSecret(oc, newAuthFile)
		defer updatePullSecret(oc, originAuth)
		err = wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-apiserver").List(metav1.ListOptions{LabelSelector: "apiserver=true"})
			for _, pod := range podList.Items {
				output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-apiserver", pod.Name, "--", "bash", "-c", "cat /var/lib/kubelet/config.json").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				if !strings.Contains(output, oc.Namespace()) {
					e2e.Logf("Go to next round")
					return false, nil
				}
			}
			return true, nil
		})
		exutil.AssertWaitPollNoErr(err, "Failed to update apiserver")

		g.By("Make sure the image can be pulled after add auth")
		err = oc.AsAdmin().WithoutNamespace().Run("tag").Args(myimage, "newis:latest", "--reference-policy=local", "--insecure", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		err = exutil.WaitForAnImageStreamTag(oc, oc.Namespace(), "newis", "latest")
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	// author: wewang@redhat.com
	g.It("NonPreRelease-ConnectedOnly-Author:wewang-Medium-43731-Image registry pods should have anti-affinity rules [Disruptive]", func() {
		g.By("Check pods anti-affinity match requiredDuringSchedulingIgnoredDuringExecution rule when replicas is 2")
		foundrequiredRules := false
		foundrequiredRules = foundAffinityRules(oc, requireRules)
		o.Expect(foundrequiredRules).To(o.BeTrue())

		g.By("Set image registry replica to 3")
		defer recoverRegistryDefaultReplicas(oc)
		err := oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"replicas":3}}`, "--type=merge").Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		g.By("Confirm 3 pods scaled up")
		err = wait.Poll(30*time.Second, 2*time.Minute, func() (bool, error) {
			podList, _ := oc.AdminKubeClient().CoreV1().Pods("openshift-image-registry").List(metav1.ListOptions{LabelSelector: "docker-registry=default"})
			if len(podList.Items) != 3 {
				e2e.Logf("Continue to next round")
				return false, nil
			} else {
				for _, pod := range podList.Items {
					if pod.Status.Phase != corev1.PodRunning {
						e2e.Logf("Continue to next round")
						return false, nil
					}
				}
				return true, nil
			}
		})
		o.Expect(err).NotTo(o.HaveOccurred())
		foundrequiredRules = foundAffinityRules(oc, preRules)
		o.Expect(foundrequiredRules).To(o.BeTrue())
		/*
		   when https://bugzilla.redhat.com/show_bug.cgi?id=2000940 is fixed, will open this part
		   		g.By("Set deployment.apps replica to 0")
		   		err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("deployment.apps/image-registry", "-p", `{"spec":{"replicas":0}}`, "--type=merge", "-n", "openshift-image-registry").Execute()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("co/image-registry", "-o=jsonpath={.status.conditions[0]}").Output()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		o.Expect(output).To(o.ContainSubstring("\"status\":\"False\""))
		   		o.Expect(output).To(o.ContainSubstring("The deployment does not have available replicas"))
		   		o.Expect(output).To(o.ContainSubstring("\"type\":\"Available\""))
		   		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.imageregistry/cluster", "-o=jsonpath={.status.readyReplicas}").Output()
		   		o.Expect(err).NotTo(o.HaveOccurred())
		   		o.Expect(output).To(o.Equal("0"))
		*/
	})

	// author: jitli@redhat.com
	g.It("NonPreRelease-Author:jitli-Critical-34895-Image registry can work well on Gov Cloud with custom endpoint defined [Disruptive]", func() {

		g.By("Check platforms")
		output, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("infrastructure.config.openshift.io", "-o=jsonpath={..status.platformStatus.type}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "AWS") {
			g.Skip("Skip for non-supported platform")
		}

		g.By("Check the cluster is with us-gov")
		output, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.region}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(output, "us-gov") {
			g.Skip("Skip for wrong region")
		}

		g.By("Set regionEndpoint if it not set")
		regionEndpoint, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.regionEndpoint}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if !strings.Contains(regionEndpoint, "https://s3.us-gov-west-1.amazonaws.com") {
			defer func() {
				output, err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"s3":{"regionEndpoint": null ,"virtualHostedStyle": null}}}}`, "--type=merge").Output()
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(string(output)).To(o.ContainSubstring("patched"))
			}()
			output, err = oc.AsAdmin().Run("patch").Args("config.image/cluster", "-p", `{"spec":{"storage":{"s3":{"regionEndpoint": "https://s3.us-gov-west-1.amazonaws.com" ,"virtualHostedStyle": true}}}}`, "--type=merge").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(output).To(o.ContainSubstring("patched"))
		}

		err = wait.Poll(2*time.Second, 10*time.Second, func() (bool, error) {
			regionEndpoint, err = oc.WithoutNamespace().AsAdmin().Run("get").Args("config.image/cluster", "-o=jsonpath={.status.storage.s3.regionEndpoint}").Output()
			o.Expect(err).NotTo(o.HaveOccurred())
			if strings.Contains(regionEndpoint, "https://s3.us-gov-west-1.amazonaws.com") {
				return true, nil
			} else {
				e2e.Logf("regionEndpoint not found, go next round")
				return false, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "regionEndpoint not found")

		g.By("Check if registry operator degraded")
		err = wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
			registryDegrade := checkRegistryDegraded(oc)
			if registryDegrade {
				return false, nil
			} else {
				return true, nil
			}
		})
		exutil.AssertWaitPollNoErr(err, "Image registry is degraded")

		g.By("Import an image with reference-policy=local")
		oc.SetupProject()
		err = oc.WithoutNamespace().AsAdmin().Run("import-image").Args("image-34895", "--from=registry.access.redhat.com/rhel7", "--reference-policy=local", "--confirm", "-n", oc.Namespace()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Start a build")
		checkRegistryFunctionFine(oc, "test-34895", oc.Namespace())

	})

})
