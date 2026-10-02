package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
)

func (c Container) acquireProxyKeeperLock(ctx context.Context, accountID string) (*sql.Conn, bool, error) {
	if c.DB == nil {
		return nil, false, ErrNetworkNotReady
	}
	conn, err := c.DB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, "proxy-keeper:"+accountID).Scan(&locked); err != nil {
		conn.Close()
		return nil, false, err
	}
	if !locked {
		conn.Close()
		return nil, false, nil
	}
	return conn, true, nil
}

func releaseProxyKeeperLock(conn *sql.Conn, accountID string) {
	if conn == nil {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, "proxy-keeper:"+accountID)
}

// MaintainProxyControlPlane is the provider-agnostic proxy keeper.
// A PostgreSQL session advisory lock serializes the full load/recovery/save
// sequence per account, including across accidental duplicate worker processes.
func (c Container) MaintainProxyControlPlane(ctx context.Context, accountID string) error {
	conn, locked, err := c.acquireProxyKeeperLock(ctx, accountID)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer releaseProxyKeeperLock(conn, accountID)

	if err := c.ensureActiveAccountProxy(ctx, accountID); err != nil {
		return err
	}

	var provider, mode, proxyID string
	if err := c.DB.QueryRowContext(ctx, `
SELECT a.provider,np.mode,COALESCE(np.proxy_id::text,'')
FROM accounts a
JOIN network_profiles np ON np.account_id=a.id
WHERE a.id=$1 AND a.enabled=true
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
