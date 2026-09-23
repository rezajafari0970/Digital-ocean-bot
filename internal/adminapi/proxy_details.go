package adminapi

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
)

func (s *Server) proxyDetails(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var name, typ, host, username string
	var port int
	if err := s.DB.QueryRowContext(r.Context(), `SELECT name,type,host,port,COALESCE(username,'') FROM proxies WHERE id=$1`, id).Scan(&name, &typ, &host, &port, &username); err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	password := ""
	if b, err := s.Container.Secrets.GetProxy(r.Context(), id, "proxy-password"); err == nil {
		password = string(b)
		for i := range b {
			b[i] = 0
		}
	}
	writeJSON(w, 200, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "username": username, "password": password})
}
func (s *Server) testProxy(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r.Context())
	if !p.CanWrite() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	id := r.PathValue("id")
	var x proxyWrite
	var typ string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT name,type,host,port,COALESCE(username,'') FROM proxies WHERE id=$1`, id).Scan(&x.Name, &typ, &x.Host, &x.Port, &x.Username); err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	if b, err := s.Container.Secrets.GetProxy(r.Context(), id, "proxy-password"); err == nil {
		x.Password = string(b)
		defer func() {
			for i := range b {
				b[i] = 0
			}
		}()
	}
	detected, err := detectProxyTypes(r.Context(), x, []network.ProxyType{network.ProxyType(typ)})
	if err != nil {
		writeJSON(w, 502, map[string]string{"status": "down", "error": "proxy_test_failed"})
		return
	}
	obs, err := observeProxy(r.Context(), x, detected)
	if err != nil {
		writeJSON(w, 502, map[string]string{"status": "down", "error": "proxy_observation_failed"})
		return
	}
	_, _ = s.DB.ExecContext(r.Context(), `UPDATE proxies SET status='healthy',exit_ip=$2,country=$3,asn=$4,latency_ms=$5,last_checked_at=now(),last_success_at=now(),failure_count=0 WHERE id=$1`, id, obs.IP, obs.Country, obs.ASN, obs.LatencyMS)
	writeJSON(w, 200, map[string]any{"status": "healthy", "type": detected, "exit_ip": obs.IP, "country": obs.Country, "country_code": obs.CountryCode, "asn": obs.ASN, "latency_ms": obs.LatencyMS})
}
