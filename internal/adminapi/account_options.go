package adminapi

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) accountOptions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var regions, sizes, images []byte
	var created time.Time
	err := s.DB.QueryRowContext(r.Context(), `SELECT data->'Regions',data->'Sizes',data->'Images',created_at FROM provider_snapshots WHERE account_id=$1 ORDER BY created_at DESC LIMIT 1`, id).Scan(&regions, &sizes, &images, &created)
	if err == nil {
		writeJSON(w, 200, map[string]any{"regions": json.RawMessage(regions), "sizes": json.RawMessage(sizes), "images": json.RawMessage(images), "source": "cached", "cached_at": created.UTC().Format(time.RFC3339)})
		return
	}

	// Legacy accounts may predate provider snapshots. Return their currently
	// selected values immediately so Edit remains usable without network I/O.
	var preferredRegions, preferredSizes []byte
	var preferredImage string
	if err = s.DB.QueryRowContext(r.Context(), `SELECT preferred_regions,preferred_sizes,COALESCE(preferred_image,'') FROM accounts WHERE id=$1`, id).Scan(&preferredRegions, &preferredSizes, &preferredImage); err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	var rs, ss []string
	_ = json.Unmarshal(preferredRegions, &rs)
	_ = json.Unmarshal(preferredSizes, &ss)
	rout := make([]map[string]any, 0, len(rs))
	for _, v := range rs {
		rout = append(rout, map[string]any{"slug": v, "name": v, "available": true})
	}
	sout := make([]map[string]any, 0, len(ss))
	for _, v := range ss {
		sout = append(sout, map[string]any{"slug": v, "available": true})
	}
	iout := []map[string]any{}
	if preferredImage != "" {
		iout = append(iout, map[string]any{"ID": preferredImage, "slug": preferredImage, "Distribution": "Current", "Name": preferredImage, "Public": true, "Status": "available"})
	}
	writeJSON(w, 200, map[string]any{"regions": rout, "sizes": sout, "images": iout, "source": "account_preferences", "warning": "provider_options_not_cached"})
}
