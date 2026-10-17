package adminapi

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOutputCountryNamesAllRoutesAndObservedRegion(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	regions := []string{"fra1", "nrt", "uk-lon1", "us-lax"}
	labels := []string{"🇩🇪 Germany", "🇯🇵 Japan", "🇬🇧 United Kingdom", "🇺🇸 United States"}
	names := []string{"Frankfurt 1", "Tokyo, JP", "London #1", "Los Angeles, CA"}
	ps := []string{"digitalocean", "vultr", "upcloud", "linode"}
	expected := map[string]string{}
	for i, x := range f {
		// Requested build region is deliberately wrong; actual resource wins.
		sqlMust(t, db, `UPDATE droplets SET profile='{"Region":"ca-central"}' WHERE id=$1`, x.Droplet)
		var rid string
		if err := db.QueryRow("SELECT provider_resource_id FROM droplets WHERE id=$1", x.Droplet).Scan(&rid); err != nil {
			t.Fatal(err)
		}
		obs := providers.Observation{Catalog: providers.Catalog{Regions: []providers.Region{{ID: regions[i], Name: names[i]}}}, Inventory: providers.Inventory{Servers: []providers.Server{{ID: rid, RegionID: regions[i]}, {ID: "foreign", RegionID: "wrong"}}}}
		raw, _ := json.Marshal(obs)
		sqlMust(t, db, `INSERT INTO provider_snapshots(id,account_id,provider,data,canonical) VALUES(gen_random_uuid(),$1,$2,'{}',$3)`, x.Account, ps[i], raw)
		for _, uri := range x.URIs {
			u, _ := url.Parse(uri)
			expected[u.User.String()+"@"+u.Host] = labels[i]
		}
	}
	// Source URI checks exclude the display-only fragment and use the current
	// export policy. No native reads/writes or provider HTTP calls are available.
	var before string
	if err := db.QueryRow("SELECT md5(string_agg(uri,',' ORDER BY uri)) FROM output_config_snapshots").Scan(&before); err != nil {
		t.Fatal(err)
	}
	check := func(w *httptest.ResponseRecorder, min int) {
		t.Helper()
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		lines := strings.Fields(w.Body.String())
		if len(lines) < min {
			t.Fatal("missing output")
		}
		for _, line := range lines {
			u, e := url.Parse(line)
			if e != nil {
				t.Fatal(e)
			}
			want := expected[u.User.String()+"@"+u.Host]
			// The ALL fixture includes a copied identity at a Vultr panel.
			if u.Hostname() == "192.0.2.200" {
				want = labels[1]
			}
			if want == "" || u.Fragment != want {
				t.Fatalf("incorrect server country: host=%s got=%s want=%s", u.Host, u.Fragment, want)
			}
			if strings.Contains(u.Fragment, "dob:") || strings.Contains(u.RawFragment, "+") {
				t.Fatal("technical or incorrectly encoded name")
			}
		}
	}
	for _, class := range []string{"ALL", "DIRECT", "RESIDENTIAL"} {
		w := httptest.NewRecorder()
		s.outputConfigs(w, httptest.NewRequest("GET", "/?route_class="+class, nil))
		check(w, 4)
		token := "country-" + class
		sqlMust(t, db, "INSERT INTO output_share_tokens(token,route_class) VALUES($1,$2)", token, class)
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("token", token)
		w = httptest.NewRecorder()
		s.sharedOutput(w, r)
		check(w, 4)
		if class != "ALL" {
			for _, x := range f {
				check(readAccountOutput(s, accountOutputToken(t, s, x.Account, class), ""), 1)
			}
		}
	}
	var after string
	if err := db.QueryRow("SELECT md5(string_agg(uri,',' ORDER BY uri)) FROM output_config_snapshots").Scan(&after); err != nil || before != after {
		t.Fatal("stored source changed", err)
	}
	// No usable metadata must not manufacture a country or remove the link.
	sqlMust(t, db, "DELETE FROM provider_snapshots")
	sqlMust(t, db, "UPDATE droplets SET profile='{}'")
	w := httptest.NewRecorder()
	s.outputConfigs(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || len(strings.Fields(w.Body.String())) < 8 {
		t.Fatal("unknown metadata hid configs")
	}
	for _, line := range strings.Fields(w.Body.String()) {
		u, _ := url.Parse(line)
		if u.Fragment != "🌐 Unknown country" {
			t.Fatal(u.Fragment)
		}
	}
	// Creation-profile fallback is immediate, before first provider observation.
	sqlMust(t, db, `UPDATE droplets SET profile='{"Region":"fra1"}' WHERE id=$1`, f[0].Droplet)
	w = readAccountOutput(s, accountOutputToken(t, s, f[0].Account, "DIRECT"), "")
	u, e := url.Parse(strings.TrimSpace(w.Body.String()))
	if e != nil || u.Fragment != labels[0] {
		t.Fatal("creation-region fallback", w.Code)
	}
}
