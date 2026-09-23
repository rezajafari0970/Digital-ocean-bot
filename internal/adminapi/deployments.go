package adminapi

import (
	"encoding/json"
	"net/http"
)

type createDeploymentRequest struct {
	AccountID string `json:"account_id"`
	ProfileID string `json:"profile_id"`
}

func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request) {
	var req createDeploymentRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.AccountID == "" || req.ProfileID == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	d, err := s.Container.StartDeployment(r.Context(), req.AccountID, req.ProfileID)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "deployment_failed"})
		return
	}
	writeJSON(w, 202, d)
}

func (s *Server) deployments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,account_id::text,profile_id::text,COALESCE(provider_id,''),state,current_step,attempt,COALESCE(last_error,''),created_at,updated_at FROM deployments ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, account, profile, provider, state, step, last string
		var attempt int
		var created, updated any
		if rows.Scan(&id, &account, &profile, &provider, &state, &step, &attempt, &last, &created, &updated) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "account_id": account, "profile_id": profile, "provider_id": provider, "state": state, "step": step, "attempt": attempt, "last_error": last, "created_at": created, "updated_at": updated})
	}
	writeJSON(w, 200, out)
}
