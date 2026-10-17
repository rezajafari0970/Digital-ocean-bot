package adminapi

import (
	"encoding/json"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"net/http"
)

func (s *Server) getTrojanConfig(w http.ResponseWriter, r *http.Request) {
	p, e := trojantcp.ReadPolicy(r.Context(), s.DB)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	rows, e := s.DB.QueryContext(r.Context(), `SELECT t.panel_id::text,t.port,t.state,t.stage,t.error_code,t.certificate_not_after,t.verified_at
 FROM trojan_tcp_listeners t JOIN panel_instances pi ON pi.id=t.panel_id JOIN droplets dr ON dr.id=pi.droplet_id
 WHERE dr.state IN('READY','EXPIRING') ORDER BY t.state,t.panel_id`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	listeners := []map[string]any{}
	for rows.Next() {
		var panel, state, stage, code string
		var port int
		var until, verified any
		if e = rows.Scan(&panel, &port, &state, &stage, &code, &until, &verified); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		listeners = append(listeners, map[string]any{"panel_id": panel, "port": port, "state": state, "stage": stage, "error": code, "certificate_not_after": until, "verified_at": verified})
	}
	if rows.Err() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"policy": p, "listeners": listeners, "certificate_mode": "automatic_ip_tls", "configs_per_class_per_server": 1})
}
func (s *Server) putTrojanConfig(w http.ResponseWriter, r *http.Request) {
	var p trojantcp.Policy
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	dec.DisallowUnknownFields()
	if dec.Decode(&p) != nil || trojantcp.ValidatePort(p.Port, 0, nil, nil) != nil || p.Enabled && (!p.Direct && !p.Residential || !p.Fleet && len(p.Panels) == 0) || len(p.Panels) > 1000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_trojan_policy"})
		return
	}
	if p.Panels == nil {
		p.Panels = []string{}
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	for _, lock := range []int64{628341902732, 180061015} {
		if _, e = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock($1)", lock); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	current, e := trojantcp.ReadPolicy(r.Context(), tx)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Revision != current.Revision {
		writeJSON(w, 409, map[string]string{"error": "trojan_policy_changed", "detail": "Reload Trojan settings before saving."})
		return
	}
	var busy, exists, conflict bool
	if e = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED')),EXISTS(SELECT 1 FROM trojan_tcp_listeners WHERE state<>'RETIRED')").Scan(&busy, &exists); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if p.Enabled && busy {
		writeJSON(w, 409, map[string]string{"error": "cleanup_in_progress"})
		return
	}
	if exists && (p.Port != current.Port || p.Direct != current.Direct || p.Residential != current.Residential) {
		writeJSON(w, 409, map[string]string{"error": "trojan_listener_settings_pinned", "detail": "Existing Trojan listener ports and client classes are preserved. Complete the explicit config cleanup before changing them."})
		return
	}
	if p.Enabled {
		e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') rp CROSS JOIN LATERAL jsonb_array_elements_text(rp.ports) port WHERE port::integer=$1)
  OR EXISTS(SELECT 1 FROM panel_inbound_inventory i JOIN panel_instances pi ON pi.id=i.panel_id JOIN droplets dr ON dr.id=pi.droplet_id WHERE i.present AND i.port=$1 AND dr.state IN('READY','EXPIRING') AND NOT EXISTS(SELECT 1 FROM trojan_tcp_listeners t WHERE t.panel_id=i.panel_id AND t.remote_id=i.remote_id AND i.protocol='trojan' AND i.remark=t.managed_key))
  OR EXISTS(SELECT 1 FROM xui_panel_deployments x JOIN droplets dr ON dr.id=x.droplet_id WHERE x.port=$1 AND dr.state IN('READY','EXPIRING'))`, p.Port).Scan(&conflict)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if conflict {
			writeJSON(w, 409, map[string]string{"error": "trojan_port_conflict", "detail": "This port is reserved by Reality, a panel, or another inbound."})
			return
		}
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE trojan_tcp_policy SET enabled=$1,port=$2,direct_enabled=$3,residential_enabled=$4,fleet=$5,panel_ids=$6,revision=revision+1,updated_at=now() WHERE singleton`, p.Enabled, p.Port, p.Direct, p.Residential, p.Fleet, pq.Array(p.Panels))
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "trojan_policy_rejected"})
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 409, map[string]string{"error": "trojan_outcome_unconfirmed", "detail": "Reload to verify the saved policy before retrying."})
		return
	}
	p.Revision++
	writeJSON(w, 200, map[string]any{"policy": p})
}
