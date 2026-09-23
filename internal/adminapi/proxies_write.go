package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
	"strings"
)

type proxyWrite struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Type     string `json:"type"`
}

func validProxyWrite(x proxyWrite) bool {
	x.Name = strings.TrimSpace(x.Name)
	x.Host = strings.TrimSpace(x.Host)
	return x.Name != "" && x.Host != "" && x.Port > 0 && x.Port <= 65535
}
func resolveProxyType(r *http.Request, x proxyWrite) (network.ProxyType, error) {
	if x.Type == "" || x.Type == "auto" {
		return detectProxy(r.Context(), x)
	}
	return detectProxyTypes(r.Context(), x, []network.ProxyType{network.ProxyType(x.Type)})
}
func (s *Server) createProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x proxyWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || !validProxyWrite(x) {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	typ, err := resolveProxyType(r, x)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
		return
	}
	var id string
	err = s.DB.QueryRowContext(r.Context(), `INSERT INTO proxies(id,name,type,host,port,username,secret_ref,status) VALUES(gen_random_uuid(),$1,$2,$3,$4,NULLIF($5,''),NULL,'unknown') RETURNING id::text`, x.Name, string(typ), x.Host, x.Port, x.Username).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	cleanup := func() { _, _ = s.DB.ExecContext(r.Context(), `DELETE FROM proxies WHERE id=$1`, id) }
	if x.Password != "" {
		if err = s.Container.Secrets.PutProxy(r.Context(), id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			cleanup()
			writeJSON(w, 500, errorBody())
			return
		}
		if _, err = s.DB.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id); err != nil {
			_ = s.Container.Secrets.DeleteProxy(r.Context(), id, "proxy-password")
			cleanup()
			writeJSON(w, 500, errorBody())
			return
		}
	}
	_, err = s.DB.ExecContext(r.Context(), `UPDATE proxies SET status='healthy',failure_count=0,consecutive_successes=0,last_checked_at=now(),last_success_at=now() WHERE id=$1`, id)
	if err != nil {
		_ = s.Container.Secrets.DeleteProxy(r.Context(), id, "proxy-password")
		cleanup()
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "type": string(typ)})
}

type assignProxy struct {
	ProxyID string `json:"proxy_id"`
	Mode    string `json:"mode"`
}

func (s *Server) assignProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x assignProxy
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	accountID := r.PathValue("id")
	if x.Mode != "direct" && x.Mode != "proxy_required" {
		writeJSON(w, 400, map[string]string{"error": "invalid_mode"})
		return
	}
	if x.Mode == "proxy_required" {
		var status string
		if x.ProxyID == "" || s.DB.QueryRowContext(r.Context(), `SELECT status FROM proxies WHERE id=$1`, x.ProxyID).Scan(&status) != nil {
			writeJSON(w, 400, map[string]string{"error": "proxy_not_found"})
			return
		}
		if status != "healthy" {
			writeJSON(w, 409, map[string]string{"error": "proxy_not_healthy"})
			return
		}
	}
	res, err := s.DB.ExecContext(r.Context(), `UPDATE network_profiles SET mode=$2,proxy_id=CASE WHEN $2='direct' THEN NULL ELSE $3::uuid END,updated_at=now() WHERE account_id=$1`, accountID, x.Mode, x.ProxyID)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "account_not_found"})
		return
	}
	w.WriteHeader(204)
}
