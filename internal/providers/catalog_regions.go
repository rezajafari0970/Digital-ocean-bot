package providers

// CatalogRegionsWithOptions returns only available regions with both an
// available plan and image. A non-nil native-ID map is authoritative: an empty
// map means no regional availability, not global availability.
func CatalogRegionsWithOptions(c Catalog) []Region {
	out := []Region{}
	for _, r := range c.Regions {
		if !r.Available {
			continue
		}
		plan := false
		for _, p := range c.Plans {
			if !p.Available || (p.IDsByRegion != nil && p.IDsByRegion[r.ID] == "") {
				continue
			}
			if len(p.AvailableRegions) > 0 {
				found := false
				for _, id := range p.AvailableRegions {
					if id == r.ID {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			plan = true
			break
		}
		if !plan {
			continue
		}
		for _, im := range c.Images {
			if im.Available && (im.IDsByRegion == nil || im.IDsByRegion[r.ID] != "") {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
