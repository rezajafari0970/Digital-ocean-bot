package adminapi

import "net/http"

func (s *Server) accountRuntime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var circuit string
	var failures int
	var retry, reset any
	var remaining any
	err := s.DB.QueryRowContext(r.Context(), `SELECT circuit_state,consecutive_failures,retry_after,rate_remaining,rate_reset_at FROM account_runtime_state WHERE account_id=$1`, id).Scan(&circuit, &failures, &retry, &remaining, &reset)
	if err != nil {
		writeJSON(w, 200, map[string]any{"circuit_state": "closed", "consecutive_failures": 0})
		return
	}
	writeJSON(w, 200, map[string]any{"circuit_state": circuit, "consecutive_failures": failures, "retry_after": retry, "rate_remaining": remaining, "rate_reset_at": reset})
}
