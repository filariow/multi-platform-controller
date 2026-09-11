package aws

import (
	"errors"

	smithy "github.com/aws/smithy-go"
)

func isInsufficientCapacityError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "InsufficientInstanceCapacity", "InsufficientCapacity", "InsufficientDedicatedHostCapacity":
		return true
	default:
		return false
	}
}

func isThrottlingError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "RequestLimitExceeded", "Throttling", "ThrottlingException", "TooManyRequestsException":
		return true
	default:
		return false
	}
}
