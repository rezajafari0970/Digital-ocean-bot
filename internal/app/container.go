package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

var ErrNetworkNotReady = errors.New("account network not ready")
var ErrProviderComputeUnsupported = errors.New("provider compute capability unavailable")
var ErrRuntimeGenerationObsolete = runtimeGenerationObsoleteError{}

type runtimeGenerationObsoleteError struct{}

func (runtimeGenerationObsoleteError) Error() string {
	return "account runtime proxy generation is obsolete or unavailable"
}

func (runtimeGenerationObsoleteError) RetryDelay() time.Duration {
	return time.Second
}

type Container struct {
	DB        *sql.DB
	Secrets   *secrets.Store
	Accounts  Repository
	Providers *providers.Registry
}
type runtimeGenerationStore interface {
	CurrentGeneration(ctx context.Context, accountID, proxyID, provider string) (int64, bool, error)
}

type accountRuntimeStatusStore interface {
	MarkStaleGeneration(ctx context.Context, accountID string) error
	ClearStaleGeneration(ctx context.Context, accountID string) error
}

const (
	accountRuntimeStatusReady                 = "READY"
	accountRuntimeStatusStaleGeneration       = "STALE_GENERATION"
	accountRuntimeStatusStaleGenerationDetail = "Proxy generation was superseded by a newer runtime generation."
)

type accountRuntimeStatusSQLStore struct {
	DB *sql.DB
}

func (s accountRuntimeStatusSQLStore) MarkStaleGeneration(ctx context.Context, accountID string) error {
	_, err := s.DB.ExecContext(ctx, `
UPDATE accounts
SET runtime_status = $2, runtime_status_detail = $3
WHERE id = $1 AND runtime_status IN ($4, $5)`,
		accountRuntimeStatusStaleGeneration,
		accountRuntimeStatusStaleGenerationDetail,
		accountID,
		accountRuntimeStatusReady,
		accountRuntimeStatusStaleGeneration,
	)
	return err
}

func (s accountRuntimeStatusSQLStore) ClearStaleGeneration(ctx context.Context, accountID string) error {
	_, err := s.DB.ExecContext(ctx, `
UPDATE accounts
SET runtime_status = ?, runtime_status_detail = NULL
WHERE id = ? AND runtime_status = ?`,
		accountRuntimeStatusReady,
		accountID,
		accountRuntimeStatusStaleGeneration,
	)
	return err
}

type AccountRuntime struct {
	Config              AccountConfig
	Cell                *accounts.Cell
	Gateway             *network.Gateway
	Driver              providers.Driver
	Operations          jobs.SQLStore
	Gate                network.AccountGate
	TransportGeneration int64
	GenerationStore     runtimeGenerationStore
	runtimeStatusStore  accountRuntimeStatusStore
}

func (rt AccountRuntime) rejectMutationGeneration(ctx context.Context, cause error) error {
	if rt.runtimeStatusStore != nil {
		if err := rt.runtimeStatusStore.MarkStaleGeneration(ctx, rt.Config.ID); err != nil {
			return fmt.Errorf("%w: stale runtime status persistence failed: %v", cause, err)
		}
	}
	return cause
}

func (rt AccountRuntime) CheckMutationGeneration(ctx context.Context) error {
	if rt.TransportGeneration == 0 && rt.Config.Network.Mode != network.RouteProxyRequired {
		return nil
	}
	if rt.TransportGeneration < 1 || rt.Config.Network.ProxyID == nil || *rt.Config.Network.ProxyID == "" || rt.GenerationStore == nil {
		return rt.rejectMutationGeneration(ctx, ErrRuntimeGenerationObsolete)
	}
	generation, found, err := rt.GenerationStore.CurrentGeneration(ctx, rt.Config.ID, *rt.Config.Network.ProxyID, rt.Config.Provider)
	if err != nil {
		return rt.rejectMutationGeneration(ctx, fmt.Errorf("%w: %v", ErrRuntimeGenerationObsolete, err))
	}
	if !found {
		return rt.rejectMutationGeneration(ctx, fmt.Errorf("%w: proxy runtime state missing", ErrRuntimeGenerationObsolete))
	}
	if generation != rt.TransportGeneration {
		return rt.rejectMutationGeneration(ctx, fmt.Errorf("%w: runtime generation %d, current generation %d", ErrRuntimeGenerationObsolete, rt.TransportGeneration, generation))
	}
	if rt.runtimeStatusStore != nil {
		if err := rt.runtimeStatusStore.ClearStaleGeneration(ctx, rt.Config.ID); err != nil {
			return fmt.Errorf("clear stale runtime status: %w", err)
		}
	}
	return nil
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
		GenerationStore:     proxycontrol.SQLStore{DB: c.DB},
		runtimeStatusStore:  accountRuntimeStatusSQLStore{DB: c.DB},
	}, nil
}
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
