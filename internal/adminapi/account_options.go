package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"time"
)

func (s *Server) accountOptions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var raw []byte
	var created time.Time
	var catalog providers.Catalog
	found := false
	source := "canonical"
	if err := s.DB.QueryRowContext(r.Context(), "SELECT canonical,created_at FROM provider_snapshots WHERE account_id=$1 AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1", id).Scan(&raw, &created); err == nil {
		var obs providers.Observation
		if json.Unmarshal(raw, &obs) == nil {
			catalog = obs.Catalog
			found = true
		}
	}
	// A fresh health observation can contain an old catalog. Prefer independently
	// fetched metadata by its own timestamp, while fencing credential changes.
	var cached []byte
	var fetched time.Time
	err := s.DB.QueryRowContext(r.Context(), `SELECT c.catalog,c.refreshed_at FROM provider_catalog_cache c JOIN accounts a ON a.id=c.account_id
 WHERE c.account_id=$1 AND COALESCE(c.catalog->>'CacheVersion','')<>''
 AND c.refreshed_at<=now() AND c.refreshed_at>now()-interval '6 hours'
 AND c.scope_revision=a.provider||'/'||a.secret_ref||'/'||COALESCE((SELECT max(updated_at)::text FROM secrets WHERE account_id=a.id AND id=a.secret_ref),'missing')||'/catalog:'||(c.catalog->>'CacheVersion')`, id).Scan(&cached, &fetched)
	if err == nil {
		var candidate providers.Catalog
		if json.Unmarshal(cached, &candidate) == nil && (!found || !candidate.FetchedAt.Before(catalog.FetchedAt)) {
			catalog = candidate
			created = fetched
			source = "catalog_cache"
			found = true
		}
	}
	if found {
		images := make([]providers.Image, 0, 3)
		for _, x := range catalog.Images {
			if x.Family == "ubuntu" && (x.Version == "22.04" || x.Version == "24.04" || x.Version == "26.04") {
				images = append(images, x)
			}
		}
		writeJSON(w, 200, map[string]any{"regions": catalog.Regions, "plans": catalog.Plans, "sizes": catalog.Plans, "images": images, "pricing": catalog.Pricing, "source": source, "cached_at": created.UTC().Format(time.RFC3339), "catalog_fetched_at": catalog.FetchedAt})
		return
	}
	var preferredRegions, preferredSizes []byte
	var preferredImage string
	if err := s.DB.QueryRowContext(r.Context(), `SELECT preferred_regions,preferred_sizes,COALESCE(preferred_image,'') FROM accounts WHERE id=$1`, id).Scan(&preferredRegions, &preferredSizes, &preferredImage); err != nil {
		writeJSON(w, 404, map[string]string{"error": "not_found"})
		return
	}
	var rs, ss []string
	_ = json.Unmarshal(preferredRegions, &rs)
	_ = json.Unmarshal(preferredSizes, &ss)
	regions := make([]providers.Region, 0, len(rs))
	for _, v := range rs {
		regions = append(regions, providers.Region{ID: v, Name: v, Available: true})
	}
	plans := make([]providers.Plan, 0, len(ss))
	for _, v := range ss {
		plans = append(plans, providers.Plan{ID: v, Name: v, Available: true})
	}
	_ = preferredImage
	images := []providers.Image{{ID: "ubuntu-22-04-x64", Name: "Ubuntu 22.04", Family: "ubuntu", Version: "22.04", Architecture: "x86_64", Available: true}, {ID: "ubuntu-24-04-x64", Name: "Ubuntu 24.04", Family: "ubuntu", Version: "24.04", Architecture: "x86_64", Available: true}, {ID: "ubuntu-26-04-x64", Name: "Ubuntu 26.04", Family: "ubuntu", Version: "26.04", Architecture: "x86_64", Available: true}}
	writeJSON(w, 200, map[string]any{"regions": regions, "plans": plans, "sizes": plans, "images": images, "source": "account_preferences", "warning": "provider_options_not_cached"})
}
