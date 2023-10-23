package util

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"

	e2e "k8s.io/kubernetes/test/e2e/framework"
)

// AWSInstanceNotFound custom error for not found instances
type AWSInstanceNotFound struct{ InstanceName string }

// Error implements the error interface
func (nfe *AWSInstanceNotFound) Error() string {
	return fmt.Sprintf("No instance found in current cluster with name %s", nfe.InstanceName)
}

// AwsClient struct
type AwsClient struct {
	svc *ec2.EC2
}

// InitAwsSession init session
func InitAwsSession() *AwsClient {
	mySession := session.Must(session.NewSession())
	aClient := &AwsClient{
		svc: ec2.New(mySession, aws.NewConfig()),
	}

	return aClient
}

func InitAwsSessionWithRegion(region string) *AwsClient {
	mySession := session.Must(session.NewSession())
	aClient := &AwsClient{
		svc: ec2.New(mySession, aws.NewConfig().WithRegion(region)),
	}

	return aClient
}

// GetAwsInstanceID Get int svc instance ID
func (a *AwsClient) GetAwsInstanceID(instanceName string) (string, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("tag:Name"),
			Values: []*string{
				aws.String(instanceName),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)

	if err != nil {
		return "", err
	}

	if len(instanceInfo.Reservations) < 1 {
		return "", &AWSInstanceNotFound{instanceName}
	}

	instanceID := instanceInfo.Reservations[0].Instances[0].InstanceId
	e2e.Logf("The %s instance id is %s .", instanceName, *instanceID)
	return *instanceID, err
}

// GetAwsIntIPs get aws int ip
func (a *AwsClient) GetAwsIntIPs(instanceID string) (map[string]string, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("instance-id"),
			Values: []*string{
				aws.String(instanceID),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)
	if err != nil {
		return nil, err
	}

	if len(instanceInfo.Reservations) < 1 {
		return nil, fmt.Errorf("No instance found in current cluster with ID %s", instanceID)
	}

	privateIP := instanceInfo.Reservations[0].Instances[0].PrivateIpAddress
	publicIP := instanceInfo.Reservations[0].Instances[0].PublicIpAddress
	ips := make(map[string]string, 3)

	if publicIP == nil && privateIP == nil {
		e2e.Logf("There is no ips for this instance %s", instanceID)
		return nil, fmt.Errorf("There is no ips for this instance %s", instanceID)
	}

	if publicIP != nil {
		ips["publicIP"] = *publicIP
		e2e.Logf("The instance's public ip is %s", *publicIP)
	}

	if privateIP != nil {
		ips["privateIP"] = *privateIP
		e2e.Logf("The instance's private ip is %s", *privateIP)
	}

	return ips, nil
}

// UpdateAwsIntSecurityRule update int security rule
func (a *AwsClient) UpdateAwsIntSecurityRule(instanceID string, dstPort int64) error {
	filters := []*ec2.Filter{
		{
			Name: aws.String("instance-id"),
			Values: []*string{
				aws.String(instanceID),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)
	if err != nil {
		return err
	}

	if len(instanceInfo.Reservations) < 1 {
		return fmt.Errorf("No such instance ID in current cluster %s", instanceID)
	}

	securityGroupID := instanceInfo.Reservations[0].Instances[0].SecurityGroups[0].GroupId

	e2e.Logf("The instance's %s,security group id is %s .", instanceID, *securityGroupID)

	// Check if destination port is opned
	req := &ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(*securityGroupID)},
	}
	resp, err := a.svc.DescribeSecurityGroups(req)
	if err != nil {
		return err
	}

	if strings.Contains(resp.GoString(), "ToPort: "+strconv.FormatInt(dstPort, 10)) {
		e2e.Logf("The destination port %v was opened in security group %s .", dstPort, *securityGroupID)
		return nil
	}

	// Update ingress secure rule to allow destination port
	_, err = a.svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(*securityGroupID),
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("tcp").
				SetFromPort(dstPort).
				SetToPort(dstPort).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				}),
		},
	})

	if err != nil {
		e2e.Logf("Unable to set security group %s, ingress, %v", *securityGroupID, err)
		return err
	}

	e2e.Logf("Successfully update destination port %v to security group %s ingress rule.", dstPort, *securityGroupID)

	return nil
}

