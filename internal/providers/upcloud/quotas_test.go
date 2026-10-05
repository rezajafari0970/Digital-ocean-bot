package upcloud

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// Both nullable fields are specified by accountResourceLimits in UpCloud's
// current OpenAPI. This synthetic fixture contains no user account data.
func nullableQuotaAccount() map[string]any {
	return map[string]any{"account": map[string]any{
		"username": "fixture-account", "credits": 12.25,
		"resource_limits": map[string]any{
			"cores": 10, "memory": 16384, "public_ipv4": 10,
			"storage_total": 500, "storage_maxiops": 250,
			"cloud_server_dev_1xcpu_1gb_10gb_plans": nil,
			"cloud_server_dev_1xcpu_1gb_plans":      nil,
		},
	}}
}

func TestPreviewSequenceAcceptsDocumentedNullablePlanQuotas(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		switch r.URL.Path {
		case "/1.3/account":
			return 200, nullableQuotaAccount(), nil
		case "/1.3/server":
			return 200, map[string]any{"servers": map[string]any{"server": []any{}}}, nil
		case "/1.3/zone":
			return 200, map[string]any{"zones": map[string]any{"zone": []any{map[string]any{"id": "fi-hel1", "description": "Helsinki", "public": "yes"}}}}, nil
		case "/1.3/plan":
			return 200, planJSON(), nil
		case "/1.3/price":
			return 503, map[string]any{}, nil
		case "/1.3/storage/template":
			return 200, map[string]any{"storages": map[string]any{"storage": []any{templateJSON().(map[string]any)["storage"]}}}, nil
		default:
			t.Fatalf("unexpected preview request: %s", r.URL.Path)
			return 500, nil, nil
		}
	}, nil)
	d.planIDs = nil
	ctx := context.Background()
	a, err := d.Account(ctx)
	if err != nil || a.ID == "" {
		t.Fatalf("account: %v", err)
	}
	c, err := d.Capacity(ctx)
	if err != nil || c.LimitKnown {
		t.Fatalf("preview capacity: %+v %v", c, err)
	}
	cat, err := d.Catalog(ctx)
	if err != nil || len(cat.Plans) != 2 || len(cat.Regions) != 1 || len(cat.Images) != 1 {
		t.Fatalf("catalog: %+v %v", cat, err)
	}
}

func TestCapacityDistinguishesNullAndZeroDevQuotas(t *testing.T) {
	for _, plan := range []string{"DEV-1xCPU-1GB-10GB", "DEV-1xCPU-1GB"} {
		for _, tc := range []struct {
			name  string
			quota any
			known bool
			slots int
		}{
			{"null", nil, false, 0}, {"zero", 0, true, 0}, {"positive", 3, true, 2}, {"numeric-string", "3", true, 2},
		} {
			t.Run(plan+"/"+tc.name, func(t *testing.T) {
				key := "cloud_server_dev_1xcpu_1gb_plans"
				if plan == "DEV-1xCPU-1GB-10GB" {
					key = "cloud_server_dev_1xcpu_1gb_10gb_plans"
				}
				body := nullableQuotaAccount()
				body["account"].(map[string]any)["resource_limits"].(map[string]any)[key] = tc.quota
				usage := map[string]any{"cores": 1, "memory": 1024, "public_ipv4": 1, "storage_total": 10, "storage_maxiops": 10, key: 1}
				d := fixture(t, func(r *http.Request) (int, any, error) {
					switch r.URL.Path {
					case "/1.3/account":
						return 200, body, nil
					case "/1.3/server":
						return 200, map[string]any{"servers": map[string]any{"server": []any{}}}, nil
					case "/1.3/account/resource-usage":
						return 200, usage, nil
					case "/1.3/plan":
						return 200, map[string]any{"plans": map[string]any{"plan": []any{map[string]any{"name": plan, "core_number": 1, "memory_amount": 1024, "storage_size": 10, "storage_tier": "maxiops"}}}}, nil
					default:
						t.Fatalf("unexpected %s", r.URL.Path)
						return 500, nil, nil
					}
				}, nil)
				d.planIDs = []string{plan}
				c, err := d.Capacity(context.Background())
				if !tc.known {
					if err == nil || c.LimitKnown || providers.Class(err) != providers.ErrorUnavailable || Diagnostic(err) != "UNKNOWN_PLAN_QUOTA" {
						t.Fatalf("unknown selected quota must fail closed: %+v %v %s", c, err, Diagnostic(err))
					}
				} else if err != nil || !c.LimitKnown || c.PlanAvailable[plan] != tc.slots {
					t.Fatalf("capacity: %+v %v", c, err)
				}
			})
		}
	}
}

func TestRegularPlanCapacityIgnoresUnrelatedNullableDevQuotas(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		switch r.URL.Path {
		case "/1.3/account":
			return 200, nullableQuotaAccount(), nil
		case "/1.3/server":
			return 200, map[string]any{"servers": map[string]any{"server": []any{}}}, nil
		case "/1.3/plan":
			return 200, planJSON(), nil
		case "/1.3/account/resource-usage":
			return 200, map[string]any{"cores": 4, "memory": 4096, "public_ipv4": 3, "storage_total": 100, "storage_maxiops": 100}, nil
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
			return 500, nil, nil
		}
	}, nil)
	c, err := d.Capacity(context.Background())
	if err != nil || !c.LimitKnown || c.PlanAvailable["1xCPU-1GB"] != 6 {
		t.Fatalf("capacity: %+v %v", c, err)
	}
}

func TestQuotaInvalidNumbersStillFailClosed(t *testing.T) {
	for _, key := range []string{"cores", "memory", "public_ipv4", "storage_total", "storage_maxiops", "cloud_server_dev_1xcpu_1gb_plans", "cloud_server_dev_1xcpu_1gb_10gb_plans", "unknown_future_field"} {
		for _, raw := range []string{"-1", "1.5", `"not-a-number"`, "1125899906842625", "{}", "true"} {
			var a accountData
			if err := json.Unmarshal([]byte(`{"username":"fixture","resource_limits":{"`+key+`":`+raw+`}}`), &a); err == nil {
				t.Fatalf("invalid quota accepted: %s=%s", key, raw)
			}
		}
	}
	for _, key := range []string{"cores", "memory", "public_ipv4", "storage_total", "storage_maxiops"} {
		var a accountData
		if err := json.Unmarshal([]byte(`{"username":"fixture","resource_limits":{"`+key+`":null}}`), &a); err == nil {
			t.Fatalf("unexpected nullable quota accepted: %s", key)
		}
	}
}
