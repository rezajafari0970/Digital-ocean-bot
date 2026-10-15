package upcloud

import (
	"context"
	"net/http"
	"testing"
)

func TestIPInventoryDoesNotOwnDiscoveredAddresses(t *testing.T) {
	d := fixture(t, func(r *http.Request) (int, any, error) {
		if r.Method != "GET" {
			t.Fatal("mutation")
		}
		switch r.URL.Path {
		case "/1.3/account":
			return 200, map[string]any{"account": map[string]any{"username": "fixture", "credits": "1.0", "resource_limits": map[string]any{"public_ipv4": 5}}}, nil
		case "/1.3/ip_address":
			return 200, map[string]any{"ip_addresses": map[string]any{"ip_address": []any{map[string]any{"address": "203.0.113.2", "family": "IPv4", "access": "public", "floating": "yes", "server": serverID, "zone": "fi-hel1"}, map[string]any{"address": "2001:db8::1", "family": "IPv6", "access": "public", "is_delegated": "yes", "server": serverID, "zone": "fi-hel1"}}}}, nil
		}
		t.Fatal(r.URL.Path)
		return 500, nil, nil
	}, nil)
	o, e := d.ObserveIPs(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(o.Resources) != 2 || o.Resources[0].Kind != "floating" || o.Resources[1].Kind != "delegated" || !o.Resources[1].Protected || o.Quotas[1].Limit != nil {
		t.Fatal(o)
	}
}

func TestIPInventoryUtilityDoesNotHidePublicCapacity(t *testing.T) {
	for _, tc := range []struct {
		access    string
		wantError bool
	}{{"utility", false}, {"private", false}, {"unknown", true}, {"", true}} {
		t.Run(tc.access, func(t *testing.T) {
			calls := 0
			d := fixture(t, func(r *http.Request) (int, any, error) {
				calls++
				if r.Method != "GET" {
					t.Fatal("inventory attempted mutation")
				}
				switch r.URL.Path {
				case "/1.3/account":
					return 200, map[string]any{"account": map[string]any{"username": "fixture", "credits": "1.0", "resource_limits": map[string]any{"public_ipv4": 5}}}, nil
				case "/1.3/ip_address":
					return 200, map[string]any{"ip_addresses": map[string]any{"ip_address": []any{
						map[string]any{"address": "10.0.0.2", "family": "IPv4", "access": tc.access, "server": serverID, "zone": "fi-hel1"},
						map[string]any{"address": "203.0.113.2", "family": "IPv4", "access": "public", "floating": "yes", "server": serverID, "zone": "fi-hel1"},
					}}}, nil
				}
				t.Fatal(r.URL.Path)
				return 500, nil, nil
			}, nil)
			o, e := d.ObserveIPs(context.Background())
			if tc.wantError {
				if e == nil || o.Complete {
					t.Fatal("unknown access accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if calls != 2 || !o.Complete || len(o.Resources) != 1 || o.Resources[0].Address != "203.0.113.2" || !o.Resources[0].Protected || len(o.Quotas) != 2 || o.Quotas[0].Used == nil || *o.Quotas[0].Used != 1 || o.Quotas[0].Limit == nil || *o.Quotas[0].Limit != 5 || o.Quotas[1].Limit != nil {
				t.Fatalf("invalid public inventory: %+v calls=%d", o, calls)
			}
		})
	}
}