// GetAwsInstanceIDFromHostname Get instance ID from hostname
func (a *AwsClient) GetAwsInstanceIDFromHostname(hostname string) (string, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("private-dns-name"),
			Values: []*string{
				aws.String(hostname),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)

	if err != nil {
		return "", err
	}

	if len(instanceInfo.Reservations) < 1 {
		return "", fmt.Errorf("No instance found in current cluster with name %s", hostname)
	}

	instanceID := instanceInfo.Reservations[0].Instances[0].InstanceId
	e2e.Logf("The %s instance id is %s .", hostname, *instanceID)
	return *instanceID, err
}

// StartInstance Start an instance
func (a *AwsClient) StartInstance(instanceID string) error {
	if instanceID == "" {
		e2e.Logf("You must supply an instance ID (-i INSTANCE-ID")
		return fmt.Errorf("You must supply an instance ID (-i INSTANCE-ID")
	}
	input := &ec2.StartInstancesInput{
		InstanceIds: []*string{
			&instanceID,
		},
	}
	result, err := a.svc.StartInstances(input)
	e2e.Logf("%v", result.StartingInstances)
	return err
}

// StopInstance Stop an instance
func (a *AwsClient) StopInstance(instanceID string) error {
	if instanceID == "" {
		e2e.Logf("You must supply an instance ID (-i INSTANCE-ID")
		return fmt.Errorf("You must supply an instance ID (-i INSTANCE-ID")
	}
	input := &ec2.StopInstancesInput{
		InstanceIds: []*string{
			&instanceID,
		},
	}
	result, err := a.svc.StopInstances(input)
	e2e.Logf("%v", result.StoppingInstances)
	return err
}

// GetAwsInstanceState gives the instance state
func (a *AwsClient) GetAwsInstanceState(instanceID string) (string, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("instance-id"),
			Values: []*string{
				aws.String(instanceID),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)
	if err != nil {
		return "", err
	}

	if len(instanceInfo.Reservations) < 1 {
		return "", fmt.Errorf("No instance found in current cluster with ID %s", instanceID)
	}

	instanceState := instanceInfo.Reservations[0].Instances[0].State.Name
	return *instanceState, err
}

// CreateDhcpOptions Create a dhcpOptions
func (a *AwsClient) CreateDhcpOptions() (string, error) {
	input := &ec2.CreateDhcpOptionsInput{
		DhcpConfigurations: []*ec2.NewDhcpConfiguration{
			{
				Key: aws.String("domain-name-servers"),
				Values: []*string{
					aws.String("AmazonProvidedDNS"),
				},
			},
		},
	}
	result, err := a.svc.CreateDhcpOptions(input)
	if err != nil {
		e2e.Logf("err: %v", err)
		return "", err
	}
	dhcpOptionsID := result.DhcpOptions.DhcpOptionsId
	e2e.Logf("The created dhcpOptionsId is %s", *dhcpOptionsID)
	return *dhcpOptionsID, err
}

// DeleteDhcpOptions Delete a dhcpOptions
func (a *AwsClient) DeleteDhcpOptions(dhcpOptionsID string) error {
	input := &ec2.DeleteDhcpOptionsInput{
		DhcpOptionsId: aws.String(dhcpOptionsID),
	}
	_, err := a.svc.DeleteDhcpOptions(input)
	return err
}

// GetAwsInstanceVPCId gives the instance vpcID
func (a *AwsClient) GetAwsInstanceVPCId(instanceID string) (string, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("instance-id"),
			Values: []*string{
				aws.String(instanceID),
			},
		},
	}
	input := ec2.DescribeInstancesInput{Filters: filters}
	instanceInfo, err := a.svc.DescribeInstances(&input)
	if err != nil {
		return "", err
	}

	if len(instanceInfo.Reservations) < 1 {
		return "", fmt.Errorf("No instance found in current cluster with ID %s", instanceID)
	}

	instanceVpcID := instanceInfo.Reservations[0].Instances[0].VpcId
	return *instanceVpcID, err
}

// GetDhcpOptionsIDOfVpc Get VPC's dhcpOptionsID
func (a *AwsClient) GetDhcpOptionsIDOfVpc(vpcID string) (string, error) {
	input := &ec2.DescribeVpcsInput{
		VpcIds: []*string{
			aws.String(vpcID),
		},
	}
	result, err := a.svc.DescribeVpcs(input)
	if err != nil {
		e2e.Logf("err: %v", err)
		return "", err
	}
	dhcpOptionsID := result.Vpcs[0].DhcpOptionsId
	e2e.Logf("The %s dhcpOptionsId is %s ", vpcID, *dhcpOptionsID)
	return *dhcpOptionsID, err
}

