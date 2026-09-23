package adminapi

import (
	"encoding/json"
	"net/http"
)

type proxyWrite struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) createProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x proxyWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Host == "" || x.Port < 1 || x.Port > 65535 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.Type != "http" && x.Type != "https" && x.Type != "socks5" {
		writeJSON(w, 400, map[string]string{"error": "invalid_proxy_type"})
		return
	}
	var id string
	err := s.DB.QueryRowContext(r.Context(), `INSERT INTO proxies(id,name,type,host,port,username,secret_ref) VALUES(gen_random_uuid(),$1,$2,$3,$4,NULLIF($5,''),NULL) RETURNING id::text`, x.Name, x.Type, x.Host, x.Port, x.Username).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
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
	_, err := s.DB.ExecContext(r.Context(), `UPDATE network_profiles SET mode=$2,proxy_id=CASE WHEN $2='direct' THEN NULL ELSE NULLIF($3,'')::uuid END,updated_at=now() WHERE account_id=$1`, accountID, x.Mode, x.ProxyID)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	w.WriteHeader(204)
}
