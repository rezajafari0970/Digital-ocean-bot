package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9_-]{3,64}$`)
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]{3,64}$`)

type panelSettings struct {
	ListenPort int    `json:"listen_port"`
	WebPath    string `json:"web_path"`
	Username   string `json:"username"`
}

func (s *Server) getPanelSettings(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x panelSettings
	if err := s.DB.QueryRowContext(r.Context(), `SELECT listen_port,web_path FROM panel_settings WHERE id=true`).Scan(&x.ListenPort, &x.WebPath); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	x.Username = p.Username
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, x)
}
func (s *Server) updatePanelSettings(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var req struct {
		panelSettings
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || req.ListenPort < 1024 || req.ListenPort > 65535 || !pathPattern.MatchString(req.WebPath) || reservedPanelPath(req.WebPath) || !usernamePattern.MatchString(req.Username) {
		writeJSON(w, 400, map[string]string{"error": "invalid_settings"})
		return
	}
	if req.Password != "" && (len(req.Password) < 12 || len(req.Password) > 72) {
		writeJSON(w, 400, map[string]string{"error": "password_must_be_12_to_72_bytes"})
		return
	}
	if len(req.CurrentPassword) > 72 {
		writeJSON(w, 400, map[string]string{"error": "invalid_current_password"})
		return
	}
	var nextHash string
	if req.Password != "" {
		var err error
		nextHash, err = auth.HashPassword(req.Password)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_password"})
			return
		}
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var username, hash, role string
	err = tx.QueryRowContext(r.Context(), `SELECT username,password_hash,role FROM admin_users WHERE id=$1 AND enabled FOR UPDATE`, p.UserID).Scan(&username, &hash, &role)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && role != "admin") {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	credentialChange := req.Username != username || req.Password != ""
	if credentialChange && bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_current_password"})
		return
	}
	var priorPort int
	var priorPath string
	if err = tx.QueryRowContext(r.Context(), `SELECT listen_port,web_path FROM panel_settings WHERE id=true FOR UPDATE`).Scan(&priorPort, &priorPath); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	networkChange := priorPort != req.ListenPort || priorPath != req.WebPath
	if networkChange {
		state, err := s.updateController().Request(r.Context(), "status")
		if err != nil || !state.Available {
			writeJSON(w, 503, map[string]string{"error": "update_control_unavailable"})
			return
		}
		if state.Busy {
			writeJSON(w, 409, map[string]string{"error": "deployment_busy"})
			return
		}
		if req.ListenPort != priorPort {
			listener, err := net.Listen("tcp4", "0.0.0.0:"+strconv.Itoa(req.ListenPort))
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "port_unavailable"})
				return
			}
			listener.Close()
		}
	}
	if credentialChange {
		if nextHash == "" {
			nextHash = hash
		}
		_, err = tx.ExecContext(r.Context(), `UPDATE admin_users SET username=$2,password_hash=$3,must_change_password=false,bootstrap_token_hash=NULL,updated_at=now() WHERE id=$1`, p.UserID, req.Username, nextHash)
		if err != nil {
			var pg *pq.Error
			if errors.As(err, &pg) && pg.Code == "23505" {
				writeJSON(w, 409, map[string]string{"error": "username_in_use"})
			} else {
				writeJSON(w, 500, errorBody())
			}
			return
		}
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM admin_sessions WHERE user_id=$1`, p.UserID); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE panel_settings SET listen_host='0.0.0.0',listen_port=$1,web_path=$2,updated_at=now() WHERE id=true`, req.ListenPort, req.WebPath); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	reply := map[string]any{"saved": true, "credentials_changed": credentialChange, "settings": req.panelSettings, "restart_required": networkChange, "restart_queued": false}
	if networkChange {
		_, err = s.updateController().Request(r.Context(), "restart_api")
		if err == nil {
			reply["restart_queued"] = true
		} else {
			reply["restart_error"] = err.Error()
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, reply)
}

func reservedPanelPath(path string) bool {
	switch path {
	case "/api", "/static", "/readyz", "/healthz", "/version", "/share", "/vultr-browser":
		return true
	}
	return false
}
