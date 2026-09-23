package adminapi

import "net/http"

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,account_id::text,name,version,enabled,created_at,updated_at FROM deployment_profiles ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, account, name string
		var version int
		var enabled bool
		var created, updated any
		if rows.Scan(&id, &account, &name, &version, &enabled, &created, &updated) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "account_id": account, "name": name, "version": version, "enabled": enabled, "created_at": created, "updated_at": updated})
	}
	writeJSON(w, 200, out)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id,COALESCE(account_id::text,''),actor,action,resource_type,COALESCE(resource_id,''),result,message,created_at FROM audit_events ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var account, actor, action, typ, rid, result, message string
		var at any
		if rows.Scan(&id, &account, &actor, &action, &typ, &rid, &result, &message, &at) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "account_id": account, "actor": actor, "action": action, "resource_type": typ, "resource_id": rid, "result": result, "message": message, "created_at": at})
	}
	writeJSON(w, 200, out)
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	var accounts, deployments, workers int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM accounts WHERE enabled=true`).Scan(&accounts)
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM deployments WHERE state NOT IN ('READY','FAILED')`).Scan(&deployments)
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM worker_heartbeats WHERE last_seen_at>now()-interval '30 seconds'`).Scan(&workers)
	writeJSON(w, 200, map[string]any{"enabled_accounts": accounts, "active_deployments": deployments, "live_workers": workers})
}
