package adminapi

import (
	"encoding/json"
	"net/http"
)

type proxyWrite struct {
	Name     string `json:"name"`
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
	typ, err := detectProxy(r.Context(), x)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
		return
	}
	var id string
	err = s.DB.QueryRowContext(r.Context(), `INSERT INTO proxies(id,name,type,host,port,username,secret_ref) VALUES(gen_random_uuid(),$1,$2,$3,$4,NULLIF($5,''),NULL) RETURNING id::text`, x.Name, string(typ), x.Host, x.Port, x.Username).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	if x.Password != "" {
		if err := s.Container.Secrets.PutProxy(r.Context(), id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			_, _ = s.DB.ExecContext(r.Context(), `DELETE FROM proxies WHERE id=$1`, id)
			writeJSON(w, 500, errorBody())
			return
		}
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password',status='healthy',last_checked_at=now(),last_success_at=now() WHERE id=$1`, id)
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
	_, err := s.DB.ExecContext(r.Context(), `UPDATE network_profiles SET mode=$2,proxy_id=CASE WHEN $2='direct' THEN NULL ELSE NULLIF($3,'')::uuid END,updated_at=now() WHERE account_id=$1`, accountID, x.Mode, x.ProxyID)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	w.WriteHeader(204)
}
