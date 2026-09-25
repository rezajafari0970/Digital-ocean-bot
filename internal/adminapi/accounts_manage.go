package adminapi

import (
	"encoding/json"
	"net/http"
)

type accountUpdate struct {
	Name                   string   `json:"name"`
	Token                  string   `json:"token"`
	LoginEmail             string   `json:"login_email"`
	LoginPassword          string   `json:"login_password"`
	Regions                []string `json:"regions"`
	Sizes                  []string `json:"sizes"`
	Image                  string   `json:"image"`
	Images                 []string `json:"images"`
	LifetimeSeconds        int      `json:"lifetime_seconds"`
	LifetimeMinSeconds     int      `json:"lifetime_min_seconds"`
	LifetimeMaxSeconds     int      `json:"lifetime_max_seconds"`
	IntervalSeconds        int      `json:"interval_seconds"`
	BuildSpacingMinutes    int      `json:"build_spacing_minutes"`
	BuildSpacingMaxMinutes int      `json:"build_spacing_max_minutes"`
	BatchSize              int      `json:"batch_size"`
	MaxConcurrent          int      `json:"max_concurrent"`
	DesiredServerCount     int      `json:"desired_server_count"`
	FallbackAnyRegion      *bool    `json:"fallback_any_region"`
	NetworkMode            string   `json:"network_mode"`
	ProxyID                string   `json:"proxy_id"`
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
	if (x.LoginEmail == "") != (x.LoginPassword == "") {
		writeJSON(w, 400, map[string]string{"error": "login_credentials_must_be_provided_together"})
		return
	}
	if x.LifetimeMinSeconds < 1800 {
		x.LifetimeMinSeconds = 1800
	}
	if x.LifetimeMaxSeconds < x.LifetimeMinSeconds {
		x.LifetimeMaxSeconds = x.LifetimeMinSeconds
	}
	x.LifetimeSeconds = x.LifetimeMinSeconds
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
	if x.DesiredServerCount < 1 {
		x.DesiredServerCount = 1
	}
	if x.MaxConcurrent < 1 {
		x.MaxConcurrent = 1
	}
	fallbackAnyRegion := true
	if x.FallbackAnyRegion != nil {
		fallbackAnyRegion = *x.FallbackAnyRegion
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
	regions, _ := json.Marshal(x.Regions)
	sizes, _ := json.Marshal(x.Sizes)
	images, _ := json.Marshal(x.Images)
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
	res, err := tx.ExecContext(r.Context(), `UPDATE accounts SET name=$2,preferred_regions=$3,preferred_region=COALESCE(NULLIF($11,''),preferred_region),preferred_sizes=$4,preferred_images=$5,preferred_image=COALESCE(NULLIF($6,''),preferred_image),server_lifetime_seconds=$7,server_lifetime_min_seconds=$7,server_lifetime_max_seconds=$8,auto_interval_seconds=$9,auto_batch_size=$10,auto_max_concurrent=$12,desired_server_count=$13,fallback_any_region=$14,build_spacing_minutes=$15,build_spacing_max_minutes=$16,next_build_at=NULL,updated_at=now() WHERE id=$1`, id, x.Name, regions, sizes, images, func() string {
		if len(x.Images) > 0 {
			return x.Images[0]
		}
		return x.Image
	}(), x.LifetimeMinSeconds, x.LifetimeMaxSeconds, x.IntervalSeconds, x.BatchSize, func() string {
		if len(x.Regions) > 0 {
			return x.Regions[0]
		}
		return ""
	}(), x.MaxConcurrent, x.DesiredServerCount, fallbackAnyRegion, x.BuildSpacingMinutes, x.BuildSpacingMaxMinutes)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if x.NetworkMode == "proxy_required" {
		var collision bool
		_ = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM network_profiles other_np JOIN proxies chosen ON chosen.id=$2 LEFT JOIN account_network_identities other ON other.account_id=other_np.account_id WHERE other_np.account_id<>$1 AND other_np.mode='proxy_required' AND ((COALESCE(chosen.adapter,'generic')<>'generic' AND other_np.proxy_id=$2) OR (chosen.exit_ip IS NOT NULL AND other.exit_ip=chosen.exit_ip) OR (chosen.exit_ip IS NOT NULL AND other.subnet_key=(CASE WHEN family(chosen.exit_ip)=4 THEN host(network(set_masklen(chosen.exit_ip,24)))||'/24' ELSE host(network(set_masklen(chosen.exit_ip,48)))||'/48' END))))`, id, x.ProxyID).Scan(&collision)
		if collision {
			writeJSON(w, 409, map[string]string{"error": "network_identity_collision", "detail": "Selected proxy shares an exit IP or subnet with another account"})
			return
		}
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
	if x.LoginEmail != "" {
		if _, err := s.DB.ExecContext(r.Context(), `UPDATE accounts SET login_email=$2,login_password_secret_ref='do-login-password',password_rotation_status='pending',password_rotation_detail='credentials supplied',updated_at=now() WHERE id=$1`, id, x.LoginEmail); err != nil {
			writeJSON(w, 500, map[string]string{"error": "login_credentials_save_failed", "detail": err.Error()})
			return
		}
		if err := s.Container.Secrets.Put(r.Context(), id, "do-login-password", "digitalocean_login_password", []byte(x.LoginPassword)); err != nil {
			writeJSON(w, 500, map[string]string{"error": "login_password_store_failed", "detail": err.Error()})
			return
		}
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
