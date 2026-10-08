package adminapi

import (
	"net/http"
	"time"
)

func (s *Server) proxyTraffic(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT role,owner_id,proxy_id,purpose,sum(tx_bytes),sum(rx_bytes),sum(connections),sum(requests),sum(errors),max(updated_at) FROM proxy_traffic_hourly WHERE hour>=date_trunc('hour',now()-interval '24 hours') OR role='overflow' GROUP BY role,owner_id,proxy_id,purpose ORDER BY sum(tx_bytes)+sum(rx_bytes) DESC LIMIT 4096`)
	if err != nil {
		writeJSON(w, 503, errorBody())
		return
	}
	defer rows.Close()
	type count struct {
		Role        string    `json:"role"`
		Owner       string    `json:"owner_id"`
		Proxy       string    `json:"proxy_id"`
		Purpose     string    `json:"purpose"`
		TX          int64     `json:"tx_bytes"`
		RX          int64     `json:"rx_bytes"`
		Connections int64     `json:"connections"`
		Requests    int64     `json:"requests"`
		Errors      int64     `json:"errors"`
		Updated     time.Time `json:"updated_at"`
	}
	result := []count{}
	for rows.Next() {
		var c count
		if err = rows.Scan(&c.Role, &c.Owner, &c.Proxy, &c.Purpose, &c.TX, &c.RX, &c.Connections, &c.Requests, &c.Errors, &c.Updated); err != nil {
			writeJSON(w, 503, errorBody())
			return
		}
		result = append(result, c)
	}
	if rows.Err() != nil {
		writeJSON(w, 503, errorBody())
		return
	}
	writeJSON(w, 200, map[string]any{"metric": "observed_proxy_socket_bytes_not_provider_billing", "window": "24_hours_rounded_to_hour; overflow_is_process_lifetime", "flush_interval_seconds": 30, "rows": result})
}
