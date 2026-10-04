package usercapacity

import (
	"context"
	"fmt"
)

// Removed ports stop consuming admission slots. This closes only settled,
// global-policy scopes; it never deletes clients, resets budgets or rearms work.
func (s Service) retirePolicyScopes(ctx context.Context, panel string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM global_config_policies WHERE policy_key='reality' FOR SHARE`).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT sc.inbound_id FROM bulk_lifecycle_scopes sc JOIN panel_inbound_inventory i ON i.panel_id=sc.panel_id AND i.remote_id=sc.inbound_id CROSS JOIN global_config_policies pol CROSS JOIN client_mutation_execution_gate gate WHERE sc.panel_id=$1 AND sc.enabled AND sc.use_global_policy AND i.present AND pol.policy_key='reality' AND NOT(pol.ports @> to_jsonb(ARRAY[i.port])) AND gate.enabled AND NOT gate.kill_switch AND (gate.panel_id IS NULL OR gate.panel_id=sc.panel_id) AND (gate.inbound_id IS NULL OR gate.inbound_id=sc.inbound_id)`, panel)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		var locked bool
		if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,941))`, fmt.Sprintf("%s:%d", panel, id)).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			continue
		}
		// Fresh read after the planner lock: a just-committed plan must settle first.
		if _, err = tx.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes sc SET enabled=false,last_error='policy_port_removed',updated_at=now() WHERE sc.panel_id=$1 AND sc.inbound_id=$2 AND sc.enabled AND sc.use_global_policy AND NOT EXISTS(SELECT 1 FROM client_mutation_jobs j WHERE j.panel_id=sc.panel_id AND j.inbound_id=sc.inbound_id AND j.state IN('PENDING','RUNNING','FAILED'))`, panel, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
