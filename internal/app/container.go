package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net/http"
)

var ErrNetworkNotReady = errors.New("account network not ready")
var ErrProviderComputeUnsupported = errors.New("provider compute capability unavailable")

type Container struct {
	DB        *sql.DB
	Secrets   *secrets.Store
	Accounts  Repository
	Providers *providers.Registry
}
type AccountRuntime struct {
	Config              AccountConfig
	Cell                *accounts.Cell
	Gateway             *network.Gateway
	Driver              providers.Driver
	Operations          jobs.SQLStore
	Gate                network.AccountGate
	TransportGeneration int64
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
	if c.Providers == nil {
		return nil, errors.New("provider registry unavailable")
	}
	return c.Providers, nil
}
func (c Container) openDriver(ctx context.Context, cfg AccountConfig, client *http.Client) (providers.Driver, error) {
	r, err := c.providerRegistry()
	if err != nil {
		return nil, err
	}
	return r.Open(ctx, cfg.Provider, providers.OpenRequest{AccountID: cfg.ID, HTTPClient: client, Credentials: accountCredentialSource{store: c.Secrets, accountID: cfg.ID, ref: cfg.SecretRef}})
}

func computeDriver(rt AccountRuntime) (providers.ComputeDriver, error) {
	d, ok := rt.Driver.(providers.ComputeDriver)
	if !ok || d == nil {
		return nil, ErrProviderComputeUnsupported
	}
	return d, nil
}

func (c Container) Runtime(ctx context.Context, accountID string) (AccountRuntime, error) {
	if err := c.ensureActiveAccountProxy(ctx, accountID); err != nil {
		return AccountRuntime{}, err
	}
	cfg, err := c.Accounts.Account(ctx, accountID)
	if err != nil {
		return AccountRuntime{}, err
	}
	cell := accounts.NewCellManager().Register(accountID)
	netrt, err := c.buildProviderNetworkRuntime(ctx, cfg)
	if err != nil {
		return AccountRuntime{}, err
	}
	driver, err := c.openDriver(ctx, cfg, netrt.Client)
	if err != nil {
		netrt.CloseIdleConnections()
		return AccountRuntime{}, err
	}
	return AccountRuntime{
		Config:              cfg,
		Cell:                cell,
		Gateway:             netrt.Gateway,
		Driver:              driver,
		Operations:          jobs.SQLStore{DB: c.DB},
		Gate:                netrt.Gate,
		TransportGeneration: netrt.Generation,
	}, nil
}
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
