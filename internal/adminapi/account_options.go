package adminapi

import (
	"encoding/json"
	"net/http"
	"time"
)

// accountOptions is intentionally cache-first: opening Edit must never wait on
// DigitalOcean/proxy network I/O. Live refresh remains the explicit Validate flow.
func (s *Server) accountOptions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var regions, sizes, images []byte
	var created time.Time
	err := s.DB.QueryRowContext(r.Context(), `SELECT data->'Regions',data->'Sizes',data->'Images',created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&regions, &sizes, &images, &created)
	if err == nil {
		w.Header().Set("X-Provider-Options-Source", "cached")
		writeJSON(w, 200, map[string]any{"regions": json.RawMessage(regions), "sizes": json.RawMessage(sizes), "images": json.RawMessage(images), "source": "cached", "cached_at": created.UTC().Format(time.RFC3339)})
		return
	}
	writeJSON(w, 409, map[string]string{"error": "account_options_not_cached", "detail": "Validate this account once to load DigitalOcean options."})
}
