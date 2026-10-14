package digitalocean

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestIPInventorySeparateFamiliesAndUnknownQuota(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		if r.Method != "GET" {
			t.Fatal("mutation")
		}
		switch {
		case strings.Contains(r.URL.Path, "account"):
			return 200, `{"account":{}}`
		case strings.Contains(r.URL.Path, "reserved_ipv6"):
			return 200, `{"reserved_ipv6s":[{"ip":"2001:db8::1","region_slug":"nyc3"}],"meta":{"total":1}}`
		default:
			return 200, `{"reserved_ips":[{"ip":"203.0.113.1","region":{"slug":"nyc3"},"droplet":{"id":9}}],"meta":{"total":1}}`
		}
	})
	d, _ := NewDriver(c)
	o, e := d.ObserveIPs(context.Background())
	if e != nil || len(o.Resources) != 2 || o.Quotas[0].Limit != nil || o.Quotas[1].Limit != nil {
		t.Fatal(o, e)
	}
	if o.Resources[0].ServerID != "9" || !o.Resources[1].Protected {
		t.Fatal("ownership inferred")
	}
}
func TestIPInventoryRejectsPartialAndDuplicate(t *testing.T) {
	for _, body := range []string{`{}`, `{"reserved_ips":null,"meta":{"total":0}}`, `{"reserved_ips":[],"meta":{"total":1}}`, `{"reserved_ips":[{"ip":"203.0.113.1","region":{"slug":"x"}},{"ip":"203.0.113.1","region":{"slug":"x"}}],"meta":{"total":2}}`} {
		c := fixtureClient(t, func(r *http.Request) (int, string) {
			if strings.Contains(r.URL.Path, "account") {
				return 200, `{"account":{"floating_ip_limit":10}}`
			}
			return 200, body
		})
		d, _ := NewDriver(c)
		if o, e := d.ObserveIPs(context.Background()); e == nil || o.Complete {
			t.Fatal(body, o, e)
		}
	}
}
