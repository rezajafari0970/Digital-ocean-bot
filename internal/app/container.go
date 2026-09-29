package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http"
)

var ErrNetworkNotReady = errors.New("account network not ready")

type Container struct {
	DB        *sql.DB
	Secrets   *secrets.Store
	Accounts  Repository
	Providers *providers.Registry
}
type AccountRuntime struct {
	Config     AccountConfig
	Cell       *accounts.Cell
	Gateway    *network.Gateway
	Provider   *digitalocean.Client // compatibility path
	Driver     providers.Driver
	Operations jobs.SQLStore
	Gate       network.AccountGate
}

type accountCredentialSource struct {
	store     *secrets.Store
	accountID string
	ref       string
}

func (s accountCredentialSource) Get(ctx context.Context) ([]byte, error) {
	return s.store.Get(ctx, s.accountID, s.ref)
}

func (c Container) providerRegistry() (*providers.Registry, error) {
	if c.Providers != nil {
		return c.Providers, nil
	}
	r := providers.NewRegistry()
	if err := r.Register(digitalocean.Factory{}); err != nil {
		return nil, err
	}
	return r, nil
}
func (c Container) openDriver(ctx context.Context, cfg AccountConfig, client *http.Client) (providers.Driver, error) {
	r, err := c.providerRegistry()
	if err != nil {
		return nil, err
	}
	return r.Open(ctx, cfg.Provider, providers.OpenRequest{AccountID: cfg.ID, HTTPClient: client, Credentials: accountCredentialSource{store: c.Secrets, accountID: cfg.ID, ref: cfg.SecretRef}})
}

func (c Container) Runtime(ctx context.Context, accountID string) (AccountRuntime, error) {
	cfg, err := c.Accounts.Account(ctx, accountID)
	if err != nil {
		return AccountRuntime{}, err
	}
	cell := accounts.NewCellManager().Register(accountID)
	if cfg.Network.Mode == network.RouteProxyRequired {
		if cfg.Proxy == nil {
			return AccountRuntime{}, ErrNetworkNotReady
		}
		password := []byte(nil)
		if cfg.ProxySecretRef != "" {
			password, err = c.Secrets.GetProxy(ctx, cfg.Proxy.ID, cfg.ProxySecretRef)
			if err != nil {
				return AccountRuntime{}, err
			}
			defer wipe(password)
		}
		gateway, err := network.NewProxyGateway(accountID, *cfg.Proxy, network.ProxyCredentials{Username: cfg.ProxyUsername, Password: string(password)})
		if err != nil {
			return AccountRuntime{}, err
		}
		gate := network.AccountGate{Profile: cfg.Network, Proxy: cfg.Proxy, Health: network.HealthState{Status: cfg.Proxy.Status}}
		provider, err := digitalocean.NewClient(cell.Context, cfg.SecretRef, c.Secrets, gateway.Client)
		if err != nil {
			return AccountRuntime{}, err
		}
		driver, err := c.openDriver(ctx, cfg, gateway.Client)
		if err != nil {
			return AccountRuntime{}, err
		}
		return AccountRuntime{Config: cfg, Cell: cell, Gateway: gateway, Provider: provider, Driver: driver, Operations: jobs.SQLStore{DB: c.DB}, Gate: gate}, nil
	}
	bundle, err := network.NewIsolatedDirectClient(accountID)
	if err != nil {
		return AccountRuntime{}, err
	}
	provider, err := digitalocean.NewClient(cell.Context, cfg.SecretRef, c.Secrets, bundle.Client)
	if err != nil {
		bundle.CloseIdleConnections()
		return AccountRuntime{}, err
	}
	driver, err := c.openDriver(ctx, cfg, bundle.Client)
	if err != nil {
		bundle.CloseIdleConnections()
		return AccountRuntime{}, err
	}
	gate := network.AccountGate{Profile: cfg.Network}
	return AccountRuntime{Config: cfg, Cell: cell, Provider: provider, Driver: driver, Operations: jobs.SQLStore{DB: c.DB}, Gate: gate}, nil
}
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
