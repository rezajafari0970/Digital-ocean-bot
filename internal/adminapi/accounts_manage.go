package adminapi

import (
	"encoding/json"
	"net/http"
)

type accountUpdate struct {
	Name            string   `json:"name"`
	Token           string   `json:"token"`
	Regions         []string `json:"regions"`
	Sizes           []string `json:"sizes"`
	Image           string   `json:"image"`
	LifetimeSeconds int      `json:"lifetime_seconds"`
	IntervalSeconds int      `json:"interval_seconds"`
	BatchSize       int      `json:"batch_size"`
	MaxConcurrent   int      `json:"max_concurrent"`
	NetworkMode     string   `json:"network_mode"`
	ProxyID         string   `json:"proxy_id"`
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
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
	if x.MaxConcurrent < 1 {
		x.MaxConcurrent = 1
	}
	regions, _ := json.Marshal(x.Regions)
	sizes, _ := json.Marshal(x.Sizes)
	res, err := s.DB.ExecContext(r.Context(), `UPDATE accounts SET name=$2,preferred_regions=$3,preferred_sizes=$4,preferred_image=NULLIF($5,''),server_lifetime_seconds=$6,auto_interval_seconds=$7,auto_batch_size=$8,auto_max_concurrent=$9,updated_at=now() WHERE id=$1`, id, x.Name, regions, sizes, x.Image, x.LifetimeSeconds, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if x.NetworkMode == "" {
		x.NetworkMode = "direct"
	}
	if x.NetworkMode != "direct" && x.NetworkMode != "proxy_required" {
		writeJSON(w, 400, map[string]string{"error": "invalid_network_mode"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		var status string
		if x.ProxyID == "" || s.DB.QueryRowContext(r.Context(), `SELECT status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&status) != nil || status != "healthy" {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_healthy"})
			return
		}
	}
	if _, err := s.DB.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END) ON CONFLICT(account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id,updated_at=now()`, id, x.NetworkMode, x.ProxyID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.Token != "" {
		if err := s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
			writeJSON(w, 500, errorBody())
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
	var active int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED')`, id).Scan(&active)
	if active > 0 {
		writeJSON(w, 409, map[string]string{"error": "account_has_active_deployments"})
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
