package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

// Only static metadata is cached. Every caller still observes account authority
// and capacity fresh. The shared lease collapses API/worker catalog requests.
func (c Container) CachedProviderCatalog(ctx context.Context, rt AccountRuntime) (providers.Catalog, error) {
	cr, ok := rt.Driver.(providers.CatalogReader)
	if !ok {
		return providers.Catalog{}, ErrProviderComputeUnsupported
	}
	enabled, err := c.Economy.Enabled(ctx, rt.Config.ID)
	if err != nil {
		return providers.Catalog{}, err
	}
	_, versioned := rt.Driver.(providers.VersionedCatalogReader)
	if !enabled && !versioned {
		return cr.Catalog(ctx)
	}
	var tx *sql.Tx
	for {
		tx, err = c.DB.BeginTx(ctx, nil)
		if err != nil {
			return providers.Catalog{}, err
		}
		var locked bool
		err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "provider-catalog:"+rt.Config.ID).Scan(&locked)
		if err != nil {
			tx.Rollback()
			return providers.Catalog{}, err
		}
		if locked {
			break
		}
		tx.Rollback()
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return providers.Catalog{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer tx.Rollback()
	revision := func() (string, error) { return catalogScope(ctx, tx, rt) }

	scope, err := revision()
	if err != nil {
		return providers.Catalog{}, err
	}
	var raw []byte
	var catalog providers.Catalog
	if err = tx.QueryRowContext(ctx, `SELECT catalog FROM provider_catalog_cache WHERE account_id=$1 AND scope_revision=$2 AND refreshed_at<=now() AND refreshed_at>now()-($3*interval '1 second')`, rt.Config.ID, scope, int(network.EconomyCatalogTTL/time.Second)).Scan(&raw); err == nil {
		if json.Unmarshal(raw, &catalog) == nil {
			return catalog, nil
		}
	}
	catalog, err = cr.Catalog(ctx)
	if err != nil {
		return providers.Catalog{}, err
	}
	after, err := revision()
	if err != nil {
		return providers.Catalog{}, err
	}
	if after != scope {
		return providers.Catalog{}, fmt.Errorf("catalog credential scope changed")
	}
	if versioned {
		catalog.CacheVersion = rt.Driver.(providers.VersionedCatalogReader).CatalogCacheVersion()
	}
	catalog.FetchedAt = time.Now().UTC()
	raw, err = json.Marshal(catalog)
	if err != nil {
		return providers.Catalog{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO provider_catalog_cache(account_id,catalog,refreshed_at,scope_revision) VALUES($1,$2,now(),$3) ON CONFLICT(account_id) DO UPDATE SET catalog=EXCLUDED.catalog,refreshed_at=now(),scope_revision=EXCLUDED.scope_revision`, rt.Config.ID, raw, scope)
	if err != nil {
		return providers.Catalog{}, err
	}
	if err = tx.Commit(); err != nil {
		return providers.Catalog{}, err
	}
	return catalog, nil
}

func (c Container) ObserveProvider(ctx context.Context, rt AccountRuntime) (providers.Observation, error) {
	reader, ok := rt.Driver.(providers.ObservationReader)
	if !ok {
		return providers.Observation{}, ErrProviderComputeUnsupported
	}
	// Versioned catalog collection runs independently in SyncCatalogs. Health
	// must neither invoke optional provider metadata nor re-date an old catalog.
	if _, versioned := rt.Driver.(providers.VersionedCatalogReader); versioned {
		if fast, ok := rt.Driver.(providers.FastObservationReader); ok {
			obs, err := fast.ObserveFast(ctx)
			if err != nil {
				return obs, err
			}
			obs.Catalog, err = c.readVersionedCatalog(ctx, rt)
			if err == sql.ErrNoRows {
				err = nil
			}
			return obs, err
		}
	}
	enabled, err := c.Economy.Enabled(ctx, rt.Config.ID)
	if err != nil {
		return providers.Observation{}, err
	}
	if enabled {
		if fast, ok := rt.Driver.(providers.FastObservationReader); ok {
			obs, err := fast.ObserveFast(ctx)
			if err != nil {
				return obs, err
			}
			obs.Catalog, err = c.CachedProviderCatalog(ctx, rt)
			return obs, err
		}
	}
	// Preserve pre-economy behavior for controls and rollback.
	if rt.Config.Provider == "vultr" || rt.Config.Provider == "upcloud" || rt.Config.Provider == "linode" || rt.Config.Provider == "aws" || rt.Config.Provider == "ovh" || rt.Config.Provider == "kamatera" {
		var previous providers.Observation
		var raw []byte
		if fast, ok := rt.Driver.(providers.FastObservationReader); ok && c.DB != nil {
			if err := c.DB.QueryRowContext(ctx, `SELECT canonical FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1`, rt.Config.ID).Scan(&raw); err == nil && json.Unmarshal(raw, &previous) == nil {
				fastCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				defer cancel()
				obs, err := fast.ObserveFast(fastCtx)
				obs.Catalog = previous.Catalog
				return obs, err
			}
		}
	}
	return reader.Observe(ctx)
}

type catalogQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func catalogScope(ctx context.Context, q catalogQuery, rt AccountRuntime) (string, error) {
	var stamp string
	err := q.QueryRowContext(ctx, "SELECT COALESCE(max(updated_at)::text,'missing') FROM secrets WHERE account_id=$1 AND id=$2", rt.Config.ID, rt.Config.SecretRef).Scan(&stamp)
	scope := rt.Config.Provider + "/" + rt.Config.SecretRef + "/" + stamp
	if d, ok := rt.Driver.(providers.VersionedCatalogReader); ok {
		scope += "/catalog:" + d.CatalogCacheVersion()
	}
	return scope, err
}
func (c Container) readVersionedCatalog(ctx context.Context, rt AccountRuntime) (providers.Catalog, error) {
	var catalog providers.Catalog
	scope, err := catalogScope(ctx, c.DB, rt)
	if err != nil {
		return catalog, err
	}
	var raw []byte
	err = c.DB.QueryRowContext(ctx, "SELECT catalog FROM provider_catalog_cache WHERE account_id=$1 AND scope_revision=$2 AND refreshed_at<=now() AND refreshed_at>now()-($3*interval '1 second')", rt.Config.ID, scope, int(network.EconomyCatalogTTL/time.Second)).Scan(&raw)
	if err != nil {
		return catalog, err
	}
	if json.Unmarshal(raw, &catalog) != nil {
		return providers.Catalog{}, sql.ErrNoRows
	}
	return catalog, nil
}
