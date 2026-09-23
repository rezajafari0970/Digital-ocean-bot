package adminapi

import (
	"encoding/json"
	"net/http"
)

type accountUpdate struct {
	Name               string   `json:"name"`
	Token              string   `json:"token"`
	Regions            []string `json:"regions"`
	Sizes              []string `json:"sizes"`
	Image              string   `json:"image"`
	LifetimeSeconds    int      `json:"lifetime_seconds"`
	IntervalSeconds    int      `json:"interval_seconds"`
	BatchSize          int      `json:"batch_size"`
	MaxConcurrent      int      `json:"max_concurrent"`
	DesiredServerCount int      `json:"desired_server_count"`
	NetworkMode        string   `json:"network_mode"`
	ProxyID            string   `json:"proxy_id"`
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x accountUpdate
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.LifetimeSeconds < 1800 {
		x.LifetimeSeconds = 7200
	}
	if x.IntervalSeconds < 60 {
		x.IntervalSeconds = 300
	}
	if x.BatchSize < 1 {
		x.BatchSize = 1
	}
	if x.DesiredServerCount < 1 {
		x.DesiredServerCount = 1
	}
	if x.MaxConcurrent < 1 {
		x.MaxConcurrent = 1
	}
	regions, _ := json.Marshal(x.Regions)
	sizes, _ := json.Marshal(x.Sizes)
	var candidateEmail, candidateExternalID string
	if x.Token != "" {
		d, validateErr := s.validateReplacementToken(r.Context(), id, x.Token, x.NetworkMode, x.ProxyID)
		if validateErr != nil {
			writeJSON(w, 422, map[string]string{"error": "replacement_token_validation_failed", "detail": validateErr.Error()})
			return
		}
		candidateEmail, candidateExternalID = d.Email, d.UUID
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if x.NetworkMode == "" {
		x.NetworkMode = "direct"
	}
	if x.NetworkMode != "direct" && x.NetworkMode != "proxy_required" {
		writeJSON(w, 400, map[string]string{"error": "invalid_network_mode"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		var status string
		if x.ProxyID == "" || tx.QueryRowContext(r.Context(), `SELECT status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&status) != nil || (status != "healthy" && status != "degraded") {
			writeJSON(w, 409, map[string]string{"error": "proxy_unavailable", "detail": "Selected proxy is not currently usable"})
			return
		}
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE accounts SET name=$2,preferred_regions=$3,preferred_region=COALESCE(NULLIF($11,''),preferred_region),preferred_sizes=$4,preferred_image=COALESCE(NULLIF($5,''),preferred_image),server_lifetime_seconds=$6,auto_interval_seconds=$7,auto_batch_size=$8,auto_max_concurrent=$9,desired_server_count=$10,updated_at=now() WHERE id=$1`, id, x.Name, regions, sizes, x.Image, x.LifetimeSeconds, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent, x.DesiredServerCount, func() string {
		if len(x.Regions) > 0 {
			return x.Regions[0]
		}
		return ""
	}())
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id,updated_at=now()`, id, x.NetworkMode, x.ProxyID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err := syncAccountAutomationTx(r.Context(), tx, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": "automation_sync_failed", "detail": err.Error()})
		return
	}
	if x.Token != "" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE accounts SET external_id=$2,email=NULLIF($3,'') WHERE id=$1`, id, candidateExternalID, candidateEmail); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.Token != "" {
		if err := s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
			writeJSON(w, 500, map[string]string{"error": "token_update_failed", "detail": err.Error()})
			return
		}
	}
	w.WriteHeader(204)
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var deployments, droplets int
	if err := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM deployments WHERE account_id=$1 AND state <> 'FAILED'`, id).Scan(&deployments); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM droplets WHERE account_id=$1 AND state <> 'DELETED'`, id).Scan(&droplets); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if deployments > 0 || droplets > 0 {
		writeJSON(w, 409, map[string]any{"error": "account_has_managed_resources", "deployments": deployments, "droplets": droplets})
		return
	}
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	w.WriteHeader(204)
}
