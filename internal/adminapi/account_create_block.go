package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func (s *Server) releaseAccountCreateBlock(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x struct {
		Version  int64 `json:"version"`
		Resolved bool  `json:"restriction_resolved"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&x) != nil || x.Version <= 0 || !x.Resolved {
		writeJSON(w, 400, map[string]string{"error": "confirm_provider_restriction_resolved"})
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var trial bool
	if err = tx.QueryRowContext(r.Context(), `SELECT COALESCE((SELECT canonical->'Account'->>'Status'='trial_restricted' FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1),false)`, r.PathValue("id")).Scan(&trial); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if trial {
		writeJSON(w, 409, map[string]string{"error": "trial_restriction_active", "detail": "Refresh Account Data after UpCloud confirms the trial restriction is removed."})
		return
	}
	var code string
	err = tx.QueryRowContext(r.Context(), "DELETE FROM account_create_blocks WHERE account_id=$1 AND version=$2 RETURNING code", r.PathValue("id"), x.Version).Scan(&code)
	if err == sql.ErrNoRows {
		writeJSON(w, 409, map[string]string{"error": "create_block_changed", "detail": "Refresh the account before retrying."})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO audit_events(account_id,actor,action,resource_type,resource_id,result,message,metadata) VALUES($1,$3,'create_permission_retry_enabled','account',$1::uuid::text,'ok',$2,jsonb_build_object('block_version',$4::bigint))`, r.PathValue("id"), code+": operator confirmed restriction resolved", p.Username, x.Version)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "retry_enabled", "detail": "Existing scheduling gates still apply. A new provider rejection will block creation again."})
}