// AssociateDhcpOptions Associate a VPC with a dhcpOptions
func (a *AwsClient) AssociateDhcpOptions(vpcID, dhcpOptionsID string) error {
	input := &ec2.AssociateDhcpOptionsInput{
		VpcId:         aws.String(vpcID),
		DhcpOptionsId: aws.String(dhcpOptionsID),
	}
	_, err := a.svc.AssociateDhcpOptions(input)
	return err
}

func (a *AwsClient) CreateSecurityGroup(groupName, vpcID, description string) (string, error) {
	createRes, err := a.svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(groupName),
		Description: aws.String(description),
		VpcId:       aws.String(vpcID),
	})
	if err != nil {
		return "", err
	}

	return *createRes.GroupId, nil
}

func (a *AwsClient) DeleteSecurityGroup(groupID string) error {
	_, err := a.svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(groupID),
	})
	return err
}

func (a *AwsClient) GetInstanceSecurityGroupIDs(instanceID string) ([]string, error) {
	filters := []*ec2.Filter{
		{
			Name:   aws.String("instance-id"),
			Values: []*string{aws.String(instanceID)},
		},
		{
			Name:   aws.String("instance.group-name"),
			Values: []*string{aws.String("*")},
		},
	}

	input := &ec2.DescribeInstancesInput{Filters: filters}
	result, err := a.svc.DescribeInstances(input)
	if err != nil {
		return nil, err
	}

	if len(result.Reservations) < 1 {
		return nil, fmt.Errorf("No instance found in current cluster with ID %s", instanceID)
	}

	instance := result.Reservations[0].Instances[0]

	var securityGroups []string
	for _, group := range instance.SecurityGroups {
		securityGroups = append(securityGroups, *group.GroupId)
	}

	return securityGroups, err
}

func (a *AwsClient) CreateTag(resource string, key string, value string) error {
	createTagInput := &ec2.CreateTagsInput{
		Resources: []*string{aws.String(resource)},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String(key),
				Value: aws.String(value),
			},
		},
	}
	_, err := a.svc.CreateTags(createTagInput)
	return err
}

func (a *AwsClient) DescribeVpcEndpoint(endpointID string) (*ec2.VpcEndpoint, error) {
	res, err := a.svc.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
		VpcEndpointIds: aws.StringSlice([]string{endpointID}),
	})
	if err != nil {
		return nil, err
	}
	return res.VpcEndpoints[0], nil
}

func (a *AwsClient) GetSecurityGroupsByVpcEndpointID(endpointID string) ([]*ec2.SecurityGroupIdentifier, error) {
	ep, err := a.DescribeVpcEndpoint(endpointID)
	if err != nil {
		return []*ec2.SecurityGroupIdentifier{}, err
	}

	return ep.Groups, nil
}

func (a *AwsClient) GetDefaultSecurityGroupByVpcID(vpcID string) (*ec2.SecurityGroup, error) {
	filters := []*ec2.Filter{
		{
			Name: aws.String("vpc-id"),
			Values: []*string{
				aws.String(vpcID),
			},
		},
		{
			Name: aws.String("group-name"),
			Values: []*string{
				aws.String("default"),
			},
		},
	}
	input := ec2.DescribeSecurityGroupsInput{Filters: filters}
	ep, err := a.svc.DescribeSecurityGroups(&input)
	if err != nil {
		return nil, err
	}

	return ep.SecurityGroups[0], nil
}

func (a *AwsClient) GetAvailabilityZoneNames() ([]string, error) {
	zones, err := a.svc.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return nil, err
	}
	var zoneNames []string
	for _, az := range zones.AvailabilityZones {
		if az.ZoneName != nil {
			zoneNames = append(zoneNames, *az.ZoneName)
		}
	}
	return zoneNames, nil
}

// S3Client struct for S3 storage operations
type S3Client struct {
	svc *s3.S3
}

// NewS3Client constructor to create S3 client with default credential and config
func NewS3Client() *S3Client {
	return &S3Client{
		svc: s3.New(
			session.Must(session.NewSession()),
		),
	}
}

