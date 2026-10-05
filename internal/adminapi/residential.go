package adminapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residential"
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
	rows, e := s.DB.QueryContext(r.Context(), `SELECT rp.proxy_id::text,rp.name,rp.type,rp.host,rp.port,COALESCE(rp.username,''),rp.status,COALESCE(host(rp.exit_ip),''),COALESCE(rp.country,''),COALESCE(rp.latency_ms,0),rp.outbound_tag,rp.priority,rp.enabled,rp.secret_ref IS NOT NULL,rp.last_checked_at,rp.last_success_at,rp.last_error,rp.success_ewma,rp.latency_ewma_ms,rp.check_count FROM residential_proxies rp ORDER BY rp.priority,rp.name`)
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
		var checked, succeeded sql.NullTime
		var lastError string
		var successEWMA, latencyEWMA float64
		var count int64
		if rows.Scan(&id, &name, &typ, &host, &port, &user, &status, &ip, &country, &latency, &tag, &priority, &enabled, &pass, &checked, &succeeded, &lastError, &successEWMA, &latencyEWMA, &count) == nil {
			out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "username": user, "status": status, "exit_ip": ip, "country": country, "latency_ms": latency, "outbound_tag": tag, "priority": priority, "enabled": enabled, "password_configured": pass, "last_checked_at": nullableResidentialTime(checked), "last_success_at": nullableResidentialTime(succeeded), "last_error": lastError, "success_ewma": successEWMA, "latency_ewma_ms": latencyEWMA, "check_count": count})
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
	if json.NewDecoder(r.Body).Decode(&x) != nil || !validResidentialName(x.Name) || x.Host == "" || x.Port < 1 || x.Port > 65535 || x.Priority < 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	pw := proxyWrite{Name: x.Name, Type: x.Type, Host: x.Host, Port: x.Port, Username: x.Username, Password: x.Password, Adapter: "generic"}
	typ, e := resolveResidentialType(r, pw)
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
	e = tx.QueryRowContext(r.Context(), "SELECT gen_random_uuid()::text").Scan(&id)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	tag := "residential-ads-" + id
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO residential_proxies(proxy_id,name,type,host,port,username,outbound_tag,priority,enabled)
 VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9)`, id, x.Name, string(typ), x.Host, x.Port, x.Username, tag, x.Priority, x.Enabled); e != nil {
		writeJSON(w, 409, map[string]string{"error": "duplicate_name", "detail": "Name already exists (case-insensitive), or the endpoint conflicts with a saved record."})
		return
	}
	if x.Password != "" {
		if e = s.Container.Secrets.PutResidentialTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); e != nil {

			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE residential_proxies SET secret_ref='proxy-password' WHERE proxy_id=$1`, id); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}

	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "type": string(typ), "outbound_tag": tag})
}

func (s *Server) testResidentialProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanWrite() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	result, err := (residential.Monitor{DB: s.DB, Secrets: s.Container.Secrets}).Check(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "residential_changed", "detail": "Endpoint changed during the check; refresh and retry."})
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) updateResidentialProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	var x residentialWrite
	if json.NewDecoder(r.Body).Decode(&x) != nil || !validResidentialName(x.Name) || x.Host == "" || x.Port < 1 || x.Port > 65535 || x.Priority < 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return
	}
	id := r.PathValue("id")
	pw := proxyWrite{Name: x.Name, Type: x.Type, Host: x.Host, Port: x.Port, Username: x.Username, Password: x.Password, Adapter: "generic"}
	if pw.Password == "" {
		if secret, err := s.Container.Secrets.GetResidential(r.Context(), id, "proxy-password"); err == nil {
			pw.Password = string(secret)
			defer zeroBytes(secret)
		}
	}

	typ, e := resolveResidentialType(r, pw)
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
	res, e := tx.ExecContext(r.Context(), `UPDATE residential_proxies SET name=$2,type=$3,host=$4,port=$5,username=NULLIF($6,''),status='unknown',last_success_at=NULL,last_checked_at=NULL,last_error='',success_ewma=0,latency_ewma_ms=0,check_count=0,updated_at=now() WHERE proxy_id=$1`, id, x.Name, string(typ), x.Host, x.Port, x.Username)
	if e != nil {
		var pe *pq.Error
		if errors.As(e, &pe) && pe.Code == "23505" {
			writeJSON(w, 409, map[string]string{"error": "duplicate_name", "detail": "Name already exists (case-insensitive)."})
		} else {
			writeJSON(w, 500, errorBody())
		}
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
		if e = s.Container.Secrets.PutResidentialTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		if _, e = tx.ExecContext(r.Context(), `UPDATE residential_proxies SET secret_ref='proxy-password' WHERE proxy_id=$1`, id); e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
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

func nullableResidentialTime(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func resolveResidentialType(r *http.Request, x proxyWrite) (network.ProxyType, error) {
	switch x.Type {
	case "http", "https", "socks5":
		return network.ProxyType(x.Type), nil
	case "", "auto":
		return resolveProxyType(r, x)
	default:
		return "", errors.New("invalid proxy type")
	}
}
