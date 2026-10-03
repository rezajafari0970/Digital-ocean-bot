package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo"
	"net/http"
)

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
	var raw []byte
	err := s.DB.QueryRowContext(r.Context(), dashboardCountsSQL).Scan(&raw)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(raw)

}

func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, buildinfo.Current())
}
