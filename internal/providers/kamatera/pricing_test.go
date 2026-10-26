package kamatera

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

// Minimal synthetic component fixtures; expected totals independently exercise
// the official additive formula, including a negative RAM adjustment.
const tariffFixture = `<select id="NumberOfIPs"><option value="1" data-price="0">1</option></select><script id="calculation-js-js-extra">
var KAMATERA_DATA = {"cpuOptions":[{"value":"1A","price":0},{"value":"2B","price":10},{"value":"1T","price":0}],"ramMB":{"A":[{"value":"1024","price":-2}],"B":[{"value":"4096","price":23}],"T":[{"value":"1024","price":-2}]},"diskGB":[{"value":"20","price":6},{"value":"30","price":8}],"netPck":{"EU":[{"value":"t5000","price":0}],"AS":[{"value":"t1000","price":0}]},"osList":[{"id":"ubuntu-id","name":"ubuntu_server_24.04_64-bit","price":0,"datacenters":["EU","AS"]}]};
//# sourceURL=calculation-js-js-extra
</script>`

func pricingImages() []imageData {
	return []imageData{{ID: "EU:ubuntu-id", Code: "ubuntu_server_24.04_64-bit", Name: "Ubuntu 24.04 64-bit", Disk: 10, RAM: map[string]int{"A": 1024, "B": 1024, "T": 1024}}}
}
func TestPublicTariffMatchesExactShapeAndRegion(t *testing.T) {
	if _, e := parsePublicTariff([]byte(tariffFixture + `<script>window.KAMATERA_DATA=window.KAMATERA_DATA||{};window.KAMATERA_DATA.defaultDC="US-NY2";</script>`)); e != nil {
		t.Fatal("unrelated calculator defaults", e)
	}
	p, e := parsePublicTariff([]byte(tariffFixture))
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		cpu              string
		cores, ram, disk int
		want             float64
	}{{"A", 1, 1024, 20, 4}, {"B", 2, 4096, 30, 41}, {"T", 1, 1024, 20, 4}} {
		q, ok := p.quote(sizeData{CPUType: tc.cpu, Cores: tc.cores, RAM: tc.ram, Disk: tc.disk}, "EU", pricingImages())
		if !ok || q.MonthlyEstimate != tc.want || math.Abs(q.Hourly-tc.want/730) > 1e-10 || q.Scope != publicPriceScope || !q.Complete || q.Currency != "USD" {
			t.Fatal(tc, q, ok)
		}
		sum := 0.0
		for _, c := range q.Components {
			sum += c.Hourly
		}
		if math.Abs(sum-q.Hourly) > 1e-10 {
			t.Fatal("breakdown", q)
		}
	}
	shape := sizeData{CPUType: "A", Cores: 1, RAM: 1024, Disk: 20}
	if _, ok := p.quote(shape, "MISSING", pricingImages()); ok {
		t.Fatal("cross-region quote")
	}
	shape.Disk = 21
	if _, ok := p.quote(shape, "EU", pricingImages()); ok {
		t.Fatal("rounded unsupported disk")
	}
}
func TestPublicTariffRejectsMissingAmbiguousOrPaidComponents(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{
		{"missing RAM price", `"value":"1024","price":-2`, `"value":"1024"`},
		{"null CPU price", `"value":"1A","price":0`, `"value":"1A","price":null`},
		{"duplicate JSON price key", `"value":"1A","price":0`, `"value":"1A","price":999,"price":0`},
		{"duplicate CPU", `"value":"1A","price":0`, `"value":"1A","price":0},{"value":"1A","price":3`},
		{"negative disk", `"value":"20","price":6`, `"value":"20","price":-6`},
		{"licensed image", `"price":0,"datacenters"`, `"price":10,"datacenters"`},
		{"other image", `"ubuntu_server_24.04_64-bit"`, `"other"`},
		{"wrong OS region", `"datacenters":["EU","AS"]`, `"datacenters":["AS"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := parsePublicTariff([]byte(strings.ReplaceAll(tariffFixture, tc.old, tc.new)))
			if e != nil {
				return
			}
			ims := pricingImages()
			ims[0].ID = "EU:other-id"
			if _, ok := p.quote(sizeData{CPUType: "A", Cores: 1, RAM: 1024, Disk: 20}, "EU", ims); ok {
				t.Fatal("unverified price accepted")
			}
		})
	}
	for _, raw := range []string{"", tariffFixture + tariffFixture, strings.Replace(tariffFixture, "};", "};alert(1);", 1), strings.Replace(tariffFixture, `data-price="0"`, `data-price="unknown"`, 1), strings.Repeat("x", maxPricingBytes+1)} {
		if _, e := parsePublicTariff([]byte(raw)); e == nil {
			t.Fatal("invalid document accepted")
		}
	}
	p, _ := parsePublicTariff([]byte(tariffFixture))
	ims := pricingImages()
	ims = append(ims, imageData{Code: "ubuntu_server_26.04_64-bit", Name: "Ubuntu 26.04 64-bit"})
	if _, ok := p.quote(sizeData{CPUType: "A", Cores: 1, RAM: 1024, Disk: 20}, "EU", ims); ok {
		t.Fatal("unpriced selectable OS ignored")
	}
}
func TestPublicTariffUsesAccountTransportWithoutSecretsOrRedirects(t *testing.T) {
	calls := 0
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != pricingURL || r.Method != "GET" || r.Header.Get("AuthClientId") != "" || r.Header.Get("AuthSecret") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("unsafe public request", r.URL)
		}
		return reply(tariffFixture), nil
	}, nil)
	p, e := d.publicTariff(context.Background())
	if e != nil || p.observed.IsZero() || calls != 1 {
		t.Fatal(e, calls)
	}
	calls = 0
	d.client.http.Transport = transportFixture(func(*http.Request) (*http.Response, error) {
		calls++
		r := reply("")
		r.StatusCode = 302
		r.Header.Set("Location", "https://elsewhere.invalid/")
		return r, nil
	})
	if _, e = d.publicTariff(context.Background()); e == nil || calls != 1 {
		t.Fatal("redirect followed", calls, e)
	}
	d.client.http.Transport = transportFixture(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = d.publicTariff(ctx); e == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestCatalogPublishesRegionalPriceOrAdvisoryFailure(t *testing.T) {
	for _, available := range []bool{true, false} {
		d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
			switch {
			case r.URL.Host == "www.kamatera.com":
				if available {
					return reply(tariffFixture), nil
				}
				return nil, io.ErrUnexpectedEOF
			case r.URL.Query().Get("datacenter") == "1":
				return reply(`[{"id":"EU","name":"The Netherlands","subCategory":"Amsterdam"}]`), nil
			case r.URL.Query().Get("sizes") == "1":
				return reply(`[{"id":"small","ramMB":4096,"diskSizeGB":30,"cpuType":"B","cpuCores":2,"monthlyTrafficPackage":"t5000"}]`), nil
			case r.URL.Query().Get("images") == "1":
				b, _ := json.Marshal(pricingImages())
				return reply(string(b)), nil
			case r.URL.Query().Get("capabilities") == "1":
				return reply(capabilitiesFixture), nil
			}
			t.Fatal("unexpected request", r.URL)
			return nil, nil
		}, nil)
		c, e := d.Catalog(context.Background())
		if e != nil {
			t.Fatal("price failure blocked validation", e)
		}
		found := false
		for _, p := range c.Plans {
			if p.ID == "2B-4096-30" {
				found = true
				q, ok := p.PricesByRegion["EU"]
				if ok != available || available && q.MonthlyEstimate != 41 {
					t.Fatal(q, ok)
				}
				if p.DetailsByRegion["EU"].CatalogOrigin != "provider_preset" || p.DetailsByRegion["EU"].BillingCycle != "hourly" {
					t.Fatal(p)
				}
			}
		}
		if !found || available && c.Pricing.Status != "partial" || !available && c.Pricing.Status != "unavailable" {
			t.Fatal(c.Pricing, found)
		}
	}
}
