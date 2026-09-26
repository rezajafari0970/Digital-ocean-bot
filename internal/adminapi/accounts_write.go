package adminapi

import (
	"encoding/json"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
)

type accountWrite struct {
	Email                  string   `json:"email"`
	ExternalID             string   `json:"external_id"`
	Image                  string   `json:"image"`
	Images                 []string `json:"images"`
	Size                   string   `json:"size"`
	Sizes                  []string `json:"sizes"`
	Region                 string   `json:"region"`
	Regions                []string `json:"regions"`
	NetworkMode            string   `json:"network_mode"`
	ProxyID                string   `json:"proxy_id"`
	IntervalSeconds        int      `json:"interval_seconds"`
	BuildSpacingMinutes    int      `json:"build_spacing_minutes"`
	BuildSpacingMaxMinutes int      `json:"build_spacing_max_minutes"`
	BatchSize              int      `json:"batch_size"`
	MaxConcurrent          int      `json:"max_concurrent"`
	LifetimeSeconds        int      `json:"lifetime_seconds"`
	LifetimeMinSeconds     int      `json:"lifetime_min_seconds"`
	LifetimeMaxSeconds     int      `json:"lifetime_max_seconds"`
	DesiredServerCount     int      `json:"desired_server_count"`
	FallbackAnyRegion      *bool    `json:"fallback_any_region"`
	Name                   string   `json:"name"`
	Token                  string   `json:"token"`
	LoginEmail             string   `json:"login_email"`
	LoginPassword          string   `json:"login_password"`
	Enabled                *bool    `json:"enabled"`
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	var x accountWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Token == "" || x.ExternalID == "" || len(x.Regions) == 0 || len(x.Sizes) == 0 || len(x.Images) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if len(x.Regions) == 0 {
		x.Regions = []string{x.Region}
	}
	if code := validateAccountSettings(x, 0); code != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
		return
	}
	if len(x.Regions) > 5 {
		x.Regions = x.Regions[:5]
	}
	if len(x.Sizes) > 3 {
		x.Sizes = x.Sizes[:3]
	}
	if len(x.Images) > 3 {
		x.Images = x.Images[:3]
	}
	if (x.LoginEmail == "") != (x.LoginPassword == "") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "login_credentials_must_be_provided_together"})
		return
	}
	x.Region = x.Regions[0]
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
	if x.BuildSpacingMinutes < 1 {
		x.BuildSpacingMinutes = 60
	}
	if x.BuildSpacingMaxMinutes < x.BuildSpacingMinutes {
		x.BuildSpacingMaxMinutes = x.BuildSpacingMinutes
	}
	x.IntervalSeconds = 60
	if x.BatchSize < 1 {
		x.BatchSize = 1
	}
	if x.MaxConcurrent < 1 {
		x.MaxConcurrent = 1
	}
	if x.DesiredServerCount < 0 {
		x.DesiredServerCount = 0
	}
	if x.DesiredServerCount == 0 {
		x.DesiredServerCount = 1
	}
	if x.LifetimeMinSeconds < 1800 {
		x.LifetimeMinSeconds = 1800
	}
	if x.LifetimeMaxSeconds < x.LifetimeMinSeconds {
		x.LifetimeMaxSeconds = x.LifetimeMinSeconds
	}
	x.LifetimeSeconds = x.LifetimeMinSeconds
	fallbackAnyRegion := true
	if x.FallbackAnyRegion != nil {
		fallbackAnyRegion = *x.FallbackAnyRegion
	}
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO accounts(id,provider,name,external_id,email,preferred_region,secret_ref,auto_interval_seconds,auto_batch_size,auto_max_concurrent,server_lifetime_seconds,desired_server_count,fallback_any_region,build_spacing_minutes,build_spacing_max_minutes)
VALUES(gen_random_uuid(),'digitalocean',$1,$2,NULLIF($3,''),$4,'do-token',$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (provider,external_id) WHERE external_id IS NOT NULL DO NOTHING
RETURNING id::text`, x.Name, x.ExternalID, x.Email, x.Region, x.IntervalSeconds, x.BatchSize, x.MaxConcurrent, x.LifetimeSeconds, x.DesiredServerCount, fallbackAnyRegion, x.BuildSpacingMinutes, x.BuildSpacingMaxMinutes).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account_insert_failed", "detail": err.Error()})
		return
	}
	created := true
	cleanup := func() {
		if !created {
			return
		}
		// Account-scoped secrets are removed by the secrets.account_id
		// ON DELETE CASCADE foreign key when the account is deleted.
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
	}
	defer cleanup()
	if _, err = s.DB.ExecContext(r.Context(), `UPDATE accounts SET preferred_regions=$2::jsonb,preferred_sizes=$3::jsonb,preferred_images=$4::jsonb,preferred_image=$5,server_lifetime_min_seconds=$6,server_lifetime_max_seconds=$7,build_spacing_minutes=$8,build_spacing_max_minutes=$9,next_build_at=NULL WHERE id=$1`, id, mustJSON(x.Regions), mustJSON(x.Sizes), mustJSON(x.Images), x.Images[0], x.LifetimeMinSeconds, x.LifetimeMaxSeconds, x.BuildSpacingMinutes, x.BuildSpacingMaxMinutes); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "account_preferences_failed", "detail": err.Error()})
		return
	}
	if x.LoginEmail != "" {
		if _, err = s.DB.ExecContext(r.Context(), `UPDATE accounts SET login_email=$2,login_password_secret_ref='do-login-password',password_rotation_status='pending',password_rotation_detail='credentials supplied',updated_at=now() WHERE id=$1`, id, x.LoginEmail); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login_credentials_save_failed", "detail": err.Error()})
			return
		}
		if err = s.Container.Secrets.Put(r.Context(), id, "do-login-password", "digitalocean_login_password", []byte(x.LoginPassword)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login_password_store_failed", "detail": err.Error()})
			return
		}
	}
	if err = s.Container.Secrets.Put(r.Context(), id, "do-token", "digitalocean_token", []byte(x.Token)); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "secret_store_failed", "detail": err.Error()})
		return
	}
	if x.NetworkMode == "proxy_required" {
		collision, _ := networkIdentityCollision(r.Context(), s.DB, id, x.ProxyID)
		if collision {
			writeJSON(w, 409, map[string]string{"error": "network_identity_collision", "detail": "Selected proxy shares an exit IP or subnet with another account"})
			return
		}
	}
	if _, err = s.DB.ExecContext(r.Context(), `INSERT INTO network_profiles(id,account_id,mode,proxy_id) VALUES(gen_random_uuid(),$1,$2,CASE WHEN $2='proxy_required' THEN NULLIF($3,'')::uuid ELSE NULL END) ON CONFLICT (account_id) DO UPDATE SET mode=EXCLUDED.mode,proxy_id=EXCLUDED.proxy_id,updated_at=now()`, id, x.NetworkMode, x.ProxyID); err != nil {
		_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM accounts WHERE id=$1`, id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network_profile_failed", "detail": err.Error()})
		return
	}
	if err := s.syncAccountAutomation(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": "automation_sync_failed", "detail": err.Error()})
		return
	}
	created = false
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func requireAdmin(p auth.Principal) bool { return p.CanAdmin() }
