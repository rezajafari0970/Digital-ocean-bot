package adminapi

import (
	"encoding/json"
	"net/http"
)

func (s *Server) updateProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
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
	res, err := s.DB.ExecContext(r.Context(), `UPDATE proxies SET name=$2,type=$3,host=$4,port=$5,username=NULLIF($6,''),status='healthy',last_checked_at=now(),last_success_at=now(),updated_at=now() WHERE id=$1`, id, x.Name, string(typ), x.Host, x.Port, x.Username)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if x.Password != "" {
		if err := s.Container.Secrets.PutProxy(r.Context(), id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	writeJSON(w, 200, map[string]string{"type": string(typ)})
}
func (s *Server) deleteProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var used int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM network_profiles WHERE proxy_id=$1`, id).Scan(&used)
	if used > 0 {
		writeJSON(w, 409, map[string]string{"error": "proxy_in_use"})
		return
	}
	res, err := s.DB.ExecContext(r.Context(), `DELETE FROM proxies WHERE id=$1`, id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	w.WriteHeader(204)
}
