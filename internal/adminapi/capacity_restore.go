package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// Restore is an explicit, revision-checked choice to stop a saved cleanup and
// resume the saved policy. It never claims unfinished deletions succeeded.
func (s *Server) restoreCapacityAutomation(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		ExpectedRevision int64  `json:"expected_revision"`
		CleanupID        string `json:"cleanup_job_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || req.ExpectedRevision < 1 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	busy := func() {
		writeJSON(w, 409, map[string]string{"error": "restore_not_committed", "detail": "An operation is settling or the saved state changed. Refresh before retrying."})
	}
	if _, err = tx.ExecContext(r.Context(), "SET LOCAL lock_timeout='4s'"); err != nil {
		busy()
		return
	}
	for _, lock := range []int64{628341902731, 628341902732, 137136} {
		if _, err = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock($1)", lock); err != nil {
			busy()
			return
		}
	}
	var revision int64
	if err = tx.QueryRowContext(r.Context(), "SELECT revision FROM global_config_policies WHERE policy_key='reality' FOR UPDATE").Scan(&revision); err != nil || revision != req.ExpectedRevision {
		busy()
		return
	}
	var allowed bool
	if err = tx.QueryRowContext(r.Context(), "SELECT enabled AND auto_enroll FROM bulk_lifecycle_control WHERE singleton FOR UPDATE").Scan(&allowed); err != nil || !allowed {
		writeJSON(w, 409, map[string]string{"error": "automation_control_disabled", "detail": "Automatic enrollment is disabled. No saved limits were changed."})
		return
	}
	var fleetGate bool
	if err = tx.QueryRowContext(r.Context(), "SELECT panel_id IS NULL AND inbound_id IS NULL AND concurrency=1 FROM client_mutation_execution_gate WHERE singleton FOR UPDATE").Scan(&fleetGate); err != nil || !fleetGate {
		writeJSON(w, 409, map[string]string{"error": "scoped_execution_gate", "detail": "A scoped canary gate is configured. It cannot be widened by restoring a global policy."})
		return
	}
	var activeID string
	err = tx.QueryRowContext(r.Context(), "SELECT id::text FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED') FOR UPDATE").Scan(&activeID)
	if err != nil && err != sql.ErrNoRows {
		busy()
		return
	}
	if activeID != req.CleanupID {
		busy()
		return
	}
	if activeID != "" {
		if _, err = tx.ExecContext(r.Context(), "UPDATE panel_cleanup_jobs SET state='CANCELLED',completed_at=now() WHERE id=$1", activeID); err != nil {
			busy()
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE global_config_policies SET enabled=true,revision=revision+1,updated_at=now() WHERE policy_key='reality'"); err != nil {
		busy()
		return
	}
	if err = resumeCapacityTx(r.Context(), tx); err != nil {
		busy()
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 409, map[string]string{"error": "restore_outcome_unconfirmed", "detail": "Refresh policy and cleanup status before another request."})
		return
	}
	writeJSON(w, 200, map[string]any{"state": "enabled", "revision": revision + 1, "detail": "Saved Reality policy resumed. Remaining cleanup stopped; completed deletions are preserved. New configs appear after fresh panel verification."})
}
