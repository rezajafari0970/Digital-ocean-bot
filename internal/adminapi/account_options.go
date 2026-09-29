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
		var allImages []map[string]any
		_ = json.Unmarshal(images, &allImages)
		allowed := map[string]bool{"22.04 (LTS) x64": true, "24.04 (LTS) x64": true, "26.04 (LTS) x64": true}
		filtered := make([]map[string]any, 0, 3)
		for _, image := range allImages {
			dist, _ := image["Distribution"].(string)
			name, _ := image["Name"].(string)
			if dist == "Ubuntu" && allowed[name] {
				filtered = append(filtered, image)
			}
		}
		writeJSON(w, 200, map[string]any{"regions": json.RawMessage(regions), "sizes": json.RawMessage(sizes), "images": filtered, "source": "cached", "cached_at": created.UTC().Format(time.RFC3339)})
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
	_ = preferredImage
	iout := []map[string]any{
		{"ID": "ubuntu-22-04-x64", "Slug": "ubuntu-22-04-x64", "Distribution": "Ubuntu", "Name": "22.04 (LTS) x64", "Public": true, "Status": "available"},
		{"ID": "ubuntu-24-04-x64", "Slug": "ubuntu-24-04-x64", "Distribution": "Ubuntu", "Name": "24.04 (LTS) x64", "Public": true, "Status": "available"},
		{"ID": "ubuntu-26-04-x64", "Slug": "ubuntu-26-04-x64", "Distribution": "Ubuntu", "Name": "26.04 (LTS) x64", "Public": true, "Status": "available"},
	}
	writeJSON(w, 200, map[string]any{"regions": rout, "sizes": sout, "images": iout, "source": "account_preferences", "warning": "provider_options_not_cached"})
}
