package adminapi

import (
	"net/http"
	"time"
)

func (s *Server) buildActivity(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT d.id::text,a.name,d.state,d.current_step,d.attempt,COALESCE(d.last_error,''),d.created_at,d.updated_at,COALESCE((SELECT de.message FROM deployment_events de WHERE de.deployment_id=d.id ORDER BY de.created_at DESC LIMIT 1),'') FROM deployments d JOIN accounts a ON a.id=d.account_id ORDER BY d.updated_at DESC LIMIT 100`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, state, step, lastErr, msg string
		var attempt int
		var created, updated time.Time
		if rows.Scan(&id, &name, &state, &step, &attempt, &lastErr, &created, &updated, &msg) == nil {
			out = append(out, map[string]any{"id": id, "account": name, "state": state, "step": step, "attempt": attempt, "error": lastErr, "message": msg, "created_at": created, "updated_at": updated})
		}
	}
	writeJSON(w, 200, out)
}
