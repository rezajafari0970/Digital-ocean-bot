package adminapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]{3,64}$`)

type panelSettings struct {
	ListenHost string `json:"listen_host"`
	ListenPort int    `json:"listen_port"`
	WebPath    string `json:"web_path"`
}

func (s *Server) getPanelSettings(w http.ResponseWriter, r *http.Request) {
	var x panelSettings
	if err := s.DB.QueryRowContext(r.Context(), `SELECT listen_host,listen_port,web_path FROM panel_settings WHERE id=true`).Scan(&x.ListenHost, &x.ListenPort, &x.WebPath); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, x)
}
func (s *Server) updatePanelSettings(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x panelSettings
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.ListenPort < 1024 || x.ListenPort > 65535 || !pathPattern.MatchString(x.WebPath) {
		writeJSON(w, 400, map[string]string{"error": "invalid_settings"})
		return
	}
	x.WebPath = "/" + strings.Trim(x.WebPath, "/")
	if x.ListenHost == "" {
		x.ListenHost = "0.0.0.0"
	}
	if x.ListenHost != "0.0.0.0" && x.ListenHost != "127.0.0.1" {
		writeJSON(w, 400, map[string]string{"error": "invalid_host"})
		return
	}
	_, err := s.DB.ExecContext(r.Context(), `UPDATE panel_settings SET listen_host=$1,listen_port=$2,web_path=$3,updated_at=now() WHERE id=true`, x.ListenHost, x.ListenPort, x.WebPath)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"saved": true, "restart_required": true, "settings": x})
}
