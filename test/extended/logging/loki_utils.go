package logging

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/openshift-tests-private/test/extended/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	apiPath         = "/api/logs/v1/"
	queryPath       = "/loki/api/v1/query"
	queryRangePath  = "/loki/api/v1/query_range"
	labelsPath      = "/loki/api/v1/labels"
	labelValuesPath = "/loki/api/v1/label/%s/values"
	seriesPath      = "/loki/api/v1/series"
	tailPath        = "/loki/api/v1/tail"
	minioNS         = "minio-aosqe"
	minioSecret     = "minio-creds"
)

// s3Credential defines the s3 credentials
type s3Credential struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string //the endpoint of s3 service
}

func getAWSCredentialFromCluster(oc *exutil.CLI) s3Credential {
	region, err := exutil.GetAWSClusterRegion(oc)
	o.Expect(err).NotTo(o.HaveOccurred())

	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err = os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/aws-creds", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/aws_access_key_id")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/aws_secret_access_key")
	o.Expect(err).NotTo(o.HaveOccurred())

	cred := s3Credential{Region: region, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
	return cred
}

func getODFCreds(oc *exutil.CLI) s3Credential {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/noobaa-admin", "-n", "openshift-storage", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/AWS_ACCESS_KEY_ID")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/AWS_SECRET_ACCESS_KEY")
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://" + getRouteAddress(oc, "openshift-storage", "s3")
	return s3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

func getMinIOCreds(oc *exutil.CLI, ns string) s3Credential {
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+minioSecret, "-n", ns, "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	accessKeyID, err := os.ReadFile(dirname + "/access_key_id")
	o.Expect(err).NotTo(o.HaveOccurred())
	secretAccessKey, err := os.ReadFile(dirname + "/secret_access_key")
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://" + getRouteAddress(oc, ns, "minio")
	return s3Credential{Endpoint: endpoint, AccessKeyID: string(accessKeyID), SecretAccessKey: string(secretAccessKey)}
}

// initialize a s3 client with credential
// TODO: add an option to initialize a new client with STS
func newS3Client(oc *exutil.CLI, cred s3Credential) *s3.Client {
	var err error
	var cfg aws.Config
	if len(cred.Endpoint) > 0 {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               cred.Endpoint,
				HostnameImmutable: true,
			}, nil
		})
		// For ODF and Minio, they're deployed in OCP clusters
		// In some clusters, we can't connect it without proxy, here add proxy settings to s3 client when there has http_proxy or https_proxy in the env var
		httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
			proxy := getProxyFromEnv()
			if len(proxy) > 0 {
				proxyURL, err := url.Parse(proxy)
				o.Expect(err).NotTo(o.HaveOccurred())
				tr.Proxy = http.ProxyURL(proxyURL)
			}
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		})
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cred.AccessKeyID, cred.SecretAccessKey, "")),
			config.WithEndpointResolverWithOptions(customResolver),
			config.WithHTTPClient(httpClient))
	} else {
		// aws s3
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cred.AccessKeyID, cred.SecretAccessKey, "")),
			config.WithRegion(cred.Region))
	}
	o.Expect(err).NotTo(o.HaveOccurred())
	return s3.NewFromConfig(cfg)
}

func createS3Bucket(client *s3.Client, bucketName string, cred s3Credential) error {
	// check if the bucket exists or not
	// if exists, clear all the objects in the bucket
	// if not, create the bucket
	exist := false
	buckets, err := client.ListBuckets(context.TODO(), &s3.ListBucketsInput{})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, bu := range buckets.Buckets {
		if *bu.Name == bucketName {
			exist = true
			break
		}
	}
	// clear all the objects in the bucket
	if exist {
		return emptyS3Bucket(client, bucketName)
	}

	/*
		Per https://docs.aws.amazon.com/AmazonS3/latest/API/API_CreateBucket.html#API_CreateBucket_RequestBody,
		us-east-1 is the default region and it's not a valid value of LocationConstraint,
		using `LocationConstraint: types.BucketLocationConstraint("us-east-1")` gets error `InvalidLocationConstraint`.
		Here remove the configration when the region is us-east-1
	*/
	if len(cred.Region) == 0 || cred.Region == "us-east-1" {
		_, err = client.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &bucketName})
		return err
	}
	_, err = client.CreateBucket(context.TODO(), &s3.CreateBucketInput{Bucket: &bucketName, CreateBucketConfiguration: &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(cred.Region)}})
	return err
}

