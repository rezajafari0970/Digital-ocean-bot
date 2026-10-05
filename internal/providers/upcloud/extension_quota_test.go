package upcloud

import (
	"context"
	"net/http"
	"testing"
)

// The live trace reveals only that an extra quota is null; never fabricate the
// user's actual key. This synthetic future key tests extension compatibility.
const futureQuota = "cloud_server_future_1xcpu_1gb_plans"
const futurePlan = "FUTURE-1xCPU-1GB"

func extensionFixture(t *testing.T, limit, used any) *Driver {
	t.Helper()
	body := nullableQuotaAccount()
	body["account"].(map[string]any)["resource_limits"].(map[string]any)[futureQuota] = limit
	return fixture(t, func(r *http.Request) (int, any, error) {
		switch r.URL.Path {
		case "/1.3/account":
			return 200, body, nil
		case "/1.3/server":
			return 200, map[string]any{"servers": map[string]any{"server": []any{}}}, nil
		case "/1.3/account/resource-usage":
			return 200, map[string]any{"cores": 4, "memory": 4096, "public_ipv4": 3, "storage_total": 100, "storage_maxiops": 100, futureQuota: used}, nil
		case "/1.3/plan":
			return 200, map[string]any{"plans": map[string]any{"plan": []any{
				map[string]any{"name": "1xCPU-1GB", "core_number": 1, "memory_amount": 1024, "storage_size": 25, "storage_tier": "maxiops"},
				map[string]any{"name": futurePlan, "core_number": 1, "memory_amount": 1024, "storage_size": 25, "storage_tier": "maxiops"},
			}}}, nil
		case "/1.3/zone":
			return 200, map[string]any{"zones": map[string]any{"zone": []any{map[string]any{"id": "fi-hel1", "description": "Helsinki", "public": "yes"}}}}, nil
		case "/1.3/price":
			return 503, map[string]any{}, nil
		case "/1.3/storage/template":
			return 200, map[string]any{"storages": map[string]any{"storage": []any{templateJSON().(map[string]any)["storage"]}}}, nil
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
			return 500, nil, nil
		}
	}, nil)
}

func TestExtensionNullQuotaPreviewAndRegularCapacity(t *testing.T) {
	ctx := context.Background()
	d := extensionFixture(t, nil, nil)
	d.planIDs = nil
	a, err := d.Account(ctx)
	if err != nil || a.ID == "" {
		t.Fatalf("account %v", err)
	}
	c, err := d.Capacity(ctx)
	if err != nil || c.LimitKnown {
		t.Fatalf("preview capacity %+v %v", c, err)
	}
	cat, err := d.Catalog(ctx)
	if err != nil || len(cat.Regions) != 1 || len(cat.Plans) != 2 || len(cat.Images) != 1 {
		t.Fatalf("catalog %+v %v", cat, err)
	}
	d.planIDs = []string{"1xCPU-1GB"}
	c, err = d.Capacity(ctx)
	if err != nil || !c.LimitKnown || c.PlanAvailable["1xCPU-1GB"] != 6 {
		t.Fatalf("regular capacity %+v %v", c, err)
	}
}

func TestSelectedExtensionQuotaAndUsageRemainFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		limit, used any
		want        int
		reason      string
	}{
		{"null-limit", nil, 0, 0, "UNKNOWN_PLAN_QUOTA"},
		{"null-usage", 3, nil, 0, "UNKNOWN_RESOURCE_USAGE"},
		{"zero-limit", 0, 0, 0, ""},
		{"remaining", 3, 1, 2, ""},
		{"numeric-string", "3", "1", 2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := extensionFixture(t, tc.limit, tc.used)
			d.planIDs = []string{futurePlan}
			c, err := d.Capacity(context.Background())
			if tc.reason != "" {
				if err == nil || c.LimitKnown || Diagnostic(err) != tc.reason {
					t.Fatalf("unknown became available %+v %v %s", c, err, Diagnostic(err))
				}
			} else if err != nil || !c.LimitKnown || c.PlanAvailable[futurePlan] != tc.want {
				t.Fatalf("capacity %+v %v", c, err)
			}
		})
	}
}