// NewS3ClientFromCredFile constrctor to create S3 client with user's credential file and region
// param: filename crednetial file path
// param: profile config profile e.g. [default]
// param: region
func NewS3ClientFromCredFile(filename, profile, region string) *S3Client {

	awsSession := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigDisable,
		},
	))

	return &S3Client{
		svc: s3.New(
			awsSession,
			aws.NewConfig().
				WithRegion(region).
				WithCredentials(credentials.NewSharedCredentials(filename, "default")),
		),
	}

}

// CreateBucket create S3 bucket
// param: bucket name from user input
func (sc *S3Client) CreateBucket(name string) error {

	e2e.Logf("creating s3 bucket %s", name)

	var createBucketInput *s3.CreateBucketInput
	if *sc.svc.Config.Region == "us-east-1" {
		createBucketInput = &s3.CreateBucketInput{
			Bucket: aws.String(name),
			ACL:    aws.String(s3.BucketCannedACLPublicRead),
		}
	} else {
		createBucketInput = &s3.CreateBucketInput{
			Bucket: aws.String(name),
			CreateBucketConfiguration: &s3.CreateBucketConfiguration{
				LocationConstraint: aws.String(*sc.svc.Config.Region),
			},
		}
	}

	cbo, cboe := sc.svc.CreateBucket(createBucketInput)
	if cboe != nil {
		e2e.Logf("create bucket %s failed: %v", name, cboe)
		return cboe
	}

	e2e.Logf("bucket %s is created successfully %v", name, cbo)

	_, doe := sc.svc.DeletePublicAccessBlock(&s3.DeletePublicAccessBlockInput{
		Bucket: aws.String(name),
	})
	if doe != nil {
		e2e.Logf("delete public access block failed on bucket %s: %v", name, doe)
		return doe
	}

	return nil

}

// PutBucketPolicy configures a given bucket with a policy
// param: name bucket name
// param: policy policy that will be added the bucket
func (sc *S3Client) PutBucketPolicy(name, policy string) error {
	e2e.Logf("Setting policy in bucket %s. Policy: %s", name, policy)

	input := &s3.PutBucketPolicyInput{
		Bucket: aws.String(name),
		Policy: aws.String(policy),
	}

	result, err := sc.svc.PutBucketPolicy(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			e2e.Logf("AWS Error %s setting policy in bucket %s: %s", aerr.Code(), name, aerr.Error())
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			e2e.Logf("Error setting policy in bucket %s: %s", name, err.Error())
		}
		return err
	}

	e2e.Logf("Policy result: %s", result)

	return nil
}

// DeleteBucket delete S3 bucket
// param: name bucket name from user input
func (sc *S3Client) DeleteBucket(name string) error {

	e2e.Logf("deleting s3 bucket %s", name)

	deleteBucketInput := &s3.DeleteBucketInput{
		Bucket: aws.String(name),
	}

	_, dboe := sc.svc.DeleteBucket(deleteBucketInput)
	if dboe != nil {
		e2e.Logf("delete bucket %s failed: %v", name, dboe)
		return dboe
	}

	e2e.Logf("bucket %s is successfully deleted", name)

	return nil
}

// HeadBucket util func to check whether bucket exists or not
// param: name bucket name
func (sc *S3Client) HeadBucket(name string) error {

	e2e.Logf("check bucket %s exists or not", name)

	headBucketInput := &s3.HeadBucketInput{
		Bucket: aws.String(name),
	}

	hbo, hboe := sc.svc.HeadBucket(headBucketInput)
	if hboe != nil {
		e2e.Logf("head bucket %s failed: %v", name, hboe)
		return hboe
	}

	e2e.Logf("head bucket %s output is %v", name, hbo)

	return nil

}

// IAMClient struct for IAM operations
type IAMClient struct {
	svc *iam.IAM
}

// NewIAMClient constructor to create IAM client with default credential and config
// Should use GetAwsCredentialFromCluster(oc) to set ENV first before using it
func NewIAMClient() *IAMClient {
	return &IAMClient{
		svc: iam.New(
			session.Must(session.NewSession()),
			aws.NewConfig(),
		),
	}
}

// NewIAMClientFromCredFile constructor to create IAM client with user's credential file
func NewIAMClientFromCredFile(filename, region string) *IAMClient {
	return &IAMClient{
		svc: iam.New(
			session.Must(session.NewSession()),
			aws.NewConfig().WithCredentials(credentials.NewSharedCredentials(filename, "default")).WithRegion(region),
		),
	}
}

