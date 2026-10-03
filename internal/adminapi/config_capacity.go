package adminapi

import (
	"net/http"
)

func (s *Server) configCapacity(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT u.panel_id::text,pi.account_id::text,pi.base_url,u.inbound_id,u.port,u.target_users,u.active_users,u.expired_users,u.quota_exhausted_users,u.deficit,u.created_last_cycle,u.deleted_last_cycle,u.last_error,u.observed_at FROM user_capacity_snapshots u JOIN panel_instances pi ON pi.id=u.panel_id ORDER BY pi.base_url,u.port`)
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
	writeJSON(w, 200, map[string]any{"target": target, "active": active, "expired": expired, "quota_exhausted": quota, "deficit": deficit, "created_last_cycle": created, "inbounds": items})
}
