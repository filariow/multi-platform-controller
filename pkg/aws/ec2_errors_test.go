package aws

import (
	"errors"

	smithy "github.com/aws/smithy-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EC2 error helpers", func() {
	DescribeTable("isInsufficientCapacityError",
		func(err error, expected bool) {
			Expect(isInsufficientCapacityError(err)).To(Equal(expected))
		},
		Entry("InsufficientInstanceCapacity", &smithy.GenericAPIError{Code: "InsufficientInstanceCapacity", Message: "no capacity"}, true),
		Entry("InsufficientCapacity", &smithy.GenericAPIError{Code: "InsufficientCapacity", Message: "no capacity"}, true),
		Entry("InsufficientDedicatedHostCapacity", &smithy.GenericAPIError{Code: "InsufficientDedicatedHostCapacity", Message: "no capacity"}, true),
		Entry("VcpuLimitExceeded", &smithy.GenericAPIError{Code: "VcpuLimitExceeded", Message: "limit"}, false),
		Entry("generic error", errors.New("ec2 launch failed"), false),
	)

	DescribeTable("isThrottlingError",
		func(err error, expected bool) {
			Expect(isThrottlingError(err)).To(Equal(expected))
		},
		Entry("RequestLimitExceeded", &smithy.GenericAPIError{Code: "RequestLimitExceeded", Message: "limit"}, true),
		Entry("Throttling", &smithy.GenericAPIError{Code: "Throttling", Message: "rate exceeded"}, true),
		Entry("ThrottlingException", &smithy.GenericAPIError{Code: "ThrottlingException", Message: "rate exceeded"}, true),
		Entry("TooManyRequestsException", &smithy.GenericAPIError{Code: "TooManyRequestsException", Message: "slow down"}, true),
		Entry("InsufficientInstanceCapacity", &smithy.GenericAPIError{Code: "InsufficientInstanceCapacity", Message: "no capacity"}, false),
		Entry("generic error", errors.New("ec2 launch failed"), false),
	)
})
