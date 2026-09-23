package adminapi

import "net/http"

func (s *Server) accountDiscovery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_not_ready"})
		return
	}
	result, err := runtime.Provider.Discover(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "provider_discovery_failed"})
		return
	}
	writeJSON(w, 200, result)
}
