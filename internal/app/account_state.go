package app

import (
	"errors"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

const (
	ProviderStateActive           = "ACTIVE"
	ProviderStateLocked           = "LOCKED"
	ProviderStateTokenInvalid     = "TOKEN_INVALID"
	ProviderStatePermissionDenied = "PERMISSION_DENIED"
	ProviderStateRateLimited      = "RATE_LIMITED"
	ProviderStateTransportError   = "TRANSPORT_ERROR"
	ProviderStateProxyError       = "PROXY_ERROR"
)

func ClassifyAccountProviderError(err error, proxyRequired bool) string {
	if err == nil {
		return ProviderStateActive
	}
	if proxyRequired && (errors.Is(err, network.ErrProxyRequired) || errors.Is(err, network.ErrProxyConfigInvalid) || errors.Is(err, network.ErrAccountNetworkNotReady) || strings.Contains(strings.ToLower(err.Error()), "proxy")) {
		return ProviderStateProxyError
	}
	switch providers.Class(err) {
	case providers.ErrorAccountLocked:
		return ProviderStateLocked
	case providers.ErrorAuthentication:
		return ProviderStateTokenInvalid
	case providers.ErrorPermissionDenied:
		return ProviderStatePermissionDenied
	case providers.ErrorRateLimited:
		return ProviderStateRateLimited
	case providers.ErrorTransport, providers.ErrorUnavailable, providers.ErrorAmbiguousOutcome:
		return ProviderStateTransportError
	}
	return ProviderStateTransportError
}

func ProviderStateRuntimeStatus(state string) string {
	if state == ProviderStateActive {
		return "READY"
	}
	return "PROVIDER_" + state
}

func IsProviderObservationError(state string) bool {
	return state == ProviderStateTransportError || state == ProviderStateProxyError
}
func ProviderProbeInterval(state string) time.Duration {
	switch state {
	case ProviderStateLocked:
		return 45 * time.Second
	case ProviderStateTokenInvalid, ProviderStatePermissionDenied:
		return 5 * time.Minute
	case ProviderStateRateLimited:
		return 2 * time.Minute
	default:
		return 0
	}
}