func deleteS3Bucket(client *s3.Client, bucketName string) error {
	// empty bucket
	err := emptyS3Bucket(client, bucketName)
	if err != nil {
		return err
	}
	// delete bucket
	_, err = client.DeleteBucket(context.TODO(), &s3.DeleteBucketInput{Bucket: &bucketName})
	return err
}

func emptyS3Bucket(client *s3.Client, bucketName string) error {
	// list objects in the bucket
	objects, err := client.ListObjects(context.TODO(), &s3.ListObjectsInput{Bucket: &bucketName})
	o.Expect(err).NotTo(o.HaveOccurred())
	// remove objects in the bucket
	newObjects := []types.ObjectIdentifier{}
	for _, object := range objects.Contents {
		newObjects = append(newObjects, types.ObjectIdentifier{Key: object.Key})
	}
	if len(newObjects) > 0 {
		_, err = client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{Bucket: &bucketName, Delete: &types.Delete{Quiet: true, Objects: newObjects}})
		return err
	}
	return nil
}

// createSecretForAWSS3Bucket creates a secret for Loki to connect to s3 bucket
func createSecretForAWSS3Bucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	cred := getAWSCredentialFromCluster(oc)
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	f1, err1 := os.Create(dirname + "/aws_access_key_id")
	o.Expect(err1).NotTo(o.HaveOccurred())
	defer f1.Close()
	_, err = f1.WriteString(cred.AccessKeyID)
	o.Expect(err).NotTo(o.HaveOccurred())
	f2, err2 := os.Create(dirname + "/aws_secret_access_key")
	o.Expect(err2).NotTo(o.HaveOccurred())
	defer f2.Close()
	_, err = f2.WriteString(cred.SecretAccessKey)
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "https://s3." + cred.Region + ".amazonaws.com"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/aws_access_key_id", "--from-file=access_key_secret="+dirname+"/aws_secret_access_key", "--from-literal=region="+cred.Region, "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

func createSecretForODFBucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())
	defer os.RemoveAll(dirname)
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/noobaa-admin", "-n", "openshift-storage", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://s3.openshift-storage.svc"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/AWS_ACCESS_KEY_ID", "--from-file=access_key_secret="+dirname+"/AWS_SECRET_ACCESS_KEY", "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

func createSecretForMinIOBucket(oc *exutil.CLI, bucketName, secretName, ns, minIONS string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/"+minioSecret, "-n", minIONS, "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())

	endpoint := "http://minio." + minIONS + ".svc:9000"
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "--from-file=access_key_id="+dirname+"/access_key_id", "--from-file=access_key_secret="+dirname+"/secret_access_key", "--from-literal=bucketnames="+bucketName, "--from-literal=endpoint="+endpoint, "-n", ns).Execute()
}

// creates a secret for Loki to connect to gcs bucket
func createSecretForGCSBucket(oc *exutil.CLI, bucketName, secretName, ns string) error {
	if len(secretName) == 0 {
		return fmt.Errorf("secret name shouldn't be empty")
	}
	dirname := "/tmp/" + oc.Namespace() + "-creds"
	defer os.RemoveAll(dirname)
	err := os.MkdirAll(dirname, 0777)
	o.Expect(err).NotTo(o.HaveOccurred())

	// for GCP STS clusters, get gcp-credentials from env var GOOGLE_APPLICATION_CREDENTIALS
	// TODO: support using STS token to create the secret
	_, err = oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "gcp-credentials", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		gcsCred := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "-n", ns, "--from-literal=bucketname="+bucketName, "--from-file=key.json="+gcsCred).Execute()
	}
	_, err = oc.AsAdmin().WithoutNamespace().Run("extract").Args("secret/gcp-credentials", "-n", "kube-system", "--confirm", "--to="+dirname).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", secretName, "-n", ns, "--from-literal=bucketname="+bucketName, "--from-file=key.json="+dirname+"/service_account.json").Execute()
}

// creates a secret for Loki to connect to azure container
func createSecretForAzureContainer(oc *exutil.CLI, bucketName, secretName, ns string) error {
	environment := "AzureGlobal"
	accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
	if err1 != nil {
		return fmt.Errorf("can't get azure storage account from cluster: %v", err1)
	}
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, secretName, "--from-literal=environment="+environment, "--from-literal=container="+bucketName, "--from-literal=account_name="+accountName, "--from-literal=account_key="+accountKey).Execute()
	return err
}

