package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// mockEC2Client satisfies the ec2API interface for testing.
// Output/Err fields control return values; Input fields capture calls.
type mockEC2Client struct {
	RunInstancesOutput      *ec2.RunInstancesOutput
	RunInstancesErr         error
	RunInstancesErrs        []error
	RunInstancesInputs      []*ec2.RunInstancesInput
	runInstancesCallCount   int
	DescribeInstancesOutput *ec2.DescribeInstancesOutput
	DescribeInstancesErr    error
	DescribeInstancesInput  *ec2.DescribeInstancesInput
	TerminateInstancesErr   error
}

func (m *mockEC2Client) RunInstances(_ context.Context, input *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	m.RunInstancesInputs = append(m.RunInstancesInputs, input)
	idx := m.runInstancesCallCount
	m.runInstancesCallCount++

	if len(m.RunInstancesErrs) > 0 {
		if idx < len(m.RunInstancesErrs) {
			if err := m.RunInstancesErrs[idx]; err != nil {
				return nil, err
			}
		}
	} else if m.RunInstancesErr != nil {
		return nil, m.RunInstancesErr
	}
	return m.RunInstancesOutput, nil
}

func (m *mockEC2Client) DescribeInstances(_ context.Context, input *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	m.DescribeInstancesInput = input
	return m.DescribeInstancesOutput, m.DescribeInstancesErr
}

func (m *mockEC2Client) TerminateInstances(_ context.Context, _ *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return nil, m.TerminateInstancesErr
}
