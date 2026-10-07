package adminapi

import (
	"net/http"
)

func (s *Server) configCapacity(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT u.panel_id::text,pi.account_id::text,pi.base_url,u.inbound_id,u.port,u.target_users,u.active_users,u.expired_users,u.quota_exhausted_users,u.deficit,u.created_last_cycle,u.deleted_last_cycle,u.last_error,u.observed_at FROM user_capacity_snapshots u JOIN panel_instances pi ON pi.id=u.panel_id
 JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=dr.account_id
 WHERE pi.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dr.state IN ('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
 AND u.observed_at>now()-interval '30 seconds'
 AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=dr.id AND d.state='PANEL_COMPLETE')
 AND EXISTS(SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=pi.id AND i.remote_id=u.inbound_id AND i.present AND i.enabled)
 ORDER BY pi.base_url,u.port`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	var target, active, expired, quota, deficit, created int
	for rows.Next() {
		var panel, account, base, last string
		var inbound int64
		var port, t, a, ex, q, d, c, del int
		var observed any
		if rows.Scan(&panel, &account, &base, &inbound, &port, &t, &a, &ex, &q, &d, &c, &del, &last, &observed) != nil {
			continue
		}
		target += t
		active += a
		expired += ex
		quota += q
		deficit += d
		created += c
		items = append(items, map[string]any{"panel_id": panel, "account_id": account, "base_url": base, "inbound_id": inbound, "port": port, "target": t, "active": a, "expired": ex, "quota_exhausted": q, "deficit": d, "created_last_cycle": c, "deleted_last_cycle": del, "last_error": last, "observed_at": observed})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	rows.Close()
	automation, err := s.capacityAutomationStatus(r)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"automation": automation, "target": target, "active": active, "expired": expired, "quota_exhausted": quota, "deficit": deficit, "created_last_cycle": created, "inbounds": items})
}

// capacityAutomationStatus is authenticated operator metadata. Public Output
// remains a URI-only subscription with unchanged visibility/ownership guards.
func (s *Server) capacityAutomationStatus(r *http.Request) (map[string]any, error) {
	var enabled, kill, lifecycle, autoEnroll, policy bool
	var code, panel, job string
	var failureAt any
	err := s.DB.QueryRowContext(r.Context(), `SELECT g.enabled,g.kill_switch,c.enabled,c.auto_enroll,
 EXISTS(SELECT 1 FROM global_config_policies WHERE policy_key='reality' AND enabled),
 g.last_failure_code,g.last_failure_at,g.last_failure_panel_id,g.last_failure_job_id
 FROM client_mutation_execution_gate g CROSS JOIN bulk_lifecycle_control c WHERE g.singleton AND c.singleton`).Scan(&enabled, &kill, &lifecycle, &autoEnroll, &policy, &code, &failureAt, &panel, &job)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(r.Context(), `SELECT h.panel_id::text,h.state,h.reason_code,h.retry_after,h.updated_at
 FROM client_mutation_panel_health h JOIN panel_instances p ON p.id=h.panel_id JOIN droplets d ON d.id=p.droplet_id
 WHERE p.enabled AND d.state IN('READY','EXPIRING') AND(d.expires_at IS NULL OR d.expires_at>now())
 ORDER BY h.updated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []map[string]any{}
	cooldown, quarantined := 0, 0
	for rows.Next() {
		var id, state, reason string
		var retry, updated any
		if err := rows.Scan(&id, &state, &reason, &retry, &updated); err != nil {
			return nil, err
		}
		if state == "QUARANTINED" {
			quarantined++
		} else {
			cooldown++
		}
		issues = append(issues, map[string]any{"panel_id": id, "state": state, "reason": reason, "retry_after": retry, "updated_at": updated})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"running": enabled && !kill && lifecycle && autoEnroll && policy,
		"gate_enabled": enabled && !kill, "policy_enabled": policy, "lifecycle_enabled": lifecycle && autoEnroll,
		"last_failure_code": code, "last_failure_at": failureAt, "last_failure_panel_id": panel, "last_failure_job_id": job,
		"cooldown_panels": cooldown, "quarantined_panels": quarantined, "panels": issues}, nil
}
