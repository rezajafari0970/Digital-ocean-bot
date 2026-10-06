package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"time"
)

type BootCircuit struct {
	Image    string    `json:"image"`
	Region   string    `json:"region"`
	Plan     string    `json:"plan"`
	Failures int       `json:"failures"`
	RetryAt  time.Time `json:"retry_at"`
	State    string    `json:"state"`
}

// Scope failures to a selected boot combination. A later completed SSH probe
// clears boot failures even if installation subsequently fails. After cooldown,
// the admission transaction permits only one pending probe for that combination.
func bootCircuits(ctx context.Context, q workflow.DBTX, account string) ([]BootCircuit, error) {
	rows, err := q.QueryContext(ctx, `WITH attempts AS (
 SELECT d.profile_snapshot->>'image' image,d.profile_snapshot->>'region' region,d.profile_snapshot->>'size' plan,d.state,d.last_error,d.updated_at,
 CASE WHEN d.state IN ('READY','PANEL_COMPLETE') THEN d.updated_at
 WHEN ps.last_finished_at>=ps.last_started_at AND COALESCE(ps.last_error,'')='' AND NOT ps.terminal THEN ps.last_finished_at END boot_success_at
 FROM deployments d LEFT JOIN provision_runs pr ON pr.account_id=d.account_id AND pr.droplet_id=d.droplet_id
 LEFT JOIN provision_step_attempts ps ON ps.run_id=pr.id AND ps.step='ssh'
 WHERE d.account_id=$1 AND d.updated_at>now()-interval '24 hours'
 AND COALESCE(d.profile_snapshot->>'image','')<>'' AND COALESCE(d.profile_snapshot->>'region','')<>'' AND COALESCE(d.profile_snapshot->>'size','')<>''
 ), success AS (SELECT image,region,plan,max(boot_success_at) at FROM attempts GROUP BY image,region,plan)
 SELECT a.image,a.region,a.plan,count(*),max(a.updated_at)+interval '1 hour',
 CASE WHEN max(a.updated_at)>now()-interval '1 hour' THEN 'COOLDOWN'
 WHEN EXISTS(SELECT 1 FROM deployments p WHERE p.account_id=$1 AND p.profile_snapshot->>'image'=a.image AND p.profile_snapshot->>'region'=a.region AND p.profile_snapshot->>'size'=a.plan
 AND p.state NOT IN ('READY','PANEL_COMPLETE','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK')) THEN 'PROBE_IN_FLIGHT' ELSE 'PROBE_ELIGIBLE' END
 FROM attempts a LEFT JOIN success s ON s.image=a.image AND s.region=a.region AND s.plan=a.plan
 WHERE a.state='FAILED' AND a.last_error LIKE 'INITIAL_SSH_BUDGET_EXHAUSTED:%' AND (s.at IS NULL OR a.updated_at>s.at)
 GROUP BY a.image,a.region,a.plan HAVING count(*)>=2 ORDER BY max(a.updated_at) DESC`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BootCircuit{}
	for rows.Next() {
		var b BootCircuit
		if err = rows.Scan(&b.Image, &b.Region, &b.Plan, &b.Failures, &b.RetryAt, &b.State); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (c Container) BootFailureCircuits(ctx context.Context, account string) ([]BootCircuit, error) {
	return bootCircuits(ctx, c.DB, account)
}
func bootImageQuarantine(ctx context.Context, q workflow.DBTX, account, region, size string) (map[string]bool, error) {
	circuits, err := bootCircuits(ctx, q, account)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, b := range circuits {
		if b.Region == region && b.Plan == size && b.State != "PROBE_ELIGIBLE" {
			out[b.Image] = true
		}
	}
	return out, nil
}
