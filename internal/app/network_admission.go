package app

import (
	"context"
	"database/sql"
	"errors"
	"net"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

// Stable, credential-free reasons survive provider classification and appear in
// the panel. Unwrap retains the existing fail-closed admission contract.
type networkReadinessError string

func (e networkReadinessError) Error() string {
	return ErrNetworkNotReady.Error() + ": " + e.guidance()
}
func (e networkReadinessError) Unwrap() error { return ErrNetworkNotReady }
func (e networkReadinessError) guidance() string {
	switch e {
	case "ROUTE_MISSING":
		return "account route is missing"
	case "COUNTRY_POLICY_UNVERIFIED":
		return "selected country policy has not been verified"
	case "ROUTE_MODE_INVALID":
		return "account route mode is invalid"
	case "BASE_PROXY_UNHEALTHY":
		return "configured proxy endpoint is not healthy"
	case "IDENTITY_IPV4_MISSING":
		return "verified account exit IPv4 is missing"
	case "IDENTITY_UNVERIFIED":
		return "account exit identity is unverified or recovering"
	case "IDENTITY_HEALTH_STALE":
		return "account exit identity health check has expired"
	case "BASE_PROXY_HEALTH_STALE":
		return "base proxy health check has expired; the monitor must publish a fresh successful check"
	default:
		return "route verification unavailable"
	}
}

func (c Container) requireAccountNetworkReady(ctx context.Context, accountID string) error {
	return requireAccountNetworkReadyQuery(ctx, c.DB, accountID, c.Economy != nil)
}

// All validation can run on the connection that owns the route lock.
func requireAccountNetworkReadyQuery(ctx context.Context, db countryQuery, accountID string, useEconomy bool) error {
	var mode, proxyStatus, exitIP string
	var lastHealth sql.NullBool
	var rotationStarted sql.NullTime
	err := db.QueryRowContext(ctx, `
SELECT np.mode,COALESCE(p.status,''),COALESCE(host(ani.exit_ip),''),
       ani.last_health_ok,ani.rotation_started_at
FROM network_profiles np
LEFT JOIN proxies p ON p.id=np.proxy_id
LEFT JOIN account_network_identities ani ON ani.account_id=np.account_id
WHERE np.account_id=$1
`, accountID).Scan(&mode, &proxyStatus, &exitIP, &lastHealth, &rotationStarted)
	if errors.Is(err, sql.ErrNoRows) {
		return networkReadinessError("ROUTE_MISSING")
	}
	if err != nil {
		return err
	}
	if ready, e := countryPolicyReady(ctx, db, accountID); e != nil {
		return e
	} else if !ready {
		return networkReadinessError("COUNTRY_POLICY_UNVERIFIED")
	}
	if mode == string(network.RouteDirect) {
		return nil
	}
	if mode != string(network.RouteProxyRequired) {
		return networkReadinessError("ROUTE_MODE_INVALID")
	}
	if proxyStatus != string(network.StatusHealthy) {
		return networkReadinessError("BASE_PROXY_UNHEALTHY")
	}
	ip := net.ParseIP(exitIP)
	if ip == nil || ip.To4() == nil {
		return networkReadinessError("IDENTITY_IPV4_MISSING")
	}
	if !lastHealth.Valid || !lastHealth.Bool || rotationStarted.Valid {
		return networkReadinessError("IDENTITY_UNVERIFIED")
	}
	enabled := false
	if useEconomy {
		if e := db.QueryRowContext(ctx, `SELECT enabled AND (canary_account_id IS NULL OR canary_account_id::text=$1) FROM proxy_economy_policy WHERE singleton`, accountID).Scan(&enabled); e != nil {
			return e
		}
	}
	if enabled {
		var identityFresh, baseFresh bool
		err = db.QueryRowContext(ctx, `SELECT COALESCE(i.last_health_at<=clock_timestamp() AND i.last_health_at>clock_timestamp()-($2*interval '1 second'),false),
COALESCE(p.last_success_at<=clock_timestamp() AND p.last_success_at>clock_timestamp()-($3*interval '1 second'),false)
FROM network_profiles n JOIN proxies p ON p.id=n.proxy_id JOIN account_network_identities i ON i.account_id=n.account_id WHERE n.account_id=$1`, accountID, int(network.EconomyIdentityMaxAge.Seconds()), int(network.EconomyBaseMaxAge.Seconds())).Scan(&identityFresh, &baseFresh)
		if err != nil {
			return err
		}
		if !baseFresh {
			return networkReadinessError("BASE_PROXY_HEALTH_STALE")
		}
		if !identityFresh {
			return networkReadinessError("IDENTITY_HEALTH_STALE")
		}
	}
	return nil
}