func createSecretForSwiftContainer(oc *exutil.CLI, containerName, secretName, ns string, cred *exutil.OpenstackCredentials) error {
	userID, domainID := exutil.GetOpenStackUserIDAndDomainID(cred)
	err := oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", "-n", ns, secretName,
		"--from-literal=auth_url="+cred.Clouds.Openstack.Auth.AuthURL,
		"--from-literal=username="+cred.Clouds.Openstack.Auth.Username,
		"--from-literal=user_domain_name="+cred.Clouds.Openstack.Auth.UserDomainName,
		"--from-literal=user_domain_id="+domainID,
		"--from-literal=user_id="+userID,
		"--from-literal=password="+cred.Clouds.Openstack.Auth.Password,
		"--from-literal=domain_id="+domainID,
		"--from-literal=domain_name="+cred.Clouds.Openstack.Auth.UserDomainName,
		"--from-literal=container_name="+containerName,
		"--from-literal=project_id="+cred.Clouds.Openstack.Auth.ProjectID,
		"--from-literal=project_name="+cred.Clouds.Openstack.Auth.ProjectName,
		"--from-literal=project_domain_id="+domainID,
		"--from-literal=project_domain_name="+cred.Clouds.Openstack.Auth.UserDomainName).Execute()
	return err
}

// checkODF check if the ODF is installed in the cluster or not
// here only checks the sc/ocs-storagecluster-ceph-rbd and svc/s3
func checkODF(oc *exutil.CLI) bool {
	scFound, svcFound := false, false
	scs, err := oc.AdminKubeClient().StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	for _, sc := range scs.Items {
		if sc.Name == "ocs-storagecluster-ceph-rbd" {
			scFound = true
			break
		}
	}
	_, err = oc.AdminKubeClient().CoreV1().Services("openshift-storage").Get(context.Background(), "s3", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return scFound && svcFound
}

// checkMinIO
func checkMinIO(oc *exutil.CLI, ns string) bool {
	podReady, svcFound := false, false
	pod, err := oc.AdminKubeClient().CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: "app=minio"})
	o.Expect(err).NotTo(o.HaveOccurred())
	if len(pod.Items) > 0 && pod.Items[0].Status.Phase == "Running" {
		podReady = true
	}
	_, err = oc.AdminKubeClient().CoreV1().Services(ns).Get(context.Background(), "minio", metav1.GetOptions{})
	if err == nil {
		svcFound = true
	}
	return podReady && svcFound
}

// return the storage type per different platform
func getStorageType(oc *exutil.CLI) string {
	platform := exutil.CheckPlatform(oc)
	switch platform {
	case "aws":
		{
			return "s3"
		}
	case "gcp":
		{
			return "gcs"
		}
	case "azure":
		{
			return "azure"
		}
	case "openstack":
		{
			return "swift"
		}
	default:
		{
			if checkODF(oc) {
				return "odf"
			}
			if checkMinIO(oc, minioNS) {
				return "minio"
			}
			return ""
		}
	}
}

// lokiStack contains the configurations of loki stack
type lokiStack struct {
	name          string // lokiStack name
	namespace     string // lokiStack namespace
	tSize         string // size
	storageType   string // the backend storage type, currently support s3, gcs, azure, swift, ODF and minIO
	storageSecret string // the secret name for loki to use to connect to backend storage
	storageClass  string // storage class name
	bucketName    string // the butcket or the container name where loki stores it's data in
	template      string // the file used to create the loki stack
}

func (l lokiStack) setTSize(size string) lokiStack {
	l.tSize = size
	return l
}

