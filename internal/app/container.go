package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

var ErrNetworkNotReady = errors.New("account network not ready")

type Container struct {
	DB       *sql.DB
	Secrets  *secrets.Store
	Accounts Repository
}
type AccountRuntime struct {
	Config     AccountConfig
	Cell       *accounts.Cell
	Gateway    *network.Gateway
	Provider   *digitalocean.Client
	Operations jobs.SQLStore
	Gate       network.AccountGate
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
		return AccountRuntime{Config: cfg, Cell: cell, Gateway: gateway, Provider: provider, Operations: jobs.SQLStore{DB: c.DB}, Gate: gate}, nil
	}
	return AccountRuntime{}, ErrNetworkNotReady
}
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
