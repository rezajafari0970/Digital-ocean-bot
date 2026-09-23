package adminapi

import (
	"net/http"
	"time"
)

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id::text,a.name,COALESCE(a.email,''),COALESCE(a.preferred_region,''),a.enabled,a.created_at,n.mode,COALESCE(p.name,'') FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id ORDER BY a.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, email, region, mode, proxy string
		var enabled bool
		var created time.Time
		if rows.Scan(&id, &name, &email, &region, &enabled, &created, &mode, &proxy) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "email": email, "region": region, "network": mode, "proxy": proxy, "enabled": enabled, "added_at": created.UTC().Format("2006-01-02 15:04:05")})
	}
	writeJSON(w, 200, out)
}

func (s *Server) proxies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,name,type,host,port,status,COALESCE(host(exit_ip),''),COALESCE(country,''),COALESCE(asn,''),COALESCE(latency_ms,0),failure_count,last_checked_at FROM proxies ORDER BY name`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, typ, host, status, ip, country, asn string
		var port, fail int
		var latency int64
		var checked any
		if rows.Scan(&id, &name, &typ, &host, &port, &status, &ip, &country, &asn, &latency, &fail, &checked) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "status": status, "exit_ip": ip, "country": country, "asn": asn, "latency_ms": latency, "failure_count": fail, "last_checked_at": checked})
	}
	writeJSON(w, 200, out)
}

func errorBody() map[string]string { return map[string]string{"error": "internal_error"} }
