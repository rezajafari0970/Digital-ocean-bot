package upcloud

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func priceCatalog() providers.Catalog {
	return providers.Catalog{Regions: []providers.Region{{ID: "fi-hel1"}, {ID: "es-mad1"}}, Plans: []providers.Plan{{ID: "1xCPU-1GB"}, {ID: "DEV-1xCPU-1GB"}}}
}

func TestRegionalPricesCurrencyUnitsAndOptionalFailures(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		currency      string
		value, amount any
		want          float64
	}{
		{"number", 200, "EUR", 0.75, 1, 0.0075},
		{"numeric-string", 200, "USD", "0.5", "1", 0.005},
		{"zero", 200, "EUR", 0, 1, 0},
		{"negative", 200, "EUR", -1, 1, 0},
		{"null", 200, "EUR", nil, 1, 0},
		{"malformed", 200, "EUR", "broken", 1, 0},
		{"nan", 200, "EUR", "NaN", 1, 0},
		{"infinity", 200, "EUR", "Infinity", 1, 0},
		{"overflow", 200, "EUR", "1e999", 1, 0},
		{"wrong-amount", 200, "EUR", 1, 2, 0},
		{"missing-amount", 200, "EUR", 1, nil, 0},
		{"bad-currency", 200, "eur", 1, 1, 0},
		{"http-failure", 503, "EUR", 1, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			d := fixture(t, func(r *http.Request) (int, any, error) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/1.3/price" {
					t.Fatal("unexpected request", r.URL.Path)
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 8*time.Second {
					t.Fatal("missing bounded budget")
				}
				return tc.status, map[string]any{"prices": map[string]any{"currency": tc.currency, "zone": []any{
					map[string]any{"name": "fi-hel1", "server_plan_1xCPU-1GB": map[string]any{"amount": tc.amount, "price": tc.value}, "unrelated_extension": nil},
					map[string]any{"name": "es-mad1", "server_plan_DEV-1xCPU-1GB": map[string]any{"amount": 1, "price": 1}},
					map[string]any{"name": "unknown-zone", "server_plan_1xCPU-1GB": map[string]any{"amount": 1, "price": 1}},
				}}}, nil
			}, nil)
			cat := priceCatalog()
			d.enrichPrices(context.Background(), &cat)
			p, ok := cat.Plans[0].PricesByRegion["fi-hel1"]
			if tc.want == 0 {
				if ok {
					t.Fatalf("invalid price accepted: %+v", p)
				}
			} else {
				if !ok || p.Currency != tc.currency || math.Abs(p.Hourly-tc.want) > 1e-10 || math.Abs(p.MonthlyEstimate-tc.want*672) > 1e-10 {
					t.Fatalf("wrong units/currency %+v", p)
				}
			}
			if calls != 1 {
				t.Fatal("retried", calls)
			}
			if len(cat.Plans[0].PricesByRegion) > 1 || cat.Plans[0].PriceMonthly != 0 {
				t.Fatal("cross-zone/scalar price leak")
			}
		})
	}
}

func TestLegacyPriceCurrencyLookupAndEscapedUsername(t *testing.T) {
	for _, currency := range []string{"GBP", ""} {
		var paths []string
		d := fixture(t, func(r *http.Request) (int, any, error) {
			paths = append(paths, r.URL.EscapedPath())
			switch r.URL.EscapedPath() {
			case "/1.3/price":
				return 200, map[string]any{"prices": map[string]any{"zone": []any{map[string]any{"name": "fi-hel1", "server_plan_1xCPU-1GB": map[string]any{"amount": 1, "price": "0.5"}}}}}, nil
			case "/1.3/account":
				return 200, map[string]any{"account": map[string]any{"username": "fixture/name +?"}}, nil
			case "/1.3/account/details/fixture%2Fname%20+%3F":
				return 200, map[string]any{"account": map[string]any{"currency": currency}}, nil
			default:
				t.Fatal("unexpected path", r.URL.EscapedPath())
				return 500, nil, nil
			}
		}, nil)
		cat := priceCatalog()
		d.enrichPrices(context.Background(), &cat)
		if len(paths) != 3 {
			t.Fatal(paths)
		}
		got, ok := cat.Plans[0].PricesByRegion["fi-hel1"]
		if currency == "" && ok {
			t.Fatal("assumed currency")
		}
		if currency != "" && (!ok || got.Currency != currency) {
			t.Fatal(got)
		}
	}
}

func TestDuplicateZonePricesAreUnavailable(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		zone := map[string]any{"name": "fi-hel1", "server_plan_1xCPU-1GB": map[string]any{"amount": 1, "price": 1}}
		return 200, map[string]any{"prices": map[string]any{"currency": "EUR", "zone": []any{zone, zone}}}, nil
	}, nil)
	cat := priceCatalog()
	d.enrichPrices(context.Background(), &cat)
	if len(cat.Plans[0].PricesByRegion) != 0 {
		t.Fatal("ambiguous duplicate accepted")
	}
}

func TestPricingUsesCallerCancellation(t *testing.T) {
	calls := 0
	d := fixture(t, func(r *http.Request) (int, any, error) {
		calls++
		<-r.Context().Done()
		return 0, nil, r.Context().Err()
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	cat := priceCatalog()
	d.enrichPrices(ctx, &cat)
	if calls != 1 || len(cat.Plans[0].PricesByRegion) != 0 {
		t.Fatal("timeout retried or fabricated price")
	}
}

func TestCatalogPriceAndTemplateVariants(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		switch r.URL.Path {
		case "/1.3/zone":
			return 200, map[string]any{"zones": map[string]any{"zone": []any{map[string]any{"id": "fi-hel1", "public": "yes", "description": "Helsinki"}}}}, nil
		case "/1.3/plan":
			return 200, planJSON(), nil
		case "/1.3/price":
			return 200, map[string]any{"prices": map[string]any{"currency": "EUR", "zone": []any{map[string]any{"name": "fi-hel1", "server_plan_1xCPU-1GB": map[string]any{"amount": 1, "price": 0.75}}}}}, nil
		case "/1.3/storage/template":
			return 200, json.RawMessage(`{"storages":{"storage":[{"uuid":"variant-one","title":"Ubuntu Server 24.04 LTS","template_type":"cloud-init","type":"template","access":"public","state":"online"},{"uuid":"variant-two","title":"Ubuntu Server 24.04 LTS Minimal","template_type":"cloud-init","type":"template","access":"public","state":"online"}]}}`), nil
		default:
			t.Fatal(r.URL.Path)
			return 500, nil, nil
		}
	}, nil)
	cat, err := d.Catalog(context.Background())
	if err != nil || len(cat.Images) != 2 {
		t.Fatal(cat, err)
	}
	if cat.Plans[0].PricesByRegion["fi-hel1"].Currency != "EUR" {
		t.Fatal("price missing")
	}
	got := []string{cat.Images[0].ID, cat.Images[1].ID}
	if !reflect.DeepEqual(got, []string{"variant-one", "variant-two"}) || cat.Images[0].Name == cat.Images[1].Name {
		t.Fatal(cat.Images)
	}
	if cat.Images[0].Name != "Ubuntu Server 24.04 LTS (cloud-init)" {
		t.Fatal(cat.Images[0])
	}
}
