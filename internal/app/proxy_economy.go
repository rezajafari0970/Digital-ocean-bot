package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"time"
)

// Called while holding the account keeper lease. Never fake a new observation
// timestamp when skipping. Recovery, fallback and degraded circuits stay fast.
func (c Container) identityProbeMayWait(ctx context.Context, id string) (bool, error) {
	var fresh bool
	err := c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities i
JOIN network_profiles n ON n.account_id=i.account_id JOIN proxies p ON p.id=n.proxy_id
JOIN accounts a ON a.id=i.account_id JOIN proxy_runtime_state s ON s.account_id=a.id AND s.proxy_id=p.id AND s.provider=a.provider
WHERE i.account_id=$1 AND i.last_health_ok=true AND i.exit_ip IS NOT NULL
AND i.rotation_started_at IS NULL AND NOT i.fallback_active
AND i.last_health_at<=now() AND i.last_health_at>now()-($2*interval '1 second')
AND p.status='healthy' AND p.last_success_at<=now() AND p.last_success_at>now()-($3*interval '1 second')
AND p.consecutive_successes>=2 AND s.health_state='healthy' AND s.circuit_state='closed'
AND (s.retry_after IS NULL OR s.retry_after<=now()))`, id, int(network.EconomyIdentityInterval/time.Second), int(network.EconomyBaseMaxAge/time.Second)).Scan(&fresh)
	return fresh, err
}

// Inventory remains fast whenever work or an expiry may need fresh capacity.
// A stale idle snapshot never authorizes a create: existing preflight freshness
// rules are deliberately unchanged.
func (c Container) providerRefreshAge(ctx context.Context, id string, active time.Duration) time.Duration {
	if c.DB == nil || active >= 5*time.Minute {
		return active
	}
	enabled, err := c.Economy.Enabled(ctx, id)
	if err != nil || !enabled {
		return active
	}
	var idle bool
	err = c.DB.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND state IN ('planned','running','verifying','unknown'))
AND NOT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state<>'DELETED' AND (state<>'READY' OR expires_at IS NULL OR expires_at<=now()+interval '6 minutes'))
AND NOT EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE'))
AND NOT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state='DELETED' AND backfill_required=true AND replacement_deployment_id IS NULL)
AND NOT EXISTS(SELECT 1 FROM worker_recovery_checkpoints WHERE account_id=$1)
AND EXISTS(SELECT 1 FROM accounts a WHERE a.id=$1 AND a.enabled=true AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL
AND (a.next_build_at>now()+interval '5 minutes'
OR (a.desired_server_count>0 AND a.desired_server_count<=(SELECT count(*) FROM droplets WHERE account_id=a.id AND state='READY' AND expires_at>now()+interval '6 minutes'))))`, id).Scan(&idle)
	if err == nil && idle {
		return 5 * time.Minute
	}
	return active
}
