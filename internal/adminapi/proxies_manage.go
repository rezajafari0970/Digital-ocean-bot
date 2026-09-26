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
	if json.NewDecoder(r.Body).Decode(&x) != nil || !validProxyWrite(x) {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	if x.Password == "" {
		if b, err := s.Container.Secrets.GetProxy(r.Context(), id, "proxy-password"); err == nil {
			x.Password = string(b)
			defer zeroBytes(b)
		}
	}
	typ, err := resolveProxyType(r, x)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
		return
	}
	if x.Adapter == "" {
		_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(adapter,'generic') FROM proxies WHERE id=$1`, id).Scan(&x.Adapter)
		if x.Adapter == "" {
			x.Adapter = "generic"
		}
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE proxies SET name=$2,type=$3,host=$4,port=$5,username=NULLIF($6,''),adapter=$7,status='healthy',failure_count=0,consecutive_successes=0,last_checked_at=now(),last_success_at=now(),updated_at=now() WHERE id=$1`, id, x.Name, string(typ), x.Host, x.Port, x.Username, x.Adapter)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	_, _ = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET sticky_session=NULL,fallback_active=false,rotation_started_at=NULL,exit_ip=NULL,subnet_key=NULL,asn=NULL,country=NULL,last_health_ok=false,last_health_at=NULL,updated_at=now() WHERE account_id IN (SELECT account_id FROM network_profiles WHERE proxy_id=$1)`, id)
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.Password != "" {
		if err = s.Container.Secrets.PutProxy(r.Context(), id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			writeJSON(w, 500, map[string]string{"error": "proxy_saved_secret_update_failed"})
			return
		}
		_, _ = s.DB.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id)
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
	if err := s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM network_profiles WHERE proxy_id=$1`, id).Scan(&used); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if used > 0 {
		writeJSON(w, 409, map[string]any{"error": "proxy_in_use", "accounts": used, "detail": "Unassign this proxy from all accounts before deleting it."})
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
