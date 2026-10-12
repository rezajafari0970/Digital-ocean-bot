package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

type updateState struct {
	Available         bool   `json:"available"`
	Busy              bool   `json:"busy"`
	Status            string `json:"status"`
	Stage             int    `json:"stage"`
	Message           string `json:"message,omitempty"`
	JobID             string `json:"job_id,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	EndedAt           string `json:"ended_at,omitempty"`
	FromRevision      string `json:"from_revision,omitempty"`
	InstalledRevision string `json:"installed_revision,omitempty"`
	Error             string `json:"error,omitempty"`
	RecoveryRequired  bool   `json:"recovery_required,omitempty"`
}
type updateController interface {
	Request(context.Context, string) (updateState, error)
}
type unixUpdateController struct{ Path string }

var errUpdateUnavailable = errors.New("update_control_unavailable")

func (c unixUpdateController) Request(ctx context.Context, action string) (updateState, error) {
	path := c.Path
	if path == "" {
		path = "/run/datacenter-bot/update.sock"
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return updateState{}, errUpdateUnavailable
	}
	defer conn.Close()
	deadline := time.Now().Add(8 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]string{"action": action}); err != nil {
		return updateState{}, errUpdateUnavailable
	}
	var reply struct {
		OK    bool        `json:"ok"`
		State updateState `json:"state"`
		Error string      `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(conn, 16384)).Decode(&reply); err != nil {
		return updateState{}, errUpdateUnavailable
	}
	if !reply.OK {
		switch reply.Error {
		case "deployment_busy", "recovery_required":
			return updateState{}, errors.New(reply.Error)
		}
		return updateState{}, errUpdateUnavailable
	}
	return reply.State, nil
}
func (s *Server) updateController() updateController {
	if s.Updater != nil {
		return s.Updater
	}
	return unixUpdateController{}
}
func (s *Server) projectUpdateStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	state, err := s.updateController().Request(r.Context(), "status")
	if err != nil {
		writeJSON(w, 200, updateState{Status: "unavailable", Error: "update_control_unavailable"})
		return
	}
	writeJSON(w, 200, state)
}
func (s *Server) startProjectUpdate(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	// This operation accepts no shell arguments, release URL, environment or paths.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128))
	if err != nil || (len(body) > 0 && string(body) != "{}") {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	state, err := s.updateController().Request(r.Context(), "start")
	if err != nil {
		code := 503
		if err.Error() == "deployment_busy" || err.Error() == "recovery_required" {
			code = 409
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 202, state)
}
func (s *Server) applyPanelSettings(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	state, err := s.updateController().Request(r.Context(), "restart_api")
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, state)
}
