package adminapi

import (
	"database/sql"
	"net/http"
)

func (s *Server) accountRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var providerCircuit string
	var providerFailures int
	var providerRetry, reset, remaining any
	err := s.DB.QueryRowContext(r.Context(), `SELECT circuit_state,consecutive_failures,retry_after,rate_remaining,rate_reset_at FROM account_runtime_state WHERE account_id=$1`, id).Scan(&providerCircuit, &providerFailures, &providerRetry, &remaining, &reset)
	if err != nil && err != sql.ErrNoRows {
		writeJSON(w, 500, errorBody())
		return
	}
	if providerCircuit == "" {
		providerCircuit = "closed"
	}

	var proxyCircuit, health string
	var proxyFailures int
	var proxyRetry, lease any
	var generation int64
	_ = s.DB.QueryRowContext(r.Context(), `SELECT prs.circuit_state,prs.health_state,prs.consecutive_failures,prs.retry_after,prs.generation,prs.half_open_probe_lease_until FROM proxy_runtime_state prs JOIN network_profiles np ON np.account_id=prs.account_id AND np.proxy_id=prs.proxy_id JOIN accounts a ON a.id=prs.account_id AND a.provider=prs.provider WHERE prs.account_id=$1`, id).Scan(&proxyCircuit, &health, &proxyFailures, &proxyRetry, &generation, &lease)

	var epoch int64
	var activeProxy, reason string
	_ = s.DB.QueryRowContext(r.Context(), `SELECT transport_epoch,COALESCE(active_proxy_id::text,''),COALESCE(transition_reason,'') FROM account_transport_state WHERE account_id=$1`, id).Scan(&epoch, &activeProxy, &reason)

	writeJSON(w, 200, map[string]any{
		"provider_circuit_state": providerCircuit, "provider_consecutive_failures": providerFailures, "provider_retry_after": providerRetry,
		"rate_remaining": remaining, "rate_reset_at": reset,
		"proxy_circuit_state": proxyCircuit, "proxy_health_state": health, "proxy_consecutive_failures": proxyFailures, "proxy_retry_after": proxyRetry,
		"proxy_generation": generation, "half_open_probe_lease_until": lease,
		"transport_epoch": epoch, "active_proxy_id": activeProxy, "transport_transition_reason": reason,
	})
}
