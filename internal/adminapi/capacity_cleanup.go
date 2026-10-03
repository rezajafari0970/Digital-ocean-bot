package adminapi

import (
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"net/http"
)

func (s *Server) deleteAllCapacityClients(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id, err := (cleanup.Store{DB: s.DB}).Start(r.Context(), []string{})
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "cleanup_not_started", "detail": "Creation freeze or cleanup planning could not be committed; retry after current operations finish."})
		return
	}
	status, err := (cleanup.Store{DB: s.DB}).Status(r.Context(), id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 202, map[string]any{"job_id": id, "status": status.Status, "total": status.Total})
}
func (s *Server) cleanupJobStatus(w http.ResponseWriter, r *http.Request) {
	status, err := (cleanup.Store{DB: s.DB}).Status(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, status)
}
