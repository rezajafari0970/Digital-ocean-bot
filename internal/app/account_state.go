package app

import (
	"errors"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
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
	var h digitalocean.HTTPError
	if errors.As(err, &h) {
		msg := strings.ToLower(h.Code + " " + h.Message)
		if h.Status == 422 && strings.Contains(msg, "lock") && strings.Contains(msg, "account") {
			return ProviderStateLocked
		}
		switch h.Status {
		case 401:
			return ProviderStateTokenInvalid
		case 403:
			return ProviderStatePermissionDenied
		case 429:
			return ProviderStateRateLimited
		}
	}
	if proxyRequired && (errors.Is(err, network.ErrProxyRequired) || errors.Is(err, network.ErrProxyConfigInvalid) || errors.Is(err, network.ErrAccountNetworkNotReady) || strings.Contains(strings.ToLower(err.Error()), "proxy")) {
		return ProviderStateProxyError
	}
	return ProviderStateTransportError
}

func ProviderStateRuntimeStatus(state string) string {
	if state == ProviderStateActive {
		return "READY"
	}
	return "PROVIDER_" + state
}
