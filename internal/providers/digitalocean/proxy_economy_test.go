package digitalocean

import (
	"context"
	"net/http"
	"testing"
)

func TestEconomyObservationHTTPBudget(t *testing.T) {
	for _, fast := range []bool{false, true} {
		calls := map[string]int{}
		c := fixtureClient(t, func(r *http.Request) (int, string) {
			calls[r.URL.Path]++
			switch r.URL.Path {
			case "/v2/account":
				return 200, `{"account":{"uuid":"fixture","status":"active","droplet_limit":7}}`
			case "/v2/droplets":
				return 200, `{"droplets":[{"id":42}]}`
			case "/v2/regions":
				return 200, `{"regions":[]}`
			case "/v2/sizes":
				return 200, `{"sizes":[]}`
			case "/v2/images":
				return 200, `{"images":[]}`
			default:
				t.Errorf("unexpected %s", r.URL.Path)
				return 500, ""
			}
		})
		d, _ := NewDriver(c)
		if fast {
			obs, err := d.ObserveFast(context.Background())
			if err != nil || obs.Capacity.ComputeLimit != 7 || obs.Capacity.ComputeInUse != 1 {
				t.Fatalf("obs=%+v err=%v", obs, err)
			}
		} else {
			if _, err := d.Observe(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if calls["/v2/account"] != 1 || calls["/v2/droplets"] != 1 {
			t.Fatalf("duplicate account/inventory: %v", calls)
		}
		want := 5
		if fast {
			want = 2
		}
		if len(calls) != want {
			t.Fatalf("fast=%t calls=%v", fast, calls)
		}
	}
}
func TestEconomyFastObservationRejectsRevokedAccount(t *testing.T) {
	calls := 0
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		calls++
		return 401, `{"id":"unauthorized","message":"invalid token"}`
	})
	d, _ := NewDriver(c)
	if _, err := d.ObserveFast(context.Background()); err == nil {
		t.Fatal("revoked account admitted")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
