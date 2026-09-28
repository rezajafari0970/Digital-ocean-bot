package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/policy"
	"net/http"
)

type configRequest struct {
	PanelID             string   `json:"panel_id"`
	PolicyKey           string   `json:"policy_key"`
	Enabled             bool     `json:"enabled"`
	DesiredCount        int      `json:"desired_count"`
	PreferredPorts      []int    `json:"preferred_ports"`
	DynamicPortStart    int      `json:"dynamic_port_start"`
	DynamicPortEnd      int      `json:"dynamic_port_end"`
	ReservedPorts       []int    `json:"reserved_ports"`
	ClientsPerInbound   int      `json:"clients_per_inbound"`
	UserQuotaBytes      int64    `json:"user_quota_bytes"`
	UserLifetimeSeconds int      `json:"user_lifetime_seconds"`
	DeviceLimit         int      `json:"device_limit"`
	BulkUserCount       int      `json:"bulk_user_count"`
	UsersPerSecond      int      `json:"users_per_second"`
	SNISelectionMode    string   `json:"sni_selection_mode"`
	ManualSNIs          []string `json:"manual_snis"`
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var x configRequest
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.PolicyKey == "" {
		x.PolicyKey = "reality"
	}
	p := policy.Normalize(policy.InboundPolicy{ID: x.PolicyKey, PanelID: x.PanelID, Enabled: x.Enabled, DesiredCount: x.DesiredCount, Protocol: "vless", Transport: "tcp", Security: "reality", Listen: "", PreferredPorts: x.PreferredPorts, DynamicPortStart: x.DynamicPortStart, DynamicPortEnd: x.DynamicPortEnd, ReservedPorts: x.ReservedPorts, ClientsPerInbound: x.ClientsPerInbound, UserQuotaBytes: x.UserQuotaBytes, UserLifetimeSeconds: x.UserLifetimeSeconds, DeviceLimit: x.DeviceLimit, BulkUserCount: x.BulkUserCount, UsersPerSecond: x.UsersPerSecond, SNISelectionMode: x.SNISelectionMode, ManualSNIs: x.ManualSNIs})
	if _, err := (policy.SQLStore{DB: s.DB}).Put(r.Context(), p); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_config", "detail": err.Error()})
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) getConfigs(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT p.panel_id::text,pi.driver,pi.base_url,p.policy_key,p.enabled,p.desired_count,p.preferred_ports,p.dynamic_port_start,p.dynamic_port_end,p.reserved_ports,p.clients_per_inbound,p.user_quota_bytes,p.user_lifetime_seconds,p.device_limit,p.bulk_user_count,p.users_per_second,p.sni_selection_mode,p.manual_snis,p.revision FROM panel_inbound_policies p JOIN panel_instances pi ON pi.id=p.panel_id ORDER BY pi.created_at,p.policy_key`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "db_error"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var panelID, driver, baseURL, key, sni string
		var enabled bool
		var desired, start, end, clients, lifetime, devices, bulk, rate int
		var quota int64
		var preferred, reserved, manual []byte
		var revision int64
		if rows.Scan(&panelID, &driver, &baseURL, &key, &enabled, &desired, &preferred, &start, &end, &reserved, &clients, &quota, &lifetime, &devices, &bulk, &rate, &sni, &manual, &revision) != nil {
			continue
		}
		var pp, rr []int
		var ss []string
		_ = json.Unmarshal(preferred, &pp)
		_ = json.Unmarshal(reserved, &rr)
		_ = json.Unmarshal(manual, &ss)
		out = append(out, map[string]any{"panel_id": panelID, "driver": driver, "base_url": baseURL, "policy_key": key, "enabled": enabled, "desired_count": desired, "preferred_ports": pp, "dynamic_port_start": start, "dynamic_port_end": end, "reserved_ports": rr, "clients_per_inbound": clients, "user_quota_bytes": quota, "user_lifetime_seconds": lifetime, "device_limit": devices, "bulk_user_count": bulk, "users_per_second": rate, "sni_selection_mode": sni, "manual_snis": ss, "revision": revision})
	}
	writeJSON(w, 200, out)
}

func (s *Server) getConfigPanels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,driver,base_url,enabled FROM panel_instances ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "db_error"})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, driver, base string
		var enabled bool
		if rows.Scan(&id, &driver, &base, &enabled) == nil {
			out = append(out, map[string]any{"id": id, "driver": driver, "base_url": base, "enabled": enabled})
		}
	}
	writeJSON(w, 200, out)
}
