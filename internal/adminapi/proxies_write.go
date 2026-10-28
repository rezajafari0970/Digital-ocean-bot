package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
	"strings"
)

type proxyWrite struct {
	Revision int64  `json:"settings_revision"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Type     string `json:"type"`
	Adapter  string `json:"adapter"`
}

func validProxyWrite(x proxyWrite) bool {
	x.Name = strings.TrimSpace(x.Name)
	x.Host = strings.TrimSpace(x.Host)
	return x.Name != "" && x.Host != "" && x.Port > 0 && x.Port <= 65535 && app.ProxyAdapterRegistered(x.Adapter)
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
	if x.Adapter == "" {
		x.Adapter = "generic"
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO proxies(id,name,type,host,port,username,secret_ref,status,adapter) VALUES(gen_random_uuid(),$1,$2,$3,$4,NULLIF($5,''),NULL,'healthy',$6) RETURNING id::text`, x.Name, string(typ), x.Host, x.Port, x.Username, x.Adapter).Scan(&id)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	if x.Password != "" {
		if err = s.Container.Secrets.PutProxyTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE proxies SET last_checked_at=now(),last_success_at=now(),failure_count=0,consecutive_successes=0 WHERE id=$1`, id); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 503, map[string]string{"error": "proxy_create_unconfirmed", "detail": "Refresh the proxy list before retrying."})
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
		collision, _ := networkIdentityCollision(r.Context(), s.DB, accountID, x.ProxyID)
		if collision {
			writeJSON(w, 409, map[string]string{"error": "network_identity_collision", "detail": "Selected proxy shares an exit IP or subnet with another account"})
			return
		}
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	for _, prefix := range []string{"account-proxy:", "account-route:"} {
		if _, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+accountID); err != nil {
			writeJSON(w, 409, errorBody())
			return
		}
	}
	var accountRevision int64
	if err = tx.QueryRowContext(r.Context(), `SELECT settings_revision FROM accounts WHERE id=$1 FOR UPDATE NOWAIT`, accountID).Scan(&accountRevision); err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_busy"})
		return
	}
	var currentMode, currentProxy string
	if err = tx.QueryRowContext(r.Context(), `SELECT mode,COALESCE(proxy_id::text,'') FROM network_profiles WHERE account_id=$1`, accountID).Scan(&currentMode, &currentProxy); err != nil {
		writeJSON(w, 404, errorBody())
		return
	}
	if x.Mode == "direct" {
		x.ProxyID = ""
	}
	var desiredMatches bool
	if err = tx.QueryRowContext(r.Context(), `SELECT CASE WHEN $2='direct' THEN NOT EXISTS(SELECT 1 FROM account_proxy_pool WHERE account_id=$1 AND enabled) ELSE
 (SELECT count(*)=1 AND bool_and(proxy_id=NULLIF($3,'')::uuid) FROM account_proxy_pool WHERE account_id=$1 AND enabled) END`, accountID, x.Mode, x.ProxyID).Scan(&desiredMatches); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if currentMode == x.Mode && currentProxy == x.ProxyID && desiredMatches {
		w.WriteHeader(204)
		return
	}
	if x.Mode == "proxy_required" {
		if err = syncAccountProxyPool(r.Context(), tx, accountID, []string{x.ProxyID}); err != nil {
			writeJSON(w, 409, errorBody())
			return
		}
	} else {
		if _, err = tx.ExecContext(r.Context(), `UPDATE account_proxy_pool SET enabled=false WHERE account_id=$1`, accountID); err != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE accounts SET settings_revision=settings_revision+1,updated_at=now() WHERE id=$1`, accountID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE network_profiles SET mode=$2,proxy_id=CASE WHEN $2='direct' THEN NULL ELSE $3::uuid END,updated_at=now() WHERE account_id=$1`, accountID, x.Mode, x.ProxyID)
	if err != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "account_not_found"})
		return
	}
	if _, err = app.FenceCountryWriterTx(r.Context(), tx, accountID); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE account_network_identities SET selected_target_country=NULL,sticky_session=NULL,fallback_active=false,rotation_started_at=NULL,exit_ip=NULL,subnet_key=NULL,asn=NULL,country=NULL,country_code=NULL,preferred_country=NULL,preferred_country_code=NULL,last_health_ok=false,last_health_at=NULL,updated_at=now() WHERE account_id=$1`, accountID)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.WriteHeader(204)
}