// prepareResourcesForLokiStack creates buckets/containers in backend storage provider, and creates the secret for Loki to use
func (l lokiStack) prepareResourcesForLokiStack(oc *exutil.CLI) error {
	var err error
	if len(l.bucketName) == 0 {
		return fmt.Errorf("the bucketName should not be empty")
	}
	switch l.storageType {
	case "s3":
		{
			cred := getAWSCredentialFromCluster(oc)
			client := newS3Client(oc, cred)
			err = createS3Bucket(client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForAWSS3Bucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "azure":
		{
			accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
			if err1 != nil {
				return fmt.Errorf("can't get azure storage account from cluster: %v", err1)
			}
			client, err2 := exutil.NewAzureContainerClient(oc, accountName, accountKey, l.bucketName)
			if err2 != nil {
				return err2
			}
			err = exutil.CreateAzureStorageBlobContainer(client)
			if err != nil {
				return err
			}
			err = createSecretForAzureContainer(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "gcs":
		{
			projectID, errGetID := exutil.GetGcpProjectID(oc)
			o.Expect(errGetID).NotTo(o.HaveOccurred())
			err = exutil.CreateGCSBucket(projectID, l.bucketName)
			if err != nil {
				return err
			}
			err = createSecretForGCSBucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "swift":
		{
			cred, err1 := exutil.GetOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := exutil.NewOpenStackClient(cred, "object-store")
			err = exutil.CreateOpenStackContainer(client, l.bucketName)
			if err != nil {
				return err
			}
			err = createSecretForSwiftContainer(oc, l.bucketName, l.storageSecret, l.namespace, cred)
		}
	case "odf":
		{
			cred := getODFCreds(oc)
			client := newS3Client(oc, cred)
			err = createS3Bucket(client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForODFBucket(oc, l.bucketName, l.storageSecret, l.namespace)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newS3Client(oc, cred)
			err = createS3Bucket(client, l.bucketName, cred)
			if err != nil {
				return err
			}
			err = createSecretForMinIOBucket(oc, l.bucketName, l.storageSecret, l.namespace, minioNS)
		}
	}
	return err
}

// deployLokiStack creates the lokiStack CR with basic settings: name, namespace, size, storage.secret.name, storage.secret.type, storageClassName
// optionalParameters is designed for adding parameters to deploy lokiStack with different tenants or some other settings
func (l lokiStack) deployLokiStack(oc *exutil.CLI, optionalParameters ...string) error {
	var storage string
	if l.storageType == "odf" || l.storageType == "minio" {
		storage = "s3"
	} else {
		storage = l.storageType
	}
	parameters := []string{"-f", l.template, "-n", l.namespace, "-p", "NAME=" + l.name, "NAMESPACE=" + l.namespace, "SIZE=" + l.tSize, "SECRET_NAME=" + l.storageSecret, "STORAGE_TYPE=" + storage, "STORAGE_CLASS=" + l.storageClass}
	if len(optionalParameters) != 0 {
		parameters = append(parameters, optionalParameters...)
	}
	file, err := processTemplate(oc, parameters...)
	exutil.AssertWaitPollNoErr(err, fmt.Sprintf("Can not process %v", parameters))
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file, "-n", l.namespace).Execute()
	ls := resource{"lokistack", l.name, l.namespace}
	ls.WaitForResourceToAppear(oc)
	return err
}

func (l lokiStack) waitForLokiStackToBeReady(oc *exutil.CLI) {
	for _, deploy := range []string{l.name + "-distributor", l.name + "-gateway", l.name + "-querier", l.name + "-query-frontend"} {
		WaitForDeploymentPodsToBeReady(oc, l.namespace, deploy)
	}
	for _, ss := range []string{l.name + "-compactor", l.name + "-index-gateway", l.name + "-ingester", l.name + "-ruler"} {
		waitForStatefulsetReady(oc, l.namespace, ss)
	}
}

func (l lokiStack) removeLokiStack(oc *exutil.CLI) {
	resource{"lokistack", l.name, l.namespace}.clear(oc)
	_ = oc.AsAdmin().WithoutNamespace().Run("delete").Args("pvc", "-n", l.namespace, "-l", "app.kubernetes.io/instance="+l.name).Execute()
}

func (l lokiStack) removeObjectStorage(oc *exutil.CLI) {
	resource{"secret", l.storageSecret, l.namespace}.clear(oc)
	var err error
	switch l.storageType {
	case "s3":
		{
			cred := getAWSCredentialFromCluster(oc)
			client := newS3Client(oc, cred)
			err = deleteS3Bucket(client, l.bucketName)
		}
	case "azure":
		{
			accountName, accountKey, err1 := exutil.GetAzureStorageAccountFromCluster(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client, err2 := exutil.NewAzureContainerClient(oc, accountName, accountKey, l.bucketName)
			o.Expect(err2).NotTo(o.HaveOccurred())
			err = exutil.DeleteAzureStorageBlobContainer(client)
		}
	case "gcs":
		{
			err = exutil.DeleteGCSBucket(l.bucketName)
		}
	case "swift":
		{
			cred, err1 := exutil.GetOpenStackCredentials(oc)
			o.Expect(err1).NotTo(o.HaveOccurred())
			client := exutil.NewOpenStackClient(cred, "object-store")
			err = exutil.DeleteOpenStackContainer(client, l.bucketName)
		}
	case "odf":
		{
			cred := getODFCreds(oc)
			client := newS3Client(oc, cred)
			err = deleteS3Bucket(client, l.bucketName)
		}
	case "minio":
		{
			cred := getMinIOCreds(oc, minioNS)
			client := newS3Client(oc, cred)
			err = deleteS3Bucket(client, l.bucketName)
		}
	}
	o.Expect(err).NotTo(o.HaveOccurred())
}

func grantLokiPermissionsToSA(oc *exutil.CLI, rbacName, sa, ns string) {
	rbac := exutil.FixturePath("testdata", "logging", "lokistack", "loki-rbac.yaml")
	file, err := processTemplate(oc, "-f", rbac, "-p", "NAME="+rbacName, "-p", "SA="+sa, "NAMESPACE="+ns)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", file).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func removeLokiStackPermissionFromSA(oc *exutil.CLI, rbacName string) {
	err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("clusterrole/"+rbacName, "clusterrolebinding/"+rbacName).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

// TODO: add an option to provide TLS config
type lokiClient struct {
	username        string //Username for HTTP basic auth.
	password        string //Password for HTTP basic auth
	address         string //Server address.
	orgID           string //adds X-Scope-OrgID to API requests for representing tenant ID. Useful for requesting tenant data when bypassing an auth gateway.
	bearerToken     string //adds the Authorization header to API requests for authentication purposes.
	bearerTokenFile string //adds the Authorization header to API requests for authentication purposes.
	retries         int    //How many times to retry each query when getting an error response from Loki.
	queryTags       string //adds X-Query-Tags header to API requests.
	quiet           bool   //Suppress query metadata.
}

// newLokiClient initializes a lokiClient with server address
func newLokiClient(routeAddress string) *lokiClient {
	client := &lokiClient{}
	client.address = routeAddress
	client.retries = 5
	client.quiet = true
	return client
}

// retry sets how many times to retry each query
func (c *lokiClient) retry(retry int) *lokiClient {
	nc := *c
	nc.retries = retry
	return &nc
}

// withToken sets the token used to do query
func (c *lokiClient) withToken(bearerToken string) *lokiClient {
	nc := *c
	nc.bearerToken = bearerToken
	return &nc
}

/*
func (c *lokiClient) withTokenFile(bearerTokenFile string) *lokiClient {
	nc := *c
	nc.bearerTokenFile = bearerTokenFile
	return &nc
}
*/

func (c *lokiClient) getHTTPRequestHeader() (http.Header, error) {
	h := make(http.Header)
	if c.username != "" && c.password != "" {
		h.Set(
			"Authorization",
			"Basic "+base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password)),
		)
	}
	h.Set("User-Agent", "loki-logcli")

	if c.orgID != "" {
		h.Set("X-Scope-OrgID", c.orgID)
	}

	if c.queryTags != "" {
		h.Set("X-Query-Tags", c.queryTags)
	}

	if (c.username != "" || c.password != "") && (len(c.bearerToken) > 0 || len(c.bearerTokenFile) > 0) {
		return nil, fmt.Errorf("at most one of HTTP basic auth (username/password), bearer-token & bearer-token-file is allowed to be configured")
	}

	if len(c.bearerToken) > 0 && len(c.bearerTokenFile) > 0 {
		return nil, fmt.Errorf("at most one of the options bearer-token & bearer-token-file is allowed to be configured")
	}

	if c.bearerToken != "" {
		h.Set("Authorization", "Bearer "+c.bearerToken)
	}

	if c.bearerTokenFile != "" {
		b, err := os.ReadFile(c.bearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read authorization credentials file %s: %s", c.bearerTokenFile, err)
		}
		bearerToken := strings.TrimSpace(string(b))
		h.Set("Authorization", "Bearer "+bearerToken)
	}
	return h, nil
}

func (c *lokiClient) doRequest(path, query string, quiet bool, out interface{}) error {

	h, err := c.getHTTPRequestHeader()
	if err != nil {
		return err
	}

	return doHTTPRequest(h, c.address, path, query, "GET", quiet, c.retries, out)
}

func (c *lokiClient) doQuery(path string, query string, quiet bool) (*lokiQueryResponse, error) {
	var err error
	var r lokiQueryResponse

	if err = c.doRequest(path, query, quiet, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

/*
//query uses the /api/v1/query endpoint to execute an instant query
func (c *lokiClient) query(logType string, queryStr string, limit int, forward bool, time time.Time) (*lokiQueryResponse, error) {
	direction := func() string {
		if forward {
			return "FORWARD"
		}
		return "BACKWARD"
	}
	qsb := newQueryStringBuilder()
	qsb.setString("query", queryStr)
	qsb.setInt("limit", int64(limit))
	qsb.setInt("time", time.UnixNano())
	qsb.setString("direction", direction())
	logPath := apiPath + logType + queryPath
	return c.doQuery(logPath, qsb.encode(), c.quiet)
}
*/

// queryRange uses the /api/v1/query_range endpoint to execute a range query
// logType: application, infrastructure, audit
// queryStr: string to filter logs, for example: "{kubernetes_namespace_name="test"}"
// limit: max log count
// start: Start looking for logs at this absolute time(inclusive), e.g.: time.Now().Add(time.Duration(-1)*time.Hour) means 1 hour ago
// end: Stop looking for logs at this absolute time (exclusive)
// forward: true means scan forwards through logs, false means scan backwards through logs
func (c *lokiClient) queryRange(logType string, queryStr string, limit int, start, end time.Time, forward bool) (*lokiQueryResponse, error) {
	direction := func() string {
		if forward {
			return "FORWARD"
		}
		return "BACKWARD"
	}
	params := newQueryStringBuilder()
	params.setString("query", queryStr)
	params.setInt32("limit", limit)
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())
	params.setString("direction", direction())
	logPath := ""
	if len(logType) > 0 {
		logPath = apiPath + logType + queryRangePath
	} else {
		logPath = queryRangePath
	}

	return c.doQuery(logPath, params.encode(), c.quiet)
}

func (c *lokiClient) searchLogsInLoki(logType, query string) (*lokiQueryResponse, error) {
	res, err := c.queryRange(logType, query, 5, time.Now().Add(time.Duration(-1)*time.Hour), time.Now(), false)
	return res, err
}

func (c *lokiClient) searchByKey(logType, key, value string) (*lokiQueryResponse, error) {
	res, err := c.searchLogsInLoki(logType, "{"+key+"=\""+value+"\"}")
	return res, err
}

func (c *lokiClient) searchByNamespace(logType, projectName string) (*lokiQueryResponse, error) {
	res, err := c.searchLogsInLoki(logType, "{kubernetes_namespace_name=\""+projectName+"\"}")
	return res, err
}

// extractLogEntities extract the log entities from loki query response, designed for checking the content of log data in Loki
func extractLogEntities(lokiQueryResult *lokiQueryResponse) []LogEntity {
	var lokiLogs []LogEntity
	// convert interface{} to []string
	convert := func(t interface{}) []string {
		var data []string
		switch reflect.TypeOf(t).Kind() {
		case reflect.Slice, reflect.Array:
			s := reflect.ValueOf(t)
			for i := 0; i < s.Len(); i++ {
				data = append(data, fmt.Sprintln(s.Index(i)))
			}
		}
		return data
	}
	/*
		value example:
		  [
		    timestamp,
			log data
		  ]
	*/
	for _, res := range lokiQueryResult.Data.Result {
		for _, value := range res.Values {
			lokiLog := LogEntity{}
			// only process log data, drop timestamp
			json.Unmarshal([]byte(convert(value)[1]), &lokiLog)
			lokiLogs = append(lokiLogs, lokiLog)
		}
	}
	return lokiLogs
}

// listLabelValues uses the /api/v1/label endpoint to list label values
func (c *lokiClient) listLabelValues(logType, name string, start, end time.Time) (*labelResponse, error) {
	lpath := fmt.Sprintf(labelValuesPath, url.PathEscape(name))
	var labelResponse labelResponse
	params := newQueryStringBuilder()
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())

	path := ""
	if len(logType) > 0 {
		path = apiPath + logType + lpath
	} else {
		path = lpath
	}

	if err := c.doRequest(path, params.encode(), c.quiet, &labelResponse); err != nil {
		return nil, err
	}
	return &labelResponse, nil
}

// listLabelNames uses the /api/v1/label endpoint to list label names
func (c *lokiClient) listLabelNames(logType string, start, end time.Time) (*labelResponse, error) {
	var labelResponse labelResponse
	params := newQueryStringBuilder()
	params.setInt("start", start.UnixNano())
	params.setInt("end", end.UnixNano())
	path := ""
	if len(logType) > 0 {
		path = apiPath + logType + labelsPath
	} else {
		path = labelsPath
	}

	if err := c.doRequest(path, params.encode(), c.quiet, &labelResponse); err != nil {
		return nil, err
	}
	return &labelResponse, nil
}

// listLabels gets the label names or values
func (c *lokiClient) listLabels(logType, labelName string, start, end time.Time) []string {
	var labelResponse *labelResponse
	var err error
	if len(labelName) > 0 {
		labelResponse, err = c.listLabelValues(logType, labelName, start, end)
	} else {
		labelResponse, err = c.listLabelNames(logType, start, end)
	}
	if err != nil {
		e2e.Failf("Error doing request: %+v", err)
	}
	return labelResponse.Data
}

func doHTTPRequest(header http.Header, address, path, query, method string, quiet bool, attempts int, out interface{}) error {
	us, err := buildURL(address, path, query)
	if err != nil {
		return err
	}
	if !quiet {
		e2e.Logf(us)
	}

	req, err := http.NewRequest(strings.ToUpper(method), us, nil)
	if err != nil {
		return err
	}

	req.Header = header

	var tr *http.Transport
	proxy := getProxyFromEnv()
	if len(proxy) > 0 {
		proxyURL, err := url.Parse(proxy)
		o.Expect(err).NotTo(o.HaveOccurred())
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyURL(proxyURL),
		}
	} else {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	client := &http.Client{Transport: tr}

	var resp *http.Response
	success := false

	for attempts > 0 {
		attempts--

		resp, err = client.Do(req)
		if err != nil {
			e2e.Logf("error sending request %v", err)
			continue
		}
		if resp.StatusCode/100 != 2 {
			buf, _ := io.ReadAll(resp.Body) // nolint
			e2e.Logf("Error response from server: %s (%v) attempts remaining: %d", string(buf), err, attempts)
			if err := resp.Body.Close(); err != nil {
				e2e.Logf("error closing body", err)
			}
			continue
		}
		success = true
		break
	}
	if !success {
		return fmt.Errorf("run out of attempts while querying the server")
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			e2e.Logf("error closing body", err)
		}
	}()
	return json.NewDecoder(resp.Body).Decode(out)
}

// buildURL concats a url `http://foo/bar` with a path `/buzz`.
func buildURL(u, p, q string) (string, error) {
	url, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	url.Path = path.Join(url.Path, p)
	url.RawQuery = q
	return url.String(), nil
}

type queryStringBuilder struct {
	values url.Values
}

func newQueryStringBuilder() *queryStringBuilder {
	return &queryStringBuilder{
		values: url.Values{},
	}
}

func (b *queryStringBuilder) setString(name, value string) {
	b.values.Set(name, value)
}

func (b *queryStringBuilder) setInt(name string, value int64) {
	b.setString(name, strconv.FormatInt(value, 10))
}

func (b *queryStringBuilder) setInt32(name string, value int) {
	b.setString(name, strconv.Itoa(value))
}

/*
func (b *queryStringBuilder) setStringArray(name string, values []string) {
	for _, v := range values {
		b.values.Add(name, v)
	}
}
func (b *queryStringBuilder) setFloat32(name string, value float32) {
	b.setString(name, strconv.FormatFloat(float64(value), 'f', -1, 32))
}
func (b *queryStringBuilder) setFloat(name string, value float64) {
	b.setString(name, strconv.FormatFloat(value, 'f', -1, 64))
}
*/

// encode returns the URL-encoded query string based on key-value
// parameters added to the builder calling Set functions.
func (b *queryStringBuilder) encode() string {
	return b.values.Encode()
}

// compareClusterResources compares the remaning resource with the requested resource provide by user
func compareClusterResources(oc *exutil.CLI, cpu, memory string) bool {
	nodes, err := exutil.GetSchedulableLinuxWorkerNodes(oc)
	o.Expect(err).NotTo(o.HaveOccurred())
	var remainingCPU, remainingMemory int64
	re := exutil.GetRemainingResourcesNodesMap(oc, nodes)
	for _, node := range nodes {
		remainingCPU += re[node.Name].CPU
		remainingMemory += re[node.Name].Memory
	}

	requiredCPU, _ := k8sresource.ParseQuantity(cpu)
	requiredMemory, _ := k8sresource.ParseQuantity(memory)
	e2e.Logf("the required cpu is: %d, and the required memory is: %d", requiredCPU.MilliValue(), requiredMemory.MilliValue())
	e2e.Logf("the remaining cpu is: %d, and the remaning memory is: %d", remainingCPU, remainingMemory)
	return remainingCPU > requiredCPU.MilliValue() && remainingMemory > requiredMemory.MilliValue()
}

// validateInfraAndResourcesForLoki checks cluster remaning resources and platform type
// validateInfraAndResourcesForLoki checks cluster remaning resources and platform type
// supportedPlatforms the platform types which the case can be executed on, if it's empty, then skip this check
func validateInfraAndResourcesForLoki(oc *exutil.CLI, supportedPlatforms []string, reqMemory, reqCPU string) bool {
	currentPlatform := exutil.CheckPlatform(oc)
	if currentPlatform == "aws" {
		// skip the case on aws sts clusters
		_, err := oc.AdminKubeClient().CoreV1().Secrets("kube-system").Get(context.Background(), "aws-creds", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false
		}
	}
	if len(supportedPlatforms) > 0 {
		return contain(supportedPlatforms, currentPlatform) && compareClusterResources(oc, reqCPU, reqMemory)
	}
	return compareClusterResources(oc, reqCPU, reqMemory)
}

type externalLoki struct {
	name      string
	namespace string
}

func (l externalLoki) deployLoki(oc *exutil.CLI) {
	//Create configmap for Loki
	cmTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "loki", "loki-configmap.yaml")
	lokiCM := resource{"configmap", l.name, l.namespace}
	err := lokiCM.applyFromTemplate(oc, "-n", l.namespace, "-f", cmTemplate, "-p", "LOKINAMESPACE="+l.namespace, "-p", "LOKICMNAME="+l.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//Create Deployment for Loki
	deployTemplate := exutil.FixturePath("testdata", "logging", "external-log-stores", "loki", "loki-deployment.yaml")
	lokiDeploy := resource{"deployment", l.name, l.namespace}
	err = lokiDeploy.applyFromTemplate(oc, "-n", l.namespace, "-f", deployTemplate, "-p", "LOKISERVERNAME="+l.name, "-p", "LOKINAMESPACE="+l.namespace, "-p", "LOKICMNAME="+l.name)
	o.Expect(err).NotTo(o.HaveOccurred())

	//Expose Loki as a Service
	WaitForDeploymentPodsToBeReady(oc, l.namespace, l.name)
	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("-n", l.namespace, "deployment", l.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())

	// expose loki route
	err = oc.AsAdmin().WithoutNamespace().Run("expose").Args("-n", l.namespace, "svc", l.name).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func (l externalLoki) remove(oc *exutil.CLI) {
	resource{"configmap", l.name, l.namespace}.clear(oc)
	resource{"deployment", l.name, l.namespace}.clear(oc)
	resource{"svc", l.name, l.namespace}.clear(oc)
	resource{"route", l.name, l.namespace}.clear(oc)
}

func deployMinIO(oc *exutil.CLI) {
	// create namespace
	_, err := oc.AdminKubeClient().CoreV1().Namespaces().Get(context.Background(), minioNS, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("namespace", minioNS).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	// create secret
	_, err = oc.AdminKubeClient().CoreV1().Secrets(minioNS).Get(context.Background(), minioSecret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = oc.AsAdmin().WithoutNamespace().Run("create").Args("secret", "generic", minioSecret, "-n", minioNS, "--from-literal=access_key_id="+getRandomString(), "--from-literal=secret_access_key=passwOOrd"+getRandomString()).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
	// deploy minIO
	deployTemplate := exutil.FixturePath("testdata", "logging", "minIO", "deploy.yaml")
	deployFile, err := processTemplate(oc, "-n", minioNS, "-f", deployTemplate, "-p", "NAMESPACE="+minioNS, "NAME=minio", "SECRET_NAME="+minioSecret)
	o.Expect(err).NotTo(o.HaveOccurred())
	err = oc.AsAdmin().Run("apply").Args("-f", deployFile, "-n", minioNS).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
	// wait for minio to be ready
	for _, rs := range []string{"deployment", "svc", "route"} {
		resource{rs, "minio", minioNS}.WaitForResourceToAppear(oc)
	}
	WaitForDeploymentPodsToBeReady(oc, minioNS, "minio")
}

func removeMinIO(oc *exutil.CLI) {
	deleteNamespace(oc, minioNS)
}