func (iamClient *IAMClient) DeleteOpenIDConnectProviderByProviderName(providerName string) error {
	oidcProviderList, err := iamClient.svc.ListOpenIDConnectProviders(&iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return err
	}

	for _, provider := range oidcProviderList.OpenIDConnectProviderList {
		if strings.Contains(*provider.Arn, providerName) {
			_, err := iamClient.svc.DeleteOpenIDConnectProvider(&iam.DeleteOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: provider.Arn,
			})
			if err != nil {
				e2e.Logf("Failed to Delete existing OIDC provider arn: %s for providerName: %s", *provider.Arn, providerName)
				return err
			}
			break
		}
	}
	return nil
}

func (iamClient *IAMClient) GetRolePolicy(roleName, policyName string) (string, error) {
	rc, err := iamClient.svc.GetRolePolicy(&iam.GetRolePolicyInput{
		PolicyName: aws.String(policyName),
		RoleName:   aws.String(roleName),
	})

	if err != nil {
		e2e.Logf("Failed to GetRolePolicy with roleName: %s policyName %s error %s", roleName, policyName, err.Error())
		return "", err
	}

	decodePolicy, err := url.QueryUnescape(*rc.PolicyDocument)
	if err != nil {
		e2e.Logf("Failed to QueryUnescape role policy: role %s policyName %s error %s original rc %s", roleName, policyName, err.Error(), *rc.PolicyDocument)
		return "", err
	}

	return decodePolicy, nil
}

func (iamClient *IAMClient) UpdateRolePolicy(roleName, policyName, policyDocument string) error {
	_, err := iamClient.svc.PutRolePolicy(&iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(policyDocument),
	})

	if err != nil {
		e2e.Logf("Failed to UpdateRolePolicy for roleName: %s policyName %s error %s", roleName, policyName, err.Error())
	}

	return err
}

// Create policy
func (iamClient *IAMClient) CreatePolicy(policyDocument string, policyName string, description string, tagList map[string]string, path string) (string, error) {
	//     Check that required inputs exist
	if policyDocument == "" || policyName == "" {
		return "", errors.New("policyDocument or policyName can be an empty string")
	}
	createPolicyInput := &iam.CreatePolicyInput{
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(policyDocument),
	}
	if path != "" {
		createPolicyInput.Path = aws.String(path)
	}
	if description != "" {
		createPolicyInput.Description = aws.String(description)
	}
	if len(tagList) > 0 {
		createPolicyInput.Tags = getTags(tagList)
	}

	output, err := iamClient.svc.CreatePolicy(createPolicyInput)

	return aws.StringValue(output.Policy.Arn), err
}

// Delete policy
func (iamClient *IAMClient) DeletePolicy(policyArn string) error {
	_, err := iamClient.svc.DeletePolicy(&iam.DeletePolicyInput{
		PolicyArn: aws.String(policyArn),
	})
	return err
}

// convert tags map to []iam.Tag
func getTags(tagList map[string]string) []*iam.Tag {
	iamTags := []*iam.Tag{}
	for k, v := range tagList {
		iamTags = append(iamTags, &iam.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return iamTags
}

func (iamClient *IAMClient) ListRoles() ([]*iam.Role, error) {
	roles := []*iam.Role{}
	err := iamClient.svc.ListRolesPages(&iam.ListRolesInput{}, func(page *iam.ListRolesOutput, lastPage bool) bool {
		roles = append(roles, page.Roles...)
		return aws.BoolValue(page.IsTruncated)
	})
	return roles, err
}

func (iamClient *IAMClient) ListOperatsorRolesByPrefix(prefix string, version string) ([]*iam.Role, error) {
	operatorRoles := []*iam.Role{}
	roles, err := iamClient.ListRoles()
	if err != nil {
		return operatorRoles, err
	}
	prefixOperatorRoleRE := regexp.MustCompile(`(?i)(?P<Prefix>[\w+=,.@-]+)-(openshift|kube-system)`)
	for _, role := range roles {
		matches := prefixOperatorRoleRE.FindStringSubmatch(*role.RoleName)
		if len(matches) == 0 {
			continue
		}
		prefixIndex := prefixOperatorRoleRE.SubexpIndex("Prefix")
		foundPrefix := strings.ToLower(matches[prefixIndex])
		if foundPrefix != prefix {
			continue
		}
		operatorRoles = append(operatorRoles, role)
	}
	return operatorRoles, nil
}
