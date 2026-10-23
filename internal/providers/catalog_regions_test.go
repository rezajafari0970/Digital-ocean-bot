package providers

import "testing"

func TestCatalogRegionsRequireAvailableRegionalOptions(t *testing.T) {
	base := func() Catalog {
		return Catalog{
			Regions: []Region{{ID: "SBG", Available: true}, {ID: "SBG5", Available: true}, {ID: "down"}},
			Plans:   []Plan{{ID: "b3-8", Available: true, AvailableRegions: []string{"SBG5"}, IDsByRegion: map[string]string{"SBG5": "flavor"}}},
			Images:  []Image{{ID: "ubuntu", Available: true, IDsByRegion: map[string]string{"SBG5": "image"}}},
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Catalog)
		want   int
	}{
		{"regional", func(*Catalog) {}, 1},
		{"no plans", func(c *Catalog) { c.Plans = nil }, 0},
		{"unavailable plan", func(c *Catalog) { c.Plans[0].Available = false }, 0},
		{"unavailable image", func(c *Catalog) { c.Images[0].Available = false }, 0},
		{"empty native plan map", func(c *Catalog) { c.Plans[0].IDsByRegion = map[string]string{} }, 0},
		{"empty native image map", func(c *Catalog) { c.Images[0].IDsByRegion = map[string]string{} }, 0},
		{"disjoint regions", func(c *Catalog) { c.Images[0].IDsByRegion = map[string]string{"SBG": "image"} }, 0},
		{"global IDs", func(c *Catalog) {
			c.Plans[0].IDsByRegion = nil
			c.Plans[0].AvailableRegions = nil
			c.Images[0].IDsByRegion = nil
		}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			got := CatalogRegionsWithOptions(c)
			if len(got) != tc.want {
				t.Fatalf("got %v want %d", got, tc.want)
			}
			if tc.want == 1 && got[0].ID != "SBG5" {
				t.Fatal(got)
			}
		})
	}
}
