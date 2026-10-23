package ovh

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestOVHCatalogExcludesEmptyLegacyRegion(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("catalog must be read only")
		}
		body := strings.ReplaceAll(readFixture(r.URL.Path), "GRA11", "SBG5")
		switch {
		case strings.HasSuffix(r.URL.Path, "/region"):
			body = `["SBG","SBG5"]`
		case strings.HasSuffix(r.URL.Path, "/region/SBG"):
			body = `{"name":"SBG","status":"UP","type":"region","datacenterLocation":"SBG"}`
		case strings.HasSuffix(r.URL.Path, "/region/SBG5"):
			body = `{"name":"SBG5","status":"UP","type":"region","datacenterLocation":"SBG"}`
		}
		return response(body), nil
	})
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Regions) != 1 || c.Regions[0].ID != "SBG5" || len(c.Plans) != 1 || c.Plans[0].IDsByRegion["SBG5"] != flavorID || c.Images[0].IDsByRegion["SBG5"] != imageID {
		t.Fatal(c, e)
	}
}
