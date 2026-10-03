package usercapacity

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"strings"
	"time"
)

// Automatic admission never rearms an existing scope or an exhausted budget.
// It only enrolls new, fresh-verified v3 Reality inbounds after explicit fleet
// authorization. Expiry, account state, historical errors and the global cap
// are rechecked transactionally after the network observation.
func (s Service) autoEnrollLifecycle(ctx context.Context, p readyworker.Panel, rt *sanaei.PanelRuntime) error {
	var eligible bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bulk_lifecycle_control c CROSS JOIN client_mutation_execution_gate g CROSS JOIN global_config_policies pol WHERE c.enabled AND c.auto_enroll AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=$1) AND pol.policy_key='reality' AND pol.enabled)`, p.ID).Scan(&eligible)
	if err != nil || !eligible {
		return err
	}
	var rawPorts []byte
	if err = s.DB.QueryRowContext(ctx, `SELECT ports FROM global_config_policies WHERE policy_key='reality'`).Scan(&rawPorts); err != nil {
		return err
	}
	var ports []int
	if json.Unmarshal(rawPorts, &ports) != nil {
		return fmt.Errorf("lifecycle admission ports")
	}
	wanted := map[int]bool{}
	for _, port := range ports {
		wanted[port] = true
	}
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return err
	}
	inv, err := sanaei.InventoryFromRaw(p.ID, raws)
	if err != nil {
		return err
	}
	for _, in := range inv.Records {
		if !in.Enabled || in.Protocol != "vless" || in.Transport != "tcp" || in.Security != "reality" || !wanted[in.Port] {
			continue
		}
		var existing bool
		if err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bulk_lifecycle_scopes WHERE panel_id=$1 AND inbound_id=$2) OR EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING','FAILED'))`, p.ID, in.RemoteID).Scan(&existing); err != nil {
			return err
		}
		if existing {
			continue
		}
		observed, _, err := clientops.LifecycleInventory(ctx, rt, in.RemoteID)
		if err != nil {
			return err
		}
		for _, c := range observed {
			if _, err = c.InactiveReason(time.Now()); err != nil {
				return err
			}
		}
		if err = s.admitLifecycleScope(ctx, p.ID, in.RemoteID, in.Port); err != nil {
			return err
		}
	}
	return nil
}
func (s Service) admitLifecycleScope(ctx context.Context, panel string, inbound int64, port int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(137136)`).Scan(&locked); err != nil || !locked {
		return err
	}
	// Serialize against lifecycle planners and lock all authorization/state rows
	// before evaluating eligibility with a new READ COMMITTED snapshot.
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,941))`, fmt.Sprintf("%s:%d", panel, inbound)).Scan(&locked); err != nil || !locked {
		return err
	}
	locks, err := tx.QueryContext(ctx, `SELECT c.singleton FROM bulk_lifecycle_control c CROSS JOIN client_mutation_execution_gate g CROSS JOIN global_config_policies pol JOIN panel_instances p ON p.id=$1 JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id WHERE pol.policy_key='reality' FOR SHARE OF c,g,pol,p,d,a,dep`, panel)
	if err != nil {
		return err
	}
	count := 0
	for locks.Next() {
		var singleton bool
		if err = locks.Scan(&singleton); err != nil {
			locks.Close()
			return err
		}
		count++
	}
	err = locks.Err()
	locks.Close()
	if err != nil || count == 0 {
		return err
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bulk_lifecycle_control c CROSS JOIN client_mutation_execution_gate g CROSS JOIN global_config_policies pol JOIN panel_instances p ON p.id=$1 JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id WHERE c.enabled AND c.auto_enroll AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=p.id) AND (g.inbound_id IS NULL OR g.inbound_id=$2) AND pol.policy_key='reality' AND pol.enabled AND pol.ports @> to_jsonb(ARRAY[$3::integer]) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND (d.expires_at IS NULL OR d.expires_at>now()+interval '60 seconds') AND (SELECT count(*) FROM bulk_lifecycle_scopes WHERE enabled AND expires_at>now())<c.max_active_scopes) AND NOT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING','FAILED'))`, panel, inbound, port).Scan(&allowed)
	if err != nil || !allowed {
		return err
	}
	marker := fmt.Sprintf("p%s-i%d", strings.ReplaceAll(panel, "-", ""), inbound)
	var generation, state string
	err = tx.QueryRowContext(ctx, `INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,$2,'POLICY',$3) ON CONFLICT(marker) DO UPDATE SET marker=excluded.marker RETURNING id::text,state`, panel, inbound, marker).Scan(&generation, &state)
	if err != nil {
		return err
	}
	if state != "ACTIVE" {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,max_batch_size,remaining_operations,expires_at) SELECT $1,$2,$3,true,true,true,c.max_batch_size,c.operation_budget,LEAST(COALESCE(d.expires_at-interval '10 seconds',now()+interval '4 hours'),now()+interval '24 hours') FROM bulk_lifecycle_control c JOIN panel_instances p ON p.id=$1 JOIN droplets d ON d.id=p.droplet_id WHERE c.enabled AND c.auto_enroll ON CONFLICT(panel_id,inbound_id) DO NOTHING`, panel, inbound, generation)
	if err != nil {
		return err
	}
	return tx.Commit()
}
