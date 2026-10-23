package ovh

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func offerJSON(code string, price int64) string {
	return `{"planCode":"` + code + `","pricingType":"consumption","pricings":[{"price":` + jsonNumber(price) + `,"mode":"default","type":"consumption","interval":1,"intervalUnit":"hour","quantity":{"min":1,"max":null}}]}`
}
func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }
func priceFixture() string {
	return `{"catalogId":123,"locale":{"currencyCode":"EUR","subsidiary":"FR"},"addons":[` +
		strings.Replace(offerJSON("b3-8.consumption", 5120000), `"pricingType"`, `"addonFamilies":[{"name":"local-storage","addons":["disk.hour"]}],"pricingType"`, 1) + "," +
		offerJSON("disk.hour", 14580) + "]}"
}
func TestPriceEvidenceExactUnitsAndComponents(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/order/catalog/") {
			if r.Header.Get("X-Ovh-Signature") != "" || r.Header.Get("X-Ovh-Consumer") != "" {
				t.Fatal("secret on public request")
			}
			if r.URL.Query().Get("ovhSubsidiary") != "FR" {
				t.Fatal("market mismatch")
			}
			return response(priceFixture()), nil
		}
		if strings.HasSuffix(r.URL.Path, "/me") {
			return &http.Response{StatusCode: 403, Body: response("{}").Body}, nil
		}
		return response(readFixture(r.URL.Path)), nil
	})
	p := d.pricing(context.Background())
	if p.meta.Status != "available" || p.meta.Basis != "public_reference" || p.meta.Currency != "EUR" || p.meta.ObservedAt.IsZero() {
		t.Fatal(p.meta)
	}
	f := flavorData{Disk: 50}
	f.PlanCodes.Hourly = "b3-8.consumption"
	rate, ok := p.flavor(f)
	if !ok || !rate.Complete || rate.Hourly != 0.05849 || len(rate.Components) != 2 || rate.MonthlyEstimate != rate.Hourly*730 {
		t.Fatal(rate, ok)
	}
	f.PlanCodes.Hourly = "b3-8-flex.consumption"
	if _, ok = p.flavor(f); ok {
		t.Fatal("guessed missing plan code")
	}
}
func TestPriceEvidenceRejectsAmbiguityAndWrongUnits(t *testing.T) {
	var valid catalogOffer
	if json.Unmarshal([]byte(offerJSON("test", 1040000)), &valid) != nil {
		t.Fatal("fixture")
	}
	for _, kind := range []string{"zero", "duplicate", "monthly", "commitment", "missing", "tiered"} {
		copy := valid
		copy.Pricings = append(copy.Pricings[:0:0], valid.Pricings...)
		switch kind {
		case "zero":
			n := int64(0)
			copy.Pricings[0].Price = &n
		case "duplicate":
			copy.Pricings = append(copy.Pricings, copy.Pricings[0])
		case "monthly":
			copy.Pricings[0].Unit = "month"
		case "commitment":
			copy.Pricings[0].Commitment = 12
		case "missing":
			copy.Pricings[0].Price = nil
		case "tiered":
			n := 10
			copy.Pricings[0].Quantity.Max = &n
		}
		p := priceEvidence{offers: map[string]catalogOffer{"test": copy}}
		rate, ok := p.rate("test")
		if kind == "zero" {
			if !ok || rate != 0 {
				t.Fatal("explicit zero lost")
			}
		} else if ok {
			t.Fatal("invalid rate accepted", kind)
		}
	}
}
func TestPricingFailureDoesNotBreakCatalog(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/order/catalog/") {
			return &http.Response{StatusCode: 503, Body: response("{}").Body}, nil
		}
		return response(readFixture(r.URL.Path)), nil
	})
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Plans) != 1 || !c.Plans[0].Available || c.Pricing.Status != "unavailable" {
		t.Fatal(c, e)
	}
}
func TestFullCatalogSeparatesLaunchPolicyAndNativePricing(t *testing.T) {
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/me") {
			return response(`{"ovhSubsidiary":"FR"}`), nil
		}
		if strings.Contains(r.URL.Path, "/order/catalog/") {
			return response(priceFixture()), nil
		}
		if strings.HasSuffix(r.URL.Path, "/flavor") {
			base := strings.TrimSuffix(strings.TrimPrefix(readFixture(r.URL.Path), "["), "]")
			variants := []string{}
			for i, name := range []string{"b3-8", "b3-8-flex", "i1-45", "a10-45", "win-b3-8"} {
				x := strings.Replace(base, "b3-8", name, 1)
				x = strings.Replace(x, flavorID, "22222222-2222-2222-2222-22222222222"+string(rune('0'+i)), 1)
				x = strings.Replace(x, `"available":true`, `"available":true,"quota":0,"planCodes":{"hourly":"b3-8.consumption"}`, 1)
				if strings.HasPrefix(name, "win-") {
					x = strings.Replace(x, `"osType":"linux"`, `"osType":"windows"`, 1)
				}
				variants = append(variants, x)
			}
			return response("[" + strings.Join(variants, ",") + "]"), nil
		}
		return response(readFixture(r.URL.Path)), nil
	})
	c, e := d.Catalog(context.Background())
	if e != nil || len(c.Plans) != 5 || c.Pricing.Basis != "account_market" {
		t.Fatal(c, e)
	}
	eligible := 0
	for _, p := range c.Plans {
		x := p.DetailsByRegion["GRA11"]
		if x.Quota == nil || *x.Quota != 0 || p.PricesByRegion["GRA11"].Hourly <= 0 {
			t.Fatal(p)
		}
		if p.Available {
			eligible++
			if p.IDsByRegion["GRA11"] == "" {
				t.Fatal(p)
			}
		} else if len(p.IDsByRegion) != 0 || x.Reason == "" {
			t.Fatal("unsupported launch leaked")
		}
	}
	if eligible != 3 || len(providers.CatalogRegionsWithOptions(c)) != 1 {
		t.Fatal("policy mismatch")
	}
}
func TestFlexAndIOPSUseExistingCreateContract(t *testing.T) {
	for _, name := range []string{"b3-8-flex", "i1-45"} {
		t.Run(name, func(t *testing.T) {
			var d *Driver
			posts := 0
			d = openTest(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPost {
					posts++
					return response(`{"id":"` + serverID + `","name":"` + d.name("fixed") + `","region":"GRA11","monthlyBilling":null}`), nil
				}
				return response(strings.ReplaceAll(readFixture(r.URL.Path), "b3-8", name)), nil
			})
			_, err := d.CreateServer(context.Background(), providers.CreateServerRequest{Identity: "fixed", RegionID: "GRA11", PlanID: flavorID, ImageID: imageID, SSHAuthorizedKeys: []string{key(t)}})
			if err != nil || posts != 1 {
				t.Fatal(err, posts)
			}
		})
	}
}

func TestFlavorQuotaZeroRejectsBeforeMutation(t *testing.T) {
	posts := 0
	d := openTest(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			posts++
		}
		body := readFixture(r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/flavor") {
			body = strings.Replace(body, `"available":true`, `"available":true,"quota":0`, 1)
		}
		return response(body), nil
	})
	_, err := d.CreateServer(context.Background(), providers.CreateServerRequest{Identity: "fixed", RegionID: "GRA11", PlanID: flavorID, ImageID: imageID, SSHAuthorizedKeys: []string{key(t)}})
	if !providers.IsClass(err, providers.ErrorCapacity) || posts != 0 {
		t.Fatal(err, posts)
	}
}
