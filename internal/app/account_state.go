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
	ProviderStateBillingBlocked   = "BILLING_BLOCKED"
	ProviderStateRateLimited      = "RATE_LIMITED"
	ProviderStateTransportError   = "TRANSPORT_ERROR"
	ProviderStateProxyError       = "PROXY_ERROR"
)

func ClassifyAccountProviderError(err error, proxyRequired bool) string {
	if err == nil {
		return ProviderStateActive
	}
	if proxyRequired && (errors.Is(err, network.ErrProxyRequired) ||
		errors.Is(err, network.ErrProxyUnavailable) ||
		errors.Is(err, network.ErrProxyConfigInvalid) ||
		errors.Is(err, network.ErrAccountNetworkNotReady) ||
		errors.Is(err, network.ErrProxyCircuitOpen) ||
		errors.Is(err, network.ErrProxyAuth) ||
		errors.Is(err, network.ErrProxyDNS) ||
		errors.Is(err, network.ErrProxyConnect) ||
		errors.Is(err, network.ErrIPv6Prohibited) ||
		errors.Is(err, network.ErrIPv4Required) ||
		errors.Is(err, network.ErrUnexpectedExitIP) ||
		errors.Is(err, network.ErrServerIPLeak) ||
		errors.Is(err, network.ErrEgressChanged) ||
		errors.Is(err, network.ErrAccountContextMismatch)) {
		return ProviderStateProxyError
	}
	switch providers.Class(err) {
	case providers.ErrorBilling:
		return ProviderStateBillingBlocked
	case providers.ErrorAccountLocked:
		return ProviderStateLocked
	case providers.ErrorAuthentication:
		return ProviderStateTokenInvalid
	case providers.ErrorPermissionDenied:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "outstanding balance") || strings.Contains(msg, "billing profile") {
			return ProviderStateBillingBlocked
		}
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
	case ProviderStateTokenInvalid, ProviderStatePermissionDenied, ProviderStateBillingBlocked:
		return 5 * time.Minute
	case ProviderStateRateLimited:
		return 2 * time.Minute
	case ProviderStateTransportError, ProviderStateProxyError:
		return 15 * time.Second
	default:
		return 0
	}
}
