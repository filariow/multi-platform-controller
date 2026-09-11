package aws

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/go-logr/logr"
	"github.com/konflux-ci/multi-platform-controller/pkg/cloud"
	v1 "k8s.io/api/core/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errNoInstancesCreated = errors.New("no EC2 instances were created")

// pingIPAddress tries to connect to the SSH port on ipAddress with a 60-second timeout.
// An error is returned if the connection fails.
func pingIPAddress(ipAddress string) error {
	conn, err := net.DialTimeout("tcp", ipAddress+":22", 60*time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}

// validateIPAddress returns the IP address of the EC2 instance after verifying SSH connectivity.
// Returns an error if no IP is available or if the instance is not reachable via SSH.
func (ec AWSEc2DynamicConfig) validateIPAddress(ctx context.Context, instance *types.Instance) (string, error) {
	log := logr.FromContextOrDiscard(ctx)
	var ip string

	// Select IP address based on priority: PublicDnsName > PublicIpAddress > PrivateIpAddress
	if instance.PublicDnsName != nil && *instance.PublicDnsName != "" {
		ip = *instance.PublicDnsName
	} else if instance.PublicIpAddress != nil && *instance.PublicIpAddress != "" {
		ip = *instance.PublicIpAddress
	} else if instance.PrivateIpAddress != nil && *instance.PrivateIpAddress != "" && !ec.StrictPublicAddress {
		ip = *instance.PrivateIpAddress
	}

	// Return error if no IP address is available
	if ip == "" {
		return "", fmt.Errorf("instance %s has no accessible IP address", aws.ToString(instance.InstanceId))
	}

	// Verify SSH connectivity
	ping := pingIPAddress
	if ec.pingFunc != nil {
		ping = ec.pingFunc
	}
	if err := ping(ip); err != nil {
		log.Error(err, "failed to connect to AWS instance via SSH",
			"instanceID", aws.ToString(instance.InstanceId),
			"ipAddress", ip)
		return "", fmt.Errorf("failed to resolve IP address %s: %w", ip, err)
	}

	log.Info("Successfully validated IP address",
		"instanceID", aws.ToString(instance.InstanceId),
		"ipAddress", ip)

	return ip, nil
}

// getEC2Client returns the injected mock client if set, otherwise builds a real
// EC2 client from AWS credentials.
func (ec AWSEc2DynamicConfig) getEC2Client(kubeClient client.Client, ctx context.Context) (ec2API, error) {
	if ec.ec2Client != nil {
		return ec.ec2Client, nil
	}
	secretCredentials := SecretCredentialsProvider{Name: ec.Secret, Namespace: ec.SystemNamespace, Client: kubeClient}
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(secretCredentials),
		config.WithRegion(ec.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to create an AWS config for an EC2 client: %w", err)
	}
	return ec2.NewFromConfig(cfg), nil
}

func (ec AWSEc2DynamicConfig) isDedicatedHostConfig() bool {
	return ec.Tenancy != "" && ec.HostResourceGroupArn != "" && ec.LicenseConfigurationArn != ""
}

func (ec AWSEc2DynamicConfig) launchSubnets() []string {
	// DedicatedHosts don't support SubnetIds
	if ec.isDedicatedHostConfig() {
		if ec.SubnetId != "" {
			return []string{ec.SubnetId}
		}
		return nil
	}

	// use SubnetIds if set, it's expected to store
	// SubnetId as its first element
	if len(ec.SubnetIds) > 0 {
		return ec.SubnetIds
	}

	// fallback to SubnetId if SubnetIds is not set
	if ec.SubnetId != "" {
		return []string{ec.SubnetId}
	}

	return nil
}

func parseSubnetIds(primarySubnetId, fallbackSubnetIds string) []string {
	sanitize := strings.TrimSpace

	// parse the Primary Subnet
	subnets := make([]string, 0, 1)
	sanitizedPrimarySubnetId := sanitize(primarySubnetId)
	if sanitizedPrimarySubnetId != "" {
		subnets = append(subnets, sanitizedPrimarySubnetId)
	}

	// parse the FallBack Subnets
	skip := func(s string) bool { return s == "" || s == sanitizedPrimarySubnetId }
	fallbackSubnets := parseFallbackSubnetIds(fallbackSubnetIds, skip, sanitize)

	// concat the subnets
	return slices.Concat(subnets, fallbackSubnets)
}

func parseFallbackSubnetIds(fallbackSubnetIds string, skip func(string) bool, sanitize func(string) string) []string {
	// define the separator
	sep := ","

	// create a big enough subnet slice
	subnets := make([]string, 0, strings.Count(fallbackSubnetIds, sep))

	// range on all sanitized splits
	for s := range SplitSeqTransform(fallbackSubnetIds, sep, sanitize) {
		// check the subnet isn't a duplicate and it not a value we should skip
		if !slices.Contains(subnets, s) && !skip(s) {
			subnets = append(subnets, s)
		}
	}

	// release the un-needed capacity
	return slices.Clip(subnets)
}

// SplitSeqTransform splits a string and applies a custom transformation function to each segment.
func SplitSeqTransform(s, sep string, transform func(string) string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for segment := range strings.SplitSeq(s, sep) {
			if !yield(transform(segment)) {
				return
			}
		}
	}
}

