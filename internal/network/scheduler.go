package network

import (
	"context"
	"time"
)

type HealthTarget struct {
	Proxy          Proxy
	Credentials    ProxyCredentials
	ExpectedExitIP string
	ServerPublicIP string
	Endpoint       string
	State          HealthState
}

type HealthScheduler struct {
	Store   HealthStore
	Policy  HealthPolicy
	Timeout time.Duration
}

func (s HealthScheduler) Check(ctx context.Context, accountID string, target HealthTarget) (HealthState, error) {
	gateway, err := NewProxyGateway(accountID, target.Proxy, target.Credentials)
	if err != nil {
		return target.State, err
	}
	defer gateway.CloseIdleConnections()
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := CheckProxy(checkCtx, gateway, target.Endpoint, target.ExpectedExitIP, target.ServerPublicIP)
	state := target.State.Apply(result, s.Policy)
	if s.Store != nil {
		if err := s.Store.Save(ctx, target.Proxy.ID, state); err != nil {
			return state, err
		}
	}
	return state, nil
}
