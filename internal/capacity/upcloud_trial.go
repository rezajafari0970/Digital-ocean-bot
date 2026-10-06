package capacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// Opt-in authorizes a compatible request, not unrestricted permission.
// An actual create denial remains authoritative until explicit recovery.
func AccountStatusAllowsCreate(ctx context.Context, q Querier, id, status string) (bool, error) {
	if status == "active" {
		return true, nil
	}
	if status != "trial_restricted" {
		return false, nil
	}
	var allowed bool
	err := q.QueryRowContext(ctx, "SELECT provider='upcloud' AND upcloud_trial_compatible FROM accounts WHERE id=$1", id).Scan(&allowed)
	return allowed, err
}

type TrialModeError struct{ Code, Detail string }

func (e *TrialModeError) Error() string { return e.Code + ": " + e.Detail }

// EnableUpCloudTrial is used by the authenticated API and the authorized local
// operator CLI. It never grants application roles or synthesizes a login.
func EnableUpCloudTrial(ctx context.Context, db *sql.DB, id string, version int64, actor string) error {
	if id == "" || version <= 0 || actor == "" {
		return &TrialModeError{Code: "invalid_trial_authorization"}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "deployment-admission:"+id); err != nil {
		return err
	}
	var provider string
	var enabled, deleting, trial bool
	err = tx.QueryRowContext(ctx, "SELECT provider,enabled,deletion_requested_at IS NOT NULL,upcloud_trial_compatible FROM accounts WHERE id=$1 FOR UPDATE", id).Scan(&provider, &enabled, &deleting, &trial)
	if err == sql.ErrNoRows {
		return &TrialModeError{Code: "not_found"}
	}
	if err != nil {
		return err
	}
	if provider != "upcloud" || !enabled || deleting || trial {
		return &TrialModeError{Code: "trial_mode_unavailable", Detail: "Requires an enabled UpCloud account with trial-compatible mode not yet enabled. A new denial after enabling needs separate review."}
	}
	var busy bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')) OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown'))`, id).Scan(&busy)
	if err != nil {
		return err
	}
	if busy {
		return &TrialModeError{Code: "create_in_flight", Detail: "Wait for the current deployment to settle before changing its network mode."}
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, "SELECT ports FROM global_config_policies WHERE policy_key='reality'").Scan(&raw); err != nil {
		return err
	}
	var ports []int
	if json.Unmarshal(raw, &ports) != nil || len(ports) == 0 {
		return &TrialModeError{Code: "trial_client_ports_unknown"}
	}
	for _, port := range ports {
		if !providers.UpCloudTrialClientPortAllowed(port) {
			return &TrialModeError{Code: "trial_client_port_incompatible", Detail: "Trial-compatible deployment requires client ports 80/443; existing global ports were not changed."}
		}
	}
	var code string
	err = tx.QueryRowContext(ctx, "DELETE FROM account_create_blocks WHERE account_id=$1 AND version=$2 AND code='TRIAL_FIREWALL' RETURNING code", id, version).Scan(&code)
	if err == sql.ErrNoRows {
		return &TrialModeError{Code: "create_block_changed", Detail: "Requires the current TRIAL_FIREWALL version. Refresh the account."}
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE accounts SET upcloud_trial_compatible=true,updated_at=now() WHERE id=$1", id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result,message,metadata) VALUES($1,$2,'upcloud_trial_compatible_enabled','account',$1::uuid::text,'ok','Operator authorized restricted trial-compatible deployment; provider firewall restrictions remain',jsonb_build_object('block_version',$3::bigint,'panel_port',3389,'firewall','on','restricted_egress_accepted',true))`, id, actor, version)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
