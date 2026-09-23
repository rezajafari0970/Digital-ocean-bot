package adminapi

import (
	"database/sql"
	"net/http"
)

type accountDashboard struct {
	Account     map[string]any `json:"account"`
	Capacity    map[string]any `json:"capacity"`
	Resources   map[string]any `json:"resources"`
	Network     map[string]any `json:"network"`
	Runtime     map[string]any `json:"runtime"`
	Deployments map[string]any `json:"deployments"`
}

func (s *Server) accountDashboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name, provider, email, externalID string
	var enabled bool
	if err := s.DB.QueryRowContext(r.Context(), `SELECT name,provider,enabled,COALESCE(email,''),COALESCE(external_id,'') FROM accounts WHERE id=$1`, id).Scan(&name, &provider, &enabled, &email, &externalID); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "not_found"})
		} else {
			writeJSON(w, 500, errorBody())
		}
		return
	}
	d := accountDashboard{Account: map[string]any{"id": id, "name": name, "provider": provider, "enabled": enabled, "email": email, "external_id": externalID}, Capacity: map[string]any{}, Resources: map[string]any{}, Network: map[string]any{}, Runtime: map[string]any{}, Deployments: map[string]any{}}
	var total, managed, active int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE managed),count(*) FILTER(WHERE state='active') FROM resources WHERE account_id=$1`, id).Scan(&total, &managed, &active)
	d.Resources = map[string]any{"total": total, "managed": managed, "unmanaged": total - managed, "active": active}

	var limit int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE((data->'Limits'->>'DropletLimit')::int,0) FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&limit)
	available := limit - active
	if available < 0 {
		available = 0
	}
	var providerDroplets int
	var lastRefresh sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COALESCE(jsonb_array_length(data->'Droplets'),0),created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&providerDroplets, &lastRefresh)
	var lastCreated sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT max(created_at) FROM droplets WHERE account_id=$1`, id).Scan(&lastCreated)
	providerAvailable := limit - providerDroplets
	if providerAvailable < 0 {
		providerAvailable = 0
	}
	d.Capacity = map[string]any{"droplet_limit": limit, "provider_droplets": providerDroplets, "managed_droplets": active, "available": providerAvailable, "last_refresh": lastRefresh.Time, "last_managed_server_created": lastCreated.Time}
	var mode, status string
	var proxyID sql.NullString
	_ = s.DB.QueryRowContext(r.Context(), `SELECT n.mode,n.proxy_id::text,COALESCE(p.status,'') FROM network_profiles n LEFT JOIN proxies p ON p.id=n.proxy_id WHERE n.account_id=$1`, id).Scan(&mode, &proxyID, &status)
	d.Network = map[string]any{"mode": mode, "proxy_id": proxyID.String, "proxy_status": status}
	var circuit string
	var failures int
	var retry sql.NullTime
	_ = s.DB.QueryRowContext(r.Context(), `SELECT circuit_state,consecutive_failures,retry_after FROM account_runtime_state WHERE account_id=$1`, id).Scan(&circuit, &failures, &retry)
	d.Runtime = map[string]any{"circuit": circuit, "failures": failures, "retry_after": retry.Time}
	var running, ready, failed int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE state NOT IN ('READY','FAILED')),count(*) FILTER(WHERE state='READY'),count(*) FILTER(WHERE state='FAILED') FROM deployments WHERE account_id=$1`, id).Scan(&running, &ready, &failed)
	d.Deployments = map[string]any{"running": running, "ready": ready, "failed": failed}
	writeJSON(w, 200, d)
}