func (ec AWSEc2DynamicConfig) launchInstanceInSubnets(
	ctx context.Context,
	ec2Client ec2API,
	log logr.Logger,
	taskRunName, instanceTag string,
	additionalInstanceTags map[string]string,
) (cloud.InstanceIdentifier, error) {
	var triedSubnets []string
	var lastCapacityErr error

	subnets := ec.launchSubnets()
	if len(subnets) == 0 {
		return "", fmt.Errorf("failed to launch EC2 instance for %s: %w", taskRunName, errors.New("no subnets configured"))
	}

	for _, subnetId := range ec.launchSubnets() {
		triedSubnets = append(triedSubnets, subnetId)
		log.Info("Attempting EC2 launch in subnet", "taskRunName", taskRunName, "subnetId", subnetId)

		instanceID, err := ec.runInstancesInSubnetWithErrorLogging(
			ctx, ec2Client, log, taskRunName, instanceTag, subnetId, additionalInstanceTags)
		// suppress the InsufficientCapacityError to be able to fallback to next the subnet
		if (err != nil && isInsufficientCapacityError(err)) || instanceID != "" {
			return instanceID, err
		}
	}

	return "", fmt.Errorf("failed to launch EC2 instance for %s: insufficient capacity in all configured subnets (tried: %s): %w",
		taskRunName, strings.Join(triedSubnets, ", "), lastCapacityErr)
}

// runInstancesInSubnetWithErrorHandling tries to run the Instance in the given Subnet.
// It logs the returned error before forwarding it the caller.
func (ec AWSEc2DynamicConfig) runInstancesInSubnetWithErrorLogging(
	ctx context.Context,
	ec2Client ec2API,
	log logr.Logger,
	taskRunName, instanceTag, subnetId string,
	additionalInstanceTags map[string]string,
) (cloud.InstanceIdentifier, error) {
	instanceID, err := ec.runInstancesInSubnet(ctx, ec2Client, taskRunName, instanceTag, subnetId, additionalInstanceTags)
	switch {
	case err == nil:
		return instanceID, nil
	case isInsufficientCapacityError(err):
		log.Info("Insufficient capacity in subnet, trying next subnet if configured",
			"taskRunName", taskRunName, "subnetId", subnetId, "error", err)
		return "", err
	case isThrottlingError(err):
		log.Info("AWS API throttled during EC2 launch, deferring to reconciler retry",
			"taskRunName", taskRunName, "subnetId", subnetId, "error", err)
		return "", fmt.Errorf("failed to launch EC2 instance for %s: AWS API throttled: %w", taskRunName, err)
	case errors.Is(err, errNoInstancesCreated) || strings.Contains(err.Error(), "failed to configure EC2 instance"):
		return "", err
	default:
		return "", fmt.Errorf("failed to launch EC2 instance for %s: %w", taskRunName, err)
	}
}

func (ec AWSEc2DynamicConfig) runInstancesInSubnet(
	ctx context.Context,
	ec2Client ec2API,
	taskRunName, instanceTag, subnetId string,
	additionalInstanceTags map[string]string,
) (cloud.InstanceIdentifier, error) {
	launchInput, err := ec.configureInstance(taskRunName, instanceTag, subnetId, additionalInstanceTags)
	if err != nil {
		return "", ec.wrapConfigureInstanceError(taskRunName, err)
	}

	runInstancesOutput, err := ec2Client.RunInstances(ctx, launchInput)
	if err != nil {
		return "", err
	}
	if len(runInstancesOutput.Instances) == 0 {
		return "", errNoInstancesCreated
	}
	return cloud.InstanceIdentifier(*runInstancesOutput.Instances[0].InstanceId), nil
}

func (ec AWSEc2DynamicConfig) wrapConfigureInstanceError(taskRunName string, err error) error {
	if strings.Contains(err.Error(), "MacOS") {
		return fmt.Errorf("missing configuration fields mandatory for MacOS instances: %w", err)
	}
	return fmt.Errorf("failed to configure EC2 instance for %s: %w", taskRunName, err)
}

