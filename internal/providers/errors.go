package providers

import (
	"errors"
	"fmt"
	"time"
)

type ErrorClass string

const (
	ErrorUnknown          ErrorClass = "unknown"
	ErrorAuthentication   ErrorClass = "authentication"
	ErrorPermissionDenied ErrorClass = "permission_denied"
	ErrorAccountLocked    ErrorClass = "account_locked"
	ErrorRateLimited      ErrorClass = "rate_limited"
	ErrorCapacity         ErrorClass = "capacity"
	ErrorRegionCapacity   ErrorClass = "region_capacity"
	ErrorImageUnavailable ErrorClass = "image_unavailable"
	ErrorNotFound         ErrorClass = "not_found"
	ErrorInvalidRequest   ErrorClass = "invalid_request"
	ErrorTransport        ErrorClass = "transport"
	ErrorUnavailable      ErrorClass = "unavailable"
	ErrorAmbiguousOutcome ErrorClass = "ambiguous_outcome"
)

type Error struct {
	Class      ErrorClass
	Operation  string
	StatusCode int
	RetryAfter time.Duration
	Message    string
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("provider %s: %s", e.Class, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("provider %s: %v", e.Class, e.Cause)
	}
	return "provider " + string(e.Class)
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func Class(err error) ErrorClass {
	if err == nil {
		return ""
	}
	var pe *Error
	if errors.As(err, &pe) {
		return pe.Class
	}
	return ErrorUnknown
}
func IsClass(err error, class ErrorClass) bool { return Class(err) == class }
func IsRetryable(err error) bool {
	switch Class(err) {
	case ErrorRateLimited, ErrorTransport, ErrorUnavailable, ErrorAmbiguousOutcome:
		return true
	default:
		return false
	}
}
