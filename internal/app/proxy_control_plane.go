package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
)

func (c Container) acquireProxyKeeperLock(ctx context.Context, accountID string) (*sql.Tx, bool, error) {
	if c.DB == nil {
		return nil, false, ErrNetworkNotReady
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "proxy-keeper:"+accountID).Scan(&locked); err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if !locked {
		_ = tx.Rollback()
		return nil, false, nil
	}
	return tx, true, nil
}

func releaseProxyKeeperLock(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

// MaintainProxyControlPlane is the provider-agnostic proxy keeper.
// A PostgreSQL transaction-scoped advisory lock serializes the full
// load/recovery/save sequence per account, including across accidental
// duplicate worker processes. Transaction scope prevents orphaned locks.
func (c Container) MaintainProxyControlPlane(ctx context.Context, accountID string) error {
	lockTx, locked, err := c.acquireProxyKeeperLock(ctx, accountID)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer releaseProxyKeeperLock(lockTx)

	if err := c.ensureActiveAccountProxy(ctx, accountID); err != nil {
		if errors.Is(err, ErrNetworkNotReady) {
			if diagnosticErr := c.recordBlockedBaseProxy(ctx, accountID); diagnosticErr != nil {
				return errors.Join(err, diagnosticErr)
			}
		}
		return err
	}

	var provider, mode, proxyID string
	if err := c.DB.QueryRowContext(ctx, `
SELECT a.provider,np.mode,COALESCE(np.proxy_id::text,'')
FROM accounts a
JOIN network_profiles np ON np.account_id=a.id
WHERE a.id=$1 AND (a.enabled=true OR a.deletion_requested_at IS NOT NULL)
`, accountID).Scan(&provider, &mode, &proxyID); err != nil {
		return err
	}
	if mode != string(network.RouteProxyRequired) || proxyID == "" {
		return nil
	}

	store := proxycontrol.SQLStore{DB: c.DB}
	state, allowed, err := store.Acquire(ctx, accountID, proxyID, provider, time.Now().UTC(), 20*time.Second)
	if err != nil {
		return err
	}
	if !allowed {
		// Cooldown or another half-open probe lease is active.
		return nil
	}

	beforeSession, beforeIP := proxyIdentitySignature(ctx, c.DB, accountID)
	started := time.Now()
	maintainErr := c.MaintainStickyIdentity(ctx, accountID)
	now := time.Now().UTC()

	switch {
	case maintainErr == nil:
		state = proxycontrol.ApplyHealth(state, network.HealthResult{
			Status:    network.StatusHealthy,
			Latency:   time.Since(started),
			CheckedAt: now,
		}, proxycontrol.DefaultPolicy())
	case errors.Is(maintainErr, ErrIsolationWait), errors.Is(maintainErr, ErrNetworkIdentityCollision), errors.Is(maintainErr, ErrProxyObservationUnavailable):
		// Isolation conflicts and observer outages are not proxy transport failures.
		// Keep them degraded/fail-closed without rotating away from a healthy proxy.
		state.HealthState = network.StatusDegraded
		state.LastCheckedAt = &now
		state.LastErrorClass = "ISOLATION_WAIT"
		if errors.Is(maintainErr, ErrProxyObservationUnavailable) {
			state.LastErrorClass = "OBSERVATION_UNAVAILABLE"
		}
		state.LastErrorDetail = maintainErr.Error()
	default:
		state = proxycontrol.ApplyHealth(state, network.HealthResult{
			Status:    network.StatusDown,
			Latency:   time.Since(started),
			CheckedAt: now,
			Error:     maintainErr.Error(),
		}, proxycontrol.DefaultPolicy())
	}

	afterSession, afterIP := proxyIdentitySignature(ctx, c.DB, accountID)
	if maintainErr == nil && (beforeSession != afterSession || beforeIP != afterIP) {
		state = proxycontrol.BumpGeneration(state)
	}

	if err := store.Save(ctx, state); err != nil {
		return err
	}
	return maintainErr
}

func proxyIdentitySignature(ctx context.Context, db *sql.DB, accountID string) (session, ip string) {
	_ = db.QueryRowContext(ctx, `
SELECT COALESCE(sticky_session,''),COALESCE(host(exit_ip),'')
FROM account_network_identities
WHERE account_id=$1
`, accountID).Scan(&session, &ip)
	return session, ip
}

// recordBlockedBaseProxy records why identity maintenance was skipped. Base
// monitor evidence never substitutes for a successful account identity probe:
// health, circuit, counters, generation and lease fields remain untouched.
func (c Container) recordBlockedBaseProxy(ctx context.Context, accountID string) error {
	_, err := c.DB.ExecContext(ctx, `
INSERT INTO proxy_runtime_state(
 account_id,proxy_id,provider,last_error_class,last_error_detail,last_checked_at,updated_at)
SELECT a.id,p.id,a.provider,
 CASE WHEN p.health_error='PROXY_AUTH_FAILED'
      THEN 'BASE_PROXY_AUTH_FAILED' ELSE 'BASE_PROXY_UNAVAILABLE' END,
 CASE WHEN p.health_error='PROXY_AUTH_FAILED'
      THEN 'base proxy authentication failed; identity check skipped'
      ELSE 'no healthy configured proxy route; identity check skipped' END,
 p.last_checked_at,now()
FROM accounts a
JOIN network_profiles np ON np.account_id=a.id
JOIN proxies p ON p.id=np.proxy_id
JOIN account_proxy_pool ap ON ap.account_id=a.id AND ap.proxy_id=p.id AND ap.enabled=true
WHERE a.id=$1 AND (a.enabled=true OR a.deletion_requested_at IS NOT NULL)
 AND np.mode='proxy_required' AND p.status<>'healthy' AND p.last_checked_at IS NOT NULL
 AND NOT EXISTS (
  SELECT 1 FROM account_proxy_pool candidate JOIN proxies healthy ON healthy.id=candidate.proxy_id
  WHERE candidate.account_id=a.id AND candidate.enabled=true AND healthy.status='healthy'
 )
ON CONFLICT(account_id,proxy_id,provider) DO UPDATE SET
 last_error_class=EXCLUDED.last_error_class,last_error_detail=EXCLUDED.last_error_detail,
 last_checked_at=EXCLUDED.last_checked_at,updated_at=now()
WHERE proxy_runtime_state.last_checked_at IS NULL
   OR proxy_runtime_state.last_checked_at<EXCLUDED.last_checked_at
`, accountID)
	return err
}
