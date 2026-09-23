package adminapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accountOptions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runtime, err := s.Container.Runtime(r.Context(), id)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": "account_not_ready", "detail": err.Error()})
		return
	}
	defer func() {
		if runtime.Gateway != nil {
			runtime.Gateway.CloseIdleConnections()
		}
	}()
	result, err := runtime.Provider.Discover(r.Context())
	if err == nil {
		writeJSON(w, 200, map[string]any{"regions": result.Regions, "sizes": result.Sizes, "images": result.Images, "source": "live"})
		return
	}
	var regions, sizes, images []byte
	var created time.Time
	cacheErr := s.DB.QueryRowContext(r.Context(), `SELECT data->'Regions',data->'Sizes',data->'Images',created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&regions, &sizes, &images, &created)
	if cacheErr == nil {
		w.Header().Set("X-Provider-Options-Source", "cached")
		writeJSON(w, 200, map[string]any{"regions": json.RawMessage(regions), "sizes": json.RawMessage(sizes), "images": json.RawMessage(images), "source": "cached", "warning": "live_provider_discovery_failed", "cached_at": created.UTC().Format(time.RFC3339)})
		return
	}
	writeJSON(w, 502, map[string]string{"error": "provider_discovery_failed", "detail": err.Error()})
}
