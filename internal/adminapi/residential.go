package adminapi

import (
	"encoding/json"
	"net/http"
)

type residentialWrite struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) residentialProxies(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.QueryContext(r.Context(), `SELECT p.id::text,p.name,p.type,p.host,p.port,COALESCE(p.username,''),p.status,COALESCE(host(p.exit_ip),''),COALESCE(p.country,''),COALESCE(p.latency_ms,0),rp.outbound_tag,rp.priority,rp.enabled,p.secret_ref IS NOT NULL FROM residential_proxies rp JOIN proxies p ON p.id=rp.proxy_id ORDER BY rp.priority,p.name`)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, typ, host, user, status, ip, country, tag string
		var port, latency, priority int
		var enabled, pass bool
		if rows.Scan(&id, &name, &typ, &host, &port, &user, &status, &ip, &country, &latency, &tag, &priority, &enabled, &pass) == nil {
			out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "username": user, "status": status, "exit_ip": ip, "country": country, "latency_ms": latency, "outbound_tag": tag, "priority": priority, "enabled": enabled, "password_configured": pass})
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) createResidentialProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x residentialWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Host == "" || x.Port < 1 || x.Port > 65535 || x.Priority < 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	pw := proxyWrite{Name: x.Name, Type: x.Type, Host: x.Host, Port: x.Port, Username: x.Username, Password: x.Password, Adapter: "generic"}
	typ, e := resolveProxyType(r, pw)
	if e != nil {
		writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	var id string
	e = tx.QueryRowContext(r.Context(), `INSERT INTO proxies(id,name,type,host,port,username,secret_ref,status,adapter) VALUES(gen_random_uuid(),$1,$2,$3,$4,NULLIF($5,''),NULL,'healthy','generic') RETURNING id::text`, x.Name, string(typ), x.Host, x.Port, x.Username).Scan(&id)
	if e != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	tag := "residential-ads-" + id
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO residential_proxies(proxy_id,outbound_tag,priority,enabled) VALUES($1,$2,$3,$4)`, id, tag, x.Priority, x.Enabled); e != nil {
		writeJSON(w, 409, errorBody())
		return
	}
	if x.Password != "" {
		if e = s.Container.Secrets.PutProxyTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); e != nil {

			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE proxies SET last_success_at=now(),last_checked_at=now() WHERE id=$1", id); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "type": string(typ), "outbound_tag": tag})
}

func (s *Server) testResidentialProxy(w http.ResponseWriter, r *http.Request) { s.testProxy(w, r) }

func (s *Server) updateResidentialProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x residentialWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || x.Name == "" || x.Host == "" || x.Port < 1 || x.Port > 65535 || x.Priority < 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	id := r.PathValue("id")
	pw := proxyWrite{Name: x.Name, Type: x.Type, Host: x.Host, Port: x.Port, Username: x.Username, Password: x.Password, Adapter: "generic"}
	if pw.Password == "" {
		if secret, err := s.Container.Secrets.GetProxy(r.Context(), id, "proxy-password"); err == nil {
			pw.Password = string(secret)
			defer zeroBytes(secret)
		}
	}

	typ, e := resolveProxyType(r, pw)
	if e != nil {
		writeJSON(w, 422, map[string]string{"error": "proxy_detection_failed"})
		return
	}
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(r.Context(), `UPDATE proxies p SET name=$2,type=$3,host=$4,port=$5,username=NULLIF($6,''),status='healthy',last_success_at=now(),last_checked_at=now(),updated_at=now() FROM residential_proxies rp WHERE p.id=$1 AND rp.proxy_id=p.id`, id, x.Name, string(typ), x.Host, x.Port, x.Username)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE residential_proxies SET priority=$2,enabled=$3,updated_at=now() WHERE proxy_id=$1`, id, x.Priority, x.Enabled)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if x.Password != "" {
		if e = s.Container.Secrets.PutProxyTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE proxies SET secret_ref='proxy-password' WHERE id=$1`, id); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	if _, e = tx.ExecContext(r.Context(), `UPDATE account_transport_state SET transport_epoch=transport_epoch+1,transition_reason='proxy-configuration-change',updated_at=now() WHERE account_id IN(SELECT account_id FROM network_profiles WHERE proxy_id=$1 UNION SELECT account_id FROM account_proxy_pool WHERE proxy_id=$1)`, id); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}

	writeJSON(w, 200, map[string]any{"id": id, "type": string(typ)})
}
func (s *Server) deleteResidentialProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	if _, err := s.removeProxy(r.Context(), id, true); err != nil {
		writeJSON(w, 409, map[string]string{"error": "residential_delete_pending", "detail": "The proxy changed concurrently; refresh and retry."})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
