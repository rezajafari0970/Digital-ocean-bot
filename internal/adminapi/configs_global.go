package adminapi

import (
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"net/http"
)

type globalConfigRequest struct {
	RouteClass             string   `json:"route_class"`
	ProfileRevision        int64    `json:"profile_revision"`
	GenerateResidential    *bool    `json:"generate_residential"`
	GenerateDirect         *bool    `json:"generate_direct"`
	Enabled                bool     `json:"enabled"`
	Ports                  []int    `json:"ports"`
	TargetUsersPerInbound  int      `json:"target_users_per_inbound"`
	UserQuotaExpression    string   `json:"user_quota_expression"`
	UserLifetimeExpression string   `json:"user_lifetime_expression"`
	DeviceLimit            int      `json:"device_limit"`
	UsersPerSecond         int      `json:"users_per_second"`
	SNISelectionMode       string   `json:"sni_selection_mode"`
	ManualSNIs             []string `json:"manual_snis"`
}

func (s *Server) putGlobalConfig(w http.ResponseWriter, r *http.Request) {
	var x globalConfigRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if field, detail := validateGlobalConfig(x); detail != "" {
		writeJSON(w, 400, map[string]string{"error": "invalid_config", "field": field, "detail": detail})
		return
	}
	if x.RouteClass != "DIRECT" && x.RouteClass != "RESIDENTIAL" {
		writeJSON(w, 400, map[string]string{"error": "invalid_route_class", "detail": "Choose Direct or Residential. Each has its own policy."})
		return
	}
	seen := map[int]bool{}
	for _, p := range x.Ports {
		if p < 1 || p > 65535 || seen[p] {
			writeJSON(w, 400, map[string]string{"error": "invalid_ports"})
			return
		}
		seen[p] = true
	}
	quota, e := parseQuotaMB(x.UserQuotaExpression)
	if e != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_quota_expression"})
		return
	}
	life, e := parseLifetimeMinutes(x.UserLifetimeExpression)
	if e != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_lifetime_expression"})
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock(628341902732)"); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	initialPorts, _ := json.Marshal(x.Ports)
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO global_config_policies(policy_key,enabled,ports) VALUES('reality',false,$1) ON CONFLICT(policy_key) DO NOTHING`, initialPorts); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var master bool
	var mode string
	var snis []byte
	if e = tx.QueryRowContext(r.Context(), "SELECT enabled,sni_selection_mode,manual_snis FROM global_config_policies WHERE policy_key='reality' FOR UPDATE").Scan(&master, &mode, &snis); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	// Never overwrite the other class's shared Reality listener settings.
	var shared []string
	_ = json.Unmarshal(snis, &shared)
	if x.SNISelectionMode != mode || !equalSNIs(shared, x.ManualSNIs) {
		writeJSON(w, 409, map[string]string{"error": "shared_listener_changed", "detail": "Shared listener SNI settings changed. Reload the profiles before saving."})
		return
	}
	var cleaning bool
	if e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED'))").Scan(&cleaning); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if cleaning {
		writeJSON(w, 409, map[string]string{"error": "cleanup_in_progress", "detail": "Finish or cancel the saved cleanup before enabling creation."})
		return
	}
	profiles, e := configprofiles.Read(r.Context(), tx, true)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	current := int64(0)
	for _, p := range profiles {
		if p.Class == x.RouteClass {
			current = p.Revision
			continue
		}
		if x.Enabled && p.Enabled {
			for _, port := range x.Ports {
				if p.Applies(port) && p.Target+x.TargetUsersPerInbound > 10000 {
					writeJSON(w, 400, map[string]string{"error": "inbound_capacity_limit", "detail": "Combined Direct and Residential configs on one port cannot exceed 10000."})
					return
				}
			}
		}
	}
	if current != x.ProfileRevision {
		writeJSON(w, 409, map[string]string{"error": "profile_changed", "detail": "This profile already exists or has changed. Reload it before saving."})
		return
	}
	ports, _ := json.Marshal(x.Ports)
	_, e = tx.ExecContext(r.Context(), `INSERT INTO reality_config_profiles(route_class,enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(route_class) DO UPDATE SET enabled=excluded.enabled,ports=excluded.ports,target_users_per_inbound=excluded.target_users_per_inbound,user_quota_bytes=excluded.user_quota_bytes,user_lifetime_seconds=excluded.user_lifetime_seconds,device_limit=excluded.device_limit,users_per_second=excluded.users_per_second,revision=reality_config_profiles.revision+1,updated_at=now()`, x.RouteClass, x.Enabled, ports, x.TargetUsersPerInbound, quota, life, x.DeviceLimit, x.UsersPerSecond)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if !master && !x.Enabled {
		if _, e = tx.ExecContext(r.Context(), `UPDATE global_config_policies SET enabled=false WHERE policy_key='reality'`); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 409, map[string]string{"error": "profile_outcome_unconfirmed", "detail": "Reload the profile to verify the saved state before retrying."})
		return
	}
	writeJSON(w, 200, map[string]any{"route_class": x.RouteClass, "profile_revision": current + 1})
}

func equalSNIs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Server) getGlobalConfigs(w http.ResponseWriter, r *http.Request) {
	profiles, e := configprofiles.Read(r.Context(), s.DB, false)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var master bool
	var mode string
	var snis json.RawMessage
	var rev int64
	e = s.DB.QueryRowContext(r.Context(), `SELECT enabled,sni_selection_mode,manual_snis,revision FROM global_config_policies WHERE policy_key='reality'`).Scan(&master, &mode, &snis, &rev)
	if e == sql.ErrNoRows {
		writeJSON(w, 200, []any{})
		return
	}
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	type view struct {
		configprofiles.Profile
		MasterEnabled bool            `json:"master_enabled"`
		Revision      int64           `json:"revision"`
		SNI           string          `json:"sni_selection_mode"`
		ManualSNIs    json.RawMessage `json:"manual_snis"`
		InboundCount  int             `json:"inbound_count"`
	}
	result := []view{}
	for _, p := range profiles {
		result = append(result, view{p, master, rev, mode, snis, len(p.Ports)})
	}
	writeJSON(w, 200, result)
}

func validateGlobalConfig(x globalConfigRequest) (string, string) {
	switch {
	case x.UsersPerSecond < 1 || x.UsersPerSecond > 100:
		return "users_per_second", "Configs created per second must be a whole number from 1 to 100. This does not limit user connections."
	case x.TargetUsersPerInbound < 0 || x.TargetUsersPerInbound > 10000:
		return "target_users_per_inbound", "Configs per port for this profile must be from 0 to 10000."
	case x.DeviceLimit < 0 || x.DeviceLimit > 10000:
		return "device_limit", "Device limit must be from 0 to 10000."
	case len(x.Ports) == 0:
		return "ports", "Enter at least one port."
	case x.SNISelectionMode != "scored" && x.SNISelectionMode != "manual":
		return "sni_selection_mode", "Select automatic or manual SNI selection."
	}
	return "", ""
}
