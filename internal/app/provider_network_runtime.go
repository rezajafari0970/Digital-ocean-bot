package app

import (
	"context"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
)

type providerNetworkRuntime struct {
	Client     *http.Client
	Gateway    *network.Gateway
	Gate       network.AccountGate
	Generation int64
}

// buildProviderNetworkRuntime is the single provider-agnostic network boundary.
// Every provider driver receives its HTTP client through this function.
// Proxy-required accounts fail closed; direct accounts receive an isolated
// per-account direct client.
func (c Container) buildProviderNetworkRuntime(ctx context.Context, cfg AccountConfig) (providerNetworkRuntime, error) {
	if cfg.Network.Mode == network.RouteProxyRequired {
		if cfg.Proxy == nil {
			return providerNetworkRuntime{}, ErrNetworkNotReady
		}
		generation := int64(1)
		if c.DB != nil {
			state, allowed, err := (proxycontrol.SQLStore{DB: c.DB}).Acquire(
				ctx, cfg.ID, cfg.Proxy.ID, cfg.Provider, time.Now().UTC(), 30*time.Second,
			)
			if err != nil {
				return providerNetworkRuntime{}, err
			}
			if !allowed {
				return providerNetworkRuntime{}, network.ErrProxyCircuitOpen
			}
			generation = state.Generation
		}
		password := []byte(nil)
		var err error
		if cfg.ProxySecretRef != "" {
			password, err = c.Secrets.GetProxy(ctx, cfg.Proxy.ID, cfg.ProxySecretRef)
			if err != nil {
				return providerNetworkRuntime{}, err
			}
			defer wipe(password)
		}
		gateway, err := network.NewProxyGateway(
			cfg.ID,
			*cfg.Proxy,
			network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(password)},
		)
		if err != nil {
			return providerNetworkRuntime{}, err
		}
		gate := network.AccountGate{
			Profile: cfg.Network,
			Proxy:   cfg.Proxy,
			Health:  network.HealthState{Status: cfg.Proxy.Status},
		}
		return providerNetworkRuntime{Client: gateway.Client, Gateway: gateway, Gate: gate, Generation: generation}, nil
	}

	bundle, err := network.NewIsolatedDirectClient(cfg.ID)
	if err != nil {
		return providerNetworkRuntime{}, err
	}
	return providerNetworkRuntime{
		Client: bundle.Client,
		Gate:   network.AccountGate{Profile: cfg.Network},
	}, nil
}
