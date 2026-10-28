package kamatera

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const nativeLicenceFixture = `{"serverCategoryDiskImages":[{"diskImages":[{"id":"EU:ubuntu-id","licensePrice":0}]}]}`

func TestKamateraNativeLicenceRequiresExactRegionalEvidence(t *testing.T) {
	p, err := parsePublicTariff([]byte(strings.Replace(tariffFixture, `"datacenters":["EU","AS"]`, `"datacenters":["AS"]`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	ims := pricingImages()
	shape := sizeData{CPUType: "B", Cores: 2, RAM: 4096, Disk: 30}
	if !p.needsNativeLicences("EU", ims) {
		t.Fatal("missing mapping was not detected")
	}
	if _, ok := p.quote(shape, "EU", ims); ok {
		t.Fatal("unverified licence quoted")
	}
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"exact_zero", nativeLicenceFixture, true},
		{"paid", strings.Replace(nativeLicenceFixture, `"licensePrice":0`, `"licensePrice":10`, 1), false},
		{"missing", strings.Replace(nativeLicenceFixture, `,"licensePrice":0`, "", 1), false},
		{"null", strings.Replace(nativeLicenceFixture, `"licensePrice":0`, `"licensePrice":null`, 1), false},
		{"other_region", strings.Replace(nativeLicenceFixture, "EU:ubuntu-id", "AS:ubuntu-id", 1), false},
		{"other_image", strings.Replace(nativeLicenceFixture, "ubuntu-id", "other", 1), false},
		{"duplicate_field", strings.Replace(nativeLicenceFixture, `"licensePrice":0`, `"licensePrice":20,"licensePrice":0`, 1), false},
		{"duplicate_image", strings.Replace(nativeLicenceFixture, `"licensePrice":0}`, `"licensePrice":0},{"id":"EU:ubuntu-id","licensePrice":0}`, 1), false},
		{"missing_groups", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, e := parseNativeImageLicences([]byte(tc.raw), "EU", ims)
			if e != nil {
				v = nil
			}
			q, ok := p.quoteWithNativeLicences(shape, "EU", ims, v)
			if ok != tc.want || ok && q.MonthlyEstimate != 41 {
				t.Fatal(ok, q, e)
			}
		})
	}
	// Even valid native zero cannot overwrite a conflicting public licence price.
	native, _ := parseNativeImageLicences([]byte(nativeLicenceFixture), "EU", ims)
	paid := *p
	paid.Images = append([]tariffImage(nil), p.Images...)
	price := 10.0
	paid.Images[0].Price = &price
	if _, ok := paid.quoteWithNativeLicences(shape, "EU", ims, native); ok {
		t.Fatal("conflicting licence evidence accepted")
	}
}

func TestKamateraNativeLicenceReadPinsHostAndRejectsRedirects(t *testing.T) {
	for _, status := range []int{200, 202, 302, 401, 500} {
		calls := 0
		d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "GET" || r.URL.String() != "https://console.kamatera.com/svc/serverCreate/datacenterConfiguration/EU" || r.Header.Get("AuthClientId") != "fixture-client" || r.Header.Get("AuthSecret") != "fixture-secret" || r.Header.Get("Cookie") != "" {
				t.Fatal("incorrect native licence request")
			}
			response := reply(nativeLicenceFixture)
			response.StatusCode = status
			response.Header.Set("Location", "https://elsewhere.invalid/")
			return response, nil
		}, nil)
		out := d.nativeImageLicences(context.Background(), "EU", pricingImages())
		if out["EU:ubuntu-id"] != (status == 200) || calls != 1 {
			t.Fatal(status, out, calls)
		}
	}
}

func TestKamateraCatalogFillsMissingMappingWithNativeLicence(t *testing.T) {
	nativeCalls := 0
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Host == "console.kamatera.com":
			nativeCalls++
			return reply(nativeLicenceFixture), nil
		case r.URL.Host == "www.kamatera.com":
			return reply(strings.Replace(tariffFixture, `"datacenters":["EU","AS"]`, `"datacenters":["AS"]`, 1)), nil
		case r.URL.Query().Get("datacenter") == "1":
			return reply(`[{"id":"EU","name":"The Netherlands","subCategory":"Amsterdam"}]`), nil
		case r.URL.Query().Get("sizes") == "1":
			return reply(`[{"id":"small","ramMB":4096,"diskSizeGB":30,"cpuType":"B","cpuCores":2}]`), nil
		case r.URL.Query().Get("images") == "1":
			raw, _ := json.Marshal(pricingImages())
			return reply(string(raw)), nil
		case r.URL.Query().Get("capabilities") == "1":
			return reply(capabilitiesFixture), nil
		default:
			t.Fatal("unexpected catalogue request", r.URL)
			return nil, nil
		}
	}, nil)
	cat, err := d.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, plan := range cat.Plans {
		if plan.ID == "2B-4096-30" {
			found = plan.PricesByRegion["EU"].MonthlyEstimate == 41
		}
	}
	if !found || nativeCalls != 1 {
		t.Fatal("fallback not cached across regional plans", found, nativeCalls)
	}
}
