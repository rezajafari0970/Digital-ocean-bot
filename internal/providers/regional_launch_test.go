package providers

import "testing"

func TestRegionalLaunchPinsNativeIDsAndFiltersFallback(t *testing.T) {
	c := Catalog{Plans: []Plan{{ID: "b3-8", IDsByRegion: map[string]string{"GRA11": "flavor-1", "BHS5": "flavor-2"}}}, Images: []Image{{ID: "ubuntu", IDsByRegion: map[string]string{"GRA11": "image-1"}}}}
	p, r, e := FreezeRegionalLaunches(c, "b3-8", "ubuntu", []string{"BHS5", "GRA11", "GRA11"})
	if e != nil || len(r) != 1 || r[0] != "GRA11" || p["GRA11"].ImageID != "image-1" {
		t.Fatal(p, r, e)
	}
	c.Images[0].IDsByRegion["GRA11"] = "image-new"
	if p["GRA11"].ImageID != "image-1" {
		t.Fatal("snapshot changed with catalog")
	}
	if _, _, e = FreezeRegionalLaunches(c, "b3-8", "ubuntu", []string{"BHS5"}); e == nil {
		t.Fatal("incompatible launch accepted")
	}
	p, _, e = FreezeRegionalLaunches(Catalog{}, "global-plan", "global-image", []string{"r"})
	if e != nil || p != nil {
		t.Fatal("legacy behavior changed")
	}
}
