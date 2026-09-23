package adminapi

import "net/http"

func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,provider,name,COALESCE(external_id,''),COALESCE(email,''),enabled,created_at FROM accounts ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, provider, name, external, email string
		var enabled bool
		var created any
		if rows.Scan(&id, &provider, &name, &external, &email, &enabled, &created) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "provider": provider, "name": name, "external_id": external, "email": email, "enabled": enabled, "created_at": created})
	}
	writeJSON(w, 200, out)
}

func (s *Server) proxies(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id::text,name,type,host,port,status,COALESCE(exit_ip::text,''),COALESCE(country,''),failure_count,last_checked_at FROM proxies ORDER BY name`)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, name, typ, host, status, ip, country string
		var port, fail int
		var checked any
		if rows.Scan(&id, &name, &typ, &host, &port, &status, &ip, &country, &fail, &checked) != nil {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "type": typ, "host": host, "port": port, "status": status, "exit_ip": ip, "country": country, "failure_count": fail, "last_checked_at": checked})
	}
	writeJSON(w, 200, out)
}

func errorBody() map[string]string { return map[string]string{"error": "internal_error"} }