// configureInstance creates and returns an EC2 instance configuration.
func (ec AWSEc2DynamicConfig) configureInstance(taskRunName, instanceTag, subnetId string, additionalInstanceTags map[string]string) (*ec2.RunInstancesInput, error) {
	// Validate that MacOS-specific fields are either all set or all empty
	tenancySet := ec.Tenancy != ""
	hostResourceGroupSet := ec.HostResourceGroupArn != ""
	licenseConfigSet := ec.LicenseConfigurationArn != ""

	if tenancySet || hostResourceGroupSet || licenseConfigSet {
		if !tenancySet || !hostResourceGroupSet || !licenseConfigSet {
			return nil, errors.New("MacOS dedicated host configuration requires all three fields: Tenancy, HostResourceGroupArn, and LicenseConfigurationArn must be set together")
		}
	}

	var subnet *string
	var securityGroups []string
	var securityGroupIds []string
	var instanceProfile *types.IamInstanceProfileSpecification
	var placement *types.Placement
	var licenseSpecifications []types.LicenseConfigurationRequest

	if subnetId != "" {
		subnet = aws.String(subnetId)
	}
	if ec.SecurityGroup != "" {
		securityGroups = []string{ec.SecurityGroup}
	}
	if ec.SecurityGroupId != "" {
		securityGroupIds = []string{ec.SecurityGroupId}
	}
	if ec.InstanceProfileName != "" || ec.InstanceProfileArn != "" {
		instanceProfile = &types.IamInstanceProfileSpecification{}
		if ec.InstanceProfileName != "" {
			instanceProfile.Name = aws.String(ec.InstanceProfileName)
		}
		if ec.InstanceProfileArn != "" {
			instanceProfile.Arn = aws.String(ec.InstanceProfileArn)
		}
	}

	// Configure placement and license for dedicated hosts (e.g., Mac instances)
	// All three fields (Tenancy, HostResourceGroupArn, LicenseConfigurationArn) are validated to be set together
	if ec.Tenancy != "" {
		placement = &types.Placement{
			Tenancy:              types.Tenancy(ec.Tenancy),
			HostResourceGroupArn: aws.String(ec.HostResourceGroupArn),
		}
		licenseSpecifications = []types.LicenseConfigurationRequest{
			{LicenseConfigurationArn: aws.String(ec.LicenseConfigurationArn)},
		}
	}

	instanceTags := []types.Tag{
		{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
		{Key: aws.String(cloud.InstanceTag), Value: aws.String(instanceTag)},
		{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-" + taskRunName)},
	}
	for k, v := range additionalInstanceTags {
		instanceTags = append(instanceTags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	return &ec2.RunInstancesInput{
		KeyName:            aws.String(ec.KeyName),
		ImageId:            aws.String(ec.Ami), //ARM RHEL
		InstanceType:       types.InstanceType(ec.InstanceType),
		MinCount:           aws.Int32(1),
		MaxCount:           aws.Int32(1),
		EbsOptimized:       aws.Bool(true),
		SecurityGroups:     securityGroups,
		SecurityGroupIds:   securityGroupIds,
		IamInstanceProfile: instanceProfile,
		SubnetId:           subnet,
		UserData:           ec.UserData,
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName:  aws.String("/dev/sda1"),
			VirtualName: aws.String("ephemeral0"),
			Ebs: &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				VolumeSize:          aws.Int32(ec.Disk),
				VolumeType:          types.VolumeTypeGp3,
				Iops:                ec.Iops,
				Throughput:          ec.Throughput,
			},
		}},
		InstanceInitiatedShutdownBehavior: types.ShutdownBehaviorTerminate,
		Placement:                         placement,
		LicenseSpecifications:             licenseSpecifications,
		TagSpecifications: []types.TagSpecification{
			{ResourceType: types.ResourceTypeInstance, Tags: instanceTags},
		},
	}, nil
}

// A SecretCredentialsProvider is a collection of information needed to generate
// AWS credentials. It implements the AWS CredentialsProvider interface.
type SecretCredentialsProvider struct {
	// Name is the name of the the Kubernetes ExternalSecret resource that contains
	// the SSH key's access key ID and secret access key.
	Name string

	// Namespace is the Kubernetes namespace the ExternalSecret resource resides in.
	Namespace string

	// Client is the client (if any) to use to connect to Kubernetes.
	Client client.Client
}

// Retrieve is one of the AWS CredentialsProvider interface's methods that uses external Kubernetes
// secrets or local environment variables to generate AWS credentials.
func (r SecretCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	// Use local environment variables for credentials
	if r.Client == nil {
		// TODO: add a check if the ENVs are empty?
		return aws.Credentials{
			AccessKeyID:     os.Getenv("MULTI_ARCH_ACCESS_KEY"),
			SecretAccessKey: os.Getenv("MULTI_ARCH_SECRET_KEY"),
			SessionToken:    os.Getenv("MULTI_ARCH_SESSION_TOKEN"),
		}, nil
	}

	// Connect to Kubernetes to get credentials info
	s := v1.Secret{}
	nameSpacedSecret := types2.NamespacedName{Name: r.Name, Namespace: r.Namespace}
	err := r.Client.Get(ctx, nameSpacedSecret, &s)
	if err != nil {
		return aws.Credentials{},
			fmt.Errorf("failed to retrieve the secret %v from the Kubernetes client: %w", nameSpacedSecret, err)
	}

	accessKeyID := string(s.Data["access-key-id"])
	secretAccessKey := string(s.Data["secret-access-key"])
	sessionToken := ""
	if sessionTokenData, ok := s.Data["session-token"]; ok {
		sessionToken = string(sessionTokenData)
	}

	return aws.Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}, nil
}
