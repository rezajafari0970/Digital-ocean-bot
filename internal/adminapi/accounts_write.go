package adminapi

import (
	"encoding/json"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

type accountWrite struct {
	Email           string `json:"email"`
	ExternalID      string `json:"external_id"`
	Image           string `json:"image"`
	Size            string `json:"size"`
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var x accountWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Token == "" || x.ExternalID == "" || x.Region == "" || x.Size == "" || x.Image == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if x.NetworkMode == "" {
		x.NetworkMode = "direct"
	}
	if x.NetworkMode != "direct" && x.NetworkMode != "proxy_required" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_network_mode"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		var status string
		if x.ProxyID == "" || s.DB.QueryRowContext(r.Context(), `SELECT status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&status) != nil || status != "healthy" {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "proxy_not_healthy"})
			return
		}
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
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO accounts(id,provider,name,external_id,email,preferred_region,secret_ref,auto_interval_seconds,auto_batch_size,auto_max_concurrent)
VALUES(gen_random_uuid(),'digitalocean',$1,$2,NULLIF($3,''),$4,'do-token',$5,$6,$7)
ON CONFLICT (provider,external_id) WHERE external_id IS NOT NULL DO UPDATE SET
name=EXCLUDED.name,email=EXCLUDED.email,preferred_region=EXCLUDED.preferred_region,
auto_interval_seconds=EXCLUDED.auto_interval_seconds,auto_batch_size=EXCLUDED.auto_batch_size,
auto_max_concurrent=EXCLUDED.auto_max_concurrent,updated_at=now()
RETURNING id::text`, x.Name, x.ExternalID, x.Email, x.Region, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account_insert_failed", "detail": err.Error()})
		return
	}
	if _, err = s.DB.ExecContext(r.Context(), `UPDATE accounts SET preferred_regions=jsonb_build_array($2::text),preferred_sizes=jsonb_build_array($3::text),preferred_image=$4 WHERE id=$1`, id, x.Region, x.Size, x.Image); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "account_preferences_failed", "detail": err.Error()})
		return
	}
	if err = s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_store_failed", "detail": err.Error()})
		return
	}
	if _, err = s.DB.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END) ON CONFLICT (account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id,updated_at=now()`, id, x.NetworkMode, x.ProxyID); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network_profile_failed", "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func requireAdmin(p auth.Principal) bool { return p.CanAdmin() }
