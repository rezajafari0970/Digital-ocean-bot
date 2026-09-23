package adminapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT a.id::text,a.name,COALESCE(a.email,''),COALESCE(a.preferred_region,''),a.enabled,a.created_at,n.mode,COALESCE(p.name,''),COALESCE(a.preferred_regions,'[]'::jsonb),COALESCE(a.preferred_sizes,'[]'::jsonb),COALESCE(a.preferred_image,''),a.auto_interval_seconds,a.auto_batch_size,a.auto_max_concurrent,a.server_lifetime_seconds,COALESCE(n.proxy_id::text,''),a.runtime_status,COALESCE(a.runtime_status_detail,''),a.desired_server_count FROM accounts a LEFT JOIN network_profiles n ON n.account_id=a.id LEFT JOIN proxies p ON p.id=n.proxy_id ORDER BY a.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, email, region, mode, proxy, image, proxyID, runtimeStatus, runtimeDetail string
		var regions, sizes []byte
		var interval, batch, concurrent, lifetime, desired int
		var enabled bool
		var created time.Time
		if rows.Scan(&id, &name, &email, &region, &enabled, &created, &mode, &proxy, &regions, &sizes, &image, &interval, &batch, &concurrent, &lifetime, &proxyID, &runtimeStatus, &runtimeDetail, &desired) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "email": email, "region": region, "network": mode, "proxy": proxy, "enabled": enabled, "added_at": created.UTC().Format("2006-01-02 15:04:05"), "regions": json.RawMessage(regions), "sizes": json.RawMessage(sizes), "image": image, "interval_seconds": interval, "batch_size": batch, "max_concurrent": concurrent, "lifetime_seconds": lifetime, "proxy_id": proxyID, "runtime_status": runtimeStatus, "runtime_status_detail": runtimeDetail, "desired_server_count": desired})
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
