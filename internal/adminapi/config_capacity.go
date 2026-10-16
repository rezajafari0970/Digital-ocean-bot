package adminapi

import (
	"database/sql"
	"net/http"
)

func (s *Server) configCapacity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rows, e := s.DB.QueryContext(r.Context(), `SELECT pi.id::text,pi.account_id::text,pi.base_url,i.remote_id,i.port,
 u.target_users,u.active_users,u.expired_users,u.quota_exhausted_users,u.deficit,u.created_last_cycle,u.deleted_last_cycle,u.last_error,u.observed_at,
 COALESCE(u.observed_at<=now() AND u.observed_at>now()-interval '30 seconds',false)
 FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=dr.account_id
 JOIN panel_inbound_inventory i ON i.panel_id=pi.id AND i.present AND i.enabled AND i.protocol='vless' AND i.security='reality'
 LEFT JOIN user_capacity_snapshots u ON u.panel_id=pi.id AND u.inbound_id=i.remote_id AND u.observed_at<=now()
 WHERE pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dr.state IN ('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
 AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=dr.id AND d.state='PANEL_COMPLETE')
 ORDER BY pi.base_url,i.port,i.remote_id`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	var target, active, expired, quota, deficit, created int
	var observed, fresh, missing, stale int
	for rows.Next() {
		var panel, account, base string
		var last sql.NullString
		var inbound int64
		var port int
		var t, a, ex, q, d, c, del sql.NullInt64
		var at sql.NullTime
		var current bool
		if err := rows.Scan(&panel, &account, &base, &inbound, &port, &t, &a, &ex, &q, &d, &c, &del, &last, &at, &current); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		item := map[string]any{"panel_id": panel, "account_id": account, "base_url": base, "inbound_id": inbound, "port": port,
			"target": nil, "active": nil, "expired": nil, "quota_exhausted": nil, "deficit": nil, "created_last_cycle": nil, "deleted_last_cycle": nil, "last_error": last.String, "observed_at": nil, "fresh": current, "observation_status": "missing"}
		if at.Valid {
			observed++
			if current {
				fresh++
				item["observation_status"] = "fresh"
			} else {
				stale++
				item["observation_status"] = "stale"
			}
			target += int(t.Int64)
			active += int(a.Int64)
			expired += int(ex.Int64)
			quota += int(q.Int64)
			deficit += int(d.Int64)
			created += int(c.Int64)
			item["target"] = t.Int64
			item["active"] = a.Int64
			item["expired"] = ex.Int64
			item["quota_exhausted"] = q.Int64
			item["deficit"] = d.Int64
			item["created_last_cycle"] = c.Int64
			item["deleted_last_cycle"] = del.Int64
			item["observed_at"] = at.Time
		} else {
			missing++
		}
		items = append(items, item)
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
	writeJSON(w, 200, map[string]any{"automation": automation, "target": target, "active": active, "expired": expired, "quota_exhausted": quota, "deficit": deficit, "created_last_cycle": created, "inbounds": items, "coverage": map[string]any{"eligible": len(items), "observed": observed, "fresh": fresh, "stale": stale, "missing": missing, "complete": missing == 0, "all_fresh": fresh == len(items), "freshness_seconds": 30, "totals_basis": "last_observed"}})
}

// capacityAutomationStatus is authenticated operator metadata. Public Output
// remains a URI-only subscription with unchanged visibility/ownership guards.
func (s *Server) capacityAutomationStatus(r *http.Request) (map[string]any, error) {
	var enabled, kill, lifecycle, autoEnroll, policy bool
	var code, panel, job string
	var scopeLimit int
	var failureAt any
	err := s.DB.QueryRowContext(r.Context(), `SELECT g.enabled,g.kill_switch,c.enabled,c.auto_enroll,
 EXISTS(SELECT 1 FROM global_config_policies WHERE policy_key='reality' AND enabled),
 g.last_failure_code,g.last_failure_at,g.last_failure_panel_id,g.last_failure_job_id,c.max_active_scopes
 FROM client_mutation_execution_gate g CROSS JOIN bulk_lifecycle_control c WHERE g.singleton AND c.singleton`).Scan(&enabled, &kill, &lifecycle, &autoEnroll, &policy, &code, &failureAt, &panel, &job, &scopeLimit)
	if err != nil {
		return nil, err
	}
	var eligible, enrolled, missing, paused int
	err = s.DB.QueryRowContext(r.Context(), `SELECT count(*),
 count(*) FILTER(WHERE sc.enabled AND sc.remaining_operations>0 AND sc.expires_at>now()),
 count(*) FILTER(WHERE sc.panel_id IS NULL),
 count(*) FILTER(WHERE sc.panel_id IS NOT NULL AND NOT(sc.enabled AND sc.remaining_operations>0 AND sc.expires_at>now()))
 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 JOIN panel_inbound_inventory i ON i.panel_id=p.id
 CROSS JOIN global_config_policies pol CROSS JOIN client_mutation_execution_gate g
 LEFT JOIN bulk_lifecycle_scopes sc ON sc.panel_id=p.id AND sc.inbound_id=i.remote_id
 WHERE p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND d.state IN('READY','EXPIRING') AND(d.expires_at IS NULL OR d.expires_at>now()+interval '60 seconds')
 AND EXISTS(SELECT 1 FROM deployments dep WHERE dep.droplet_id=d.id AND dep.state='PANEL_COMPLETE')
 AND i.present AND i.enabled AND i.protocol='vless' AND i.security='reality'
 AND pol.policy_key='reality' AND pol.enabled AND pol.ports @> to_jsonb(ARRAY[i.port])
 AND(g.panel_id IS NULL OR g.panel_id=p.id) AND(g.inbound_id IS NULL OR g.inbound_id=i.remote_id)`).Scan(&eligible, &enrolled, &missing, &paused)
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
		"scope_limit": scopeLimit, "eligible_inbounds": eligible, "enrolled_inbounds": enrolled, "awaiting_enrollment": missing, "paused_scopes": paused,
		"cooldown_panels": cooldown, "quarantined_panels": quarantined, "panels": issues}, nil
}
