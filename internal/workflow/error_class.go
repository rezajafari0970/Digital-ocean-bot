package workflow

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"strings"
)

type ErrorClass string

const (
	ErrorRetryable  ErrorClass = "retryable"
	ErrorPermanent  ErrorClass = "permanent"
	ErrorCapacity   ErrorClass = "capacity"
	ErrorAuth       ErrorClass = "auth"
	ErrorTimeout    ErrorClass = "timeout"
	ErrorDependency ErrorClass = "dependency"
	ErrorUnknown    ErrorClass = "unknown"
)

func ClassifyStepError(step string, err error) ErrorClass {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorTimeout
	}
	if digitalocean.IsCapacityError(err) {
		return ErrorCapacity
	}
	var h digitalocean.HTTPError
	if errors.As(err, &h) {
		if h.Status == 401 || h.Status == 403 {
			return ErrorAuth
		}
		switch digitalocean.ClassifyError(err) {
		case resilience.Retryable, resilience.RateLimited:
			return ErrorRetryable
		case resilience.Permanent:
			return ErrorPermanent
		}
	}
	if errors.Is(err, secrets.ErrSecretNotFound) {
		return ErrorDependency
	}
	if errors.Is(err, provisioning.ErrSSHNotReady) || errors.Is(err, provisioning.ErrStepRetryDeferred) {
		return ErrorRetryable
	}
	if errors.Is(err, provisioning.ErrStepRetryLimit) || errors.Is(err, provisioning.ErrStepTerminal) || errors.Is(err, provisioning.ErrInvalidPlan) {
		return ErrorPermanent
	}
	if errors.Is(err, provisioning.ErrSSHCommand) {
		if step == "provision" {
			// Inner provisioning owns retry/backoff for remote commands.
			return ErrorRetryable
		}
		return ErrorPermanent
	}
	if errors.Is(err, ErrRuntimeConfig) || errors.Is(err, ErrStepUnavailable) {
		return ErrorDependency
	}
	m := strings.ToLower(err.Error())
	if strings.Contains(m, "authenticate") || strings.Contains(m, "unauthorized") {
		return ErrorAuth
	}
	return ErrorUnknown
}
func RetryableClass(c ErrorClass) bool {
	return c == ErrorRetryable || c == ErrorTimeout || c == ErrorCapacity || c == ErrorUnknown
}
