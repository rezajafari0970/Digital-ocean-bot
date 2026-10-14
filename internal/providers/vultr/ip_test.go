package vultr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIPInventoryDoesNotRetry429(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate"}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "test")
	c.base = s.URL
	d := &Driver{client: c}
	if _, e := d.ObserveIPs(context.Background()); e == nil || calls != 1 {
		t.Fatal(calls, e)
	}
}
func TestIPInventoryReservedSubnetAndPagination(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/reserved-ips" {
			t.Error("unexpected endpoint")
		}
		if r.URL.Query().Get("cursor") == "two" {
			w.Write([]byte(`{"reserved_ips":[{"id":"b","region":"ewr","ip_type":"v6","subnet":"2001:db8:1::","subnet_size":64}],"meta":{"total":2,"links":{"next":""}}}`))
			return
		}
		w.Write([]byte(`{"reserved_ips":[{"id":"a","region":"ewr","ip_type":"v4","subnet":"203.0.113.1","subnet_size":32}],"meta":{"total":2,"links":{"next":"https://untrusted.invalid/arbitrary?cursor=two"}}}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "test")
	c.base = s.URL
	d := &Driver{client: c}
	o, e := d.ObserveIPs(context.Background())
	if e != nil || calls != 2 || len(o.Resources) != 2 || o.Resources[1].Prefix != "2001:db8:1::/64" || o.Quotas[0].Limit != nil {
		t.Fatal(o, calls, e)
	}
}
