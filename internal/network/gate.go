package network

import "errors"

var ErrAccountNetworkNotReady = errors.New("account network not ready")
var ErrProxyCircuitOpen = errors.New("proxy circuit open")

type AccountGate struct {
	Profile Profile
	Proxy   *Proxy
	Health  HealthState
}

func (g AccountGate) AllowMutation() error {
	if err := ValidateRoute(g.Profile, g.Proxy); err != nil {
		return ErrAccountNetworkNotReady
	}
	if g.Profile.Mode == RouteProxyRequired && g.Health.Status != StatusHealthy {
		return ErrAccountNetworkNotReady
	}
	return nil
}
