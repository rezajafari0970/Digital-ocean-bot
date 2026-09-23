package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"net/http"
)

type accountWrite struct {
	Email           string `json:"email"`
	Region          string `json:"region"`
	NetworkMode     string `json:"network_mode"`
	ProxyID         string `json:"proxy_id"`
	IntervalSeconds int    `json:"interval_seconds"`
	BatchSize       int    `json:"batch_size"`
	MaxConcurrent   int    `json:"max_concurrent"`
	Name            string `json:"name"`
	Token           string `json:"token"`
	Enabled         *bool  `json:"enabled"`
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x accountWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Token == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.NetworkMode == "" {
		x.NetworkMode = "direct"
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
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO accounts(id,provider,name,email,preferred_region,secret_ref,auto_interval_seconds,auto_batch_size,auto_max_concurrent) VALUES(gen_random_uuid(),'digitalocean',$1,NULLIF($2,''),NULLIF($3,''),'do-token',$4,$5,$6) RETURNING id::text`, x.Name, x.Email, x.Region, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	if err := s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, 500, errorBody())
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END)`, id, x.NetworkMode, x.ProxyID)
	writeJSON(w, 201, map[string]string{"id": id})
}
func requireAdmin(p auth.Principal) bool { return p.CanAdmin() }
