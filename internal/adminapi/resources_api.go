package adminapi

import "net/http"

func (s *Server) accountResources(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,provider_resource_id,type,state,managed,metadata,created_at,updated_at FROM resources WHERE account_id=$1 ORDER BY updated_at DESC LIMIT 1000`, id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var rid, pid, typ, state string
		var managed bool
		var metadata []byte
		var created, updated any
		if rows.Scan(&rid, &pid, &typ, &state, &managed, &metadata, &created, &updated) != nil {
			continue
		}
		out = append(out, map[string]any{"id": rid, "provider_id": pid, "type": typ, "state": state, "managed": managed, "metadata": string(metadata), "created_at": created, "updated_at": updated})
	}
	writeJSON(w, 200, out)
}

func (s *Server) accountCapacity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_not_ready"})
		return
	}
	limits, err := runtime.Provider.GetLimits(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "provider_limits_failed"})
		return
	}
	var active, pending int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM resources WHERE account_id=$1 AND type='droplet' AND state='active'`, id).Scan(&active)
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state IN ('planned','running','verifying','unknown')`, id).Scan(&pending)
	available := limits.DropletLimit - active - pending
	if available < 0 {
		available = 0
	}
	writeJSON(w, 200, map[string]any{"droplet_limit": limits.DropletLimit, "active": active, "pending": pending, "available": available})
}
