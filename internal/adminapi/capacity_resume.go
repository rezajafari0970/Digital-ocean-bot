package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
)

func (s *Server) resumeCapacityAutomation(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	for _, lock := range []int64{628341902731, 628341902732, 137136} {
		if _, err = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock($1)", lock); err != nil {
			writeJSON(w, 409, map[string]string{"error": "automation_busy", "detail": "An operation is settling. Refresh before retrying."})
			return
		}
	}
	var allowed bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM global_config_policies g CROSS JOIN bulk_lifecycle_control c WHERE g.policy_key='reality' AND g.enabled AND c.enabled AND c.auto_enroll)
 AND NOT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED'))`).Scan(&allowed)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if !allowed {
		writeJSON(w, 409, map[string]string{"error": "automation_not_ready", "detail": "Finish or cancel cleanup and enable Global Reality before resuming automatic clients."})
		return
	}
	if err = resumeCapacityTx(r.Context(), tx); err != nil {
		if errors.Is(err, errScopedExecutionGate) {
			writeJSON(w, 409, map[string]string{"error": "scoped_execution_gate", "detail": "The scoped execution gate cannot be widened by this action."})
			return
		}
		writeJSON(w, 500, errorBody())
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"state": "enabled", "detail": "Automatic clients resumed with existing policy, scope limits and remaining budgets. Any execution error closes the gate again."})
}

var errScopedExecutionGate = errors.New("scoped execution gate cannot be widened")

func resumeCapacityTx(ctx context.Context, tx *sql.Tx) error {
	var fleet bool
	if err := tx.QueryRowContext(ctx, "SELECT panel_id IS NULL AND inbound_id IS NULL AND concurrency=1 FROM client_mutation_execution_gate WHERE singleton FOR UPDATE").Scan(&fleet); err != nil {
		return err
	}
	if !fleet {
		return errScopedExecutionGate
	}
	// Rearm only explicitly global scopes, preserving their budgets and expiry.
	// A failed client job requires its own reconciliation, never an implicit retry.
	_, err := tx.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes s SET enabled=true,updated_at=now()
 WHERE (s.panel_id,s.inbound_id) IN(
 SELECT sc.panel_id,sc.inbound_id FROM bulk_lifecycle_scopes sc
 JOIN bulk_user_generations gen ON gen.id=sc.generation_id JOIN panel_instances pi ON pi.id=sc.panel_id
 JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id
 WHERE NOT sc.enabled AND sc.use_global_policy AND sc.allow_create AND sc.remaining_operations>0
 AND sc.expires_at>now()+interval '60 seconds' AND gen.state='ACTIVE' AND pi.enabled AND a.enabled
 AND a.deletion_requested_at IS NULL AND a.provider_state='ACTIVE' AND dr.state='READY'
 AND(dr.expires_at IS NULL OR dr.expires_at>now()+interval '60 seconds')
 AND (sc.last_error<>'policy_port_removed' OR EXISTS(SELECT 1 FROM panel_inbound_inventory i CROSS JOIN global_config_policies pol WHERE i.panel_id=sc.panel_id AND i.remote_id=sc.inbound_id AND i.present AND pol.policy_key='reality' AND pol.ports @> to_jsonb(ARRAY[i.port])))
 AND NOT EXISTS(SELECT 1 FROM client_mutation_jobs j WHERE j.panel_id=sc.panel_id AND j.inbound_id=sc.inbound_id AND j.state='FAILED')
 ORDER BY sc.panel_id,sc.inbound_id
 LIMIT GREATEST(0,(SELECT max_active_scopes FROM bulk_lifecycle_control WHERE singleton)-(SELECT count(*) FROM bulk_lifecycle_scopes WHERE enabled AND expires_at>now()))
 )`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=NULL,inbound_id=NULL,updated_at=now() WHERE singleton"); err != nil {
		return err
	}
	return nil
}
