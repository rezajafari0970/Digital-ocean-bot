package network

import "errors"

var (
	ErrProxyRequired    = errors.New("proxy required: direct route denied")
	ErrProxyUnavailable = errors.New("proxy required but unavailable")
)

func ValidateRoute(profile Profile, proxy *Proxy) error {
	if profile.Mode == RouteDirect {
		return nil
	}
	if profile.Mode != RouteProxyRequired {
		return ErrProxyRequired
	}
	if profile.ProxyID == nil || proxy == nil {
		return ErrProxyUnavailable
	}
	if proxy.Status != StatusHealthy {
		return ErrProxyUnavailable
	}
	return nil
}
