package adminapi

import "net/http"

func (s *Server) providersMetadata(w http.ResponseWriter, r *http.Request) {
	if s.Container.Providers == nil {
		writeJSON(w, 503, map[string]string{"error": "provider_registry_unavailable"})
		return
	}
	writeJSON(w, 200, s.Container.Providers.MetadataAll())
}
