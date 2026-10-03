package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type globalConfigRequest struct {
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
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if len(x.Ports) == 0 || x.TargetUsersPerInbound < 0 || x.TargetUsersPerInbound > 10000 || x.UsersPerSecond > 100 || x.DeviceLimit < 0 || x.UsersPerSecond < 1 || (x.SNISelectionMode != "scored" && x.SNISelectionMode != "manual") {
		writeJSON(w, 400, map[string]string{"error": "invalid_config"})
		return
	}
	res, direct := true, false
	if err := s.DB.QueryRowContext(r.Context(), "SELECT generate_residential,generate_direct FROM global_config_policies WHERE policy_key='reality'").Scan(&res, &direct); err != nil && err != sql.ErrNoRows {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.GenerateResidential != nil {
		res = *x.GenerateResidential
	}
	if x.GenerateDirect != nil {
		direct = *x.GenerateDirect
	}
	if x.Enabled && (!res && !direct || res && direct && x.TargetUsersPerInbound == 1) {
		writeJSON(w, 400, map[string]string{"error": "invalid_route_classes", "detail": "Select at least one route class; two classes require at least two users per inbound."})
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
	var cleaning bool
	if e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED'))").Scan(&cleaning); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if cleaning && x.Enabled {
		writeJSON(w, 409, map[string]string{"error": "cleanup_in_progress", "detail": "Cleanup is paused or running. Resume it, or cancel remaining cleanup from Configs before enabling creation."})
		return
	}
	ports, _ := json.Marshal(x.Ports)
	snis, _ := json.Marshal(x.ManualSNIs)
	_, e = tx.ExecContext(r.Context(), `INSERT INTO global_config_policies(policy_key,enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second,sni_selection_mode,manual_snis,generate_residential,generate_direct) VALUES('reality',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(policy_key) DO UPDATE SET revision=global_config_policies.revision+1,enabled=excluded.enabled,ports=excluded.ports,target_users_per_inbound=excluded.target_users_per_inbound,user_quota_bytes=excluded.user_quota_bytes,user_lifetime_seconds=excluded.user_lifetime_seconds,device_limit=excluded.device_limit,users_per_second=excluded.users_per_second,sni_selection_mode=excluded.sni_selection_mode,manual_snis=excluded.manual_snis,generate_residential=excluded.generate_residential,generate_direct=excluded.generate_direct,updated_at=now()`, x.Enabled, ports, x.TargetUsersPerInbound, quota, life, x.DeviceLimit, x.UsersPerSecond, x.SNISelectionMode, snis, res, direct)
	if e != nil {
		writeJSON(w, 500, map[string]string{"error": "db_error"})
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"policy_key": "reality", "inbound_count": len(x.Ports), "user_quota_bytes": quota, "user_lifetime_seconds": life})
}

func (s *Server) getGlobalConfigs(w http.ResponseWriter, r *http.Request) {
	row := s.DB.QueryRowContext(r.Context(), `SELECT enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second,sni_selection_mode,manual_snis,revision,generate_residential,generate_direct FROM global_config_policies WHERE policy_key='reality'`)
	var enabled, residential, direct bool
	var portsRaw, snisRaw []byte
	var target, life, devices, rate int
	var quota, rev int64
	var mode string
	if e := row.Scan(&enabled, &portsRaw, &target, &quota, &life, &devices, &rate, &mode, &snisRaw, &rev, &residential, &direct); e != nil {
		writeJSON(w, 200, []any{})
		return
	}
	var ports []int
	var snis []string
	_ = json.Unmarshal(portsRaw, &ports)
	_ = json.Unmarshal(snisRaw, &snis)
	writeJSON(w, 200, []any{map[string]any{"policy_key": "reality", "enabled": enabled, "generate_residential": residential, "generate_direct": direct, "ports": ports, "inbound_count": len(ports), "target_users_per_inbound": target, "user_quota_bytes": quota, "user_lifetime_seconds": life, "device_limit": devices, "users_per_second": rate, "sni_selection_mode": mode, "manual_snis": snis, "revision": rev}})
}
