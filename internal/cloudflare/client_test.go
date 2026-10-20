package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudflareDriverPaginationAuthAndRedaction(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-fixture" {
			t.Error("missing scoped auth")
		}
		count++
		if strings.Contains(r.URL.Path, "dns_records") {
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(429)
			w.Write([]byte(`{"secret":"secret-fixture"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []Zone{{ID: r.URL.Query().Get("page"), Name: "example.com", Type: "full", Status: "active"}}, "result_info": map[string]int{"total_pages": 2}})
	}))
	defer server.Close()
	c := NewClient(Auth{Token: "secret-fixture"})
	c.base = server.URL
	c.HTTP = server.Client()
	zones, e := c.Zones(context.Background())
	if e != nil || len(zones) != 2 || count != 2 {
		t.Fatal(zones, e, count)
	}
	_, e = c.CreateRecord(context.Background(), "zone", Record{})
	var api *APIError
	if !errors.As(e, &api) || api.Status != 429 || api.RetryAfter != 90*time.Second || strings.Contains(e.Error(), "secret") {
		t.Fatal("unsafe API error", e)
	}
}
func TestCloudflareAmbiguousWriteIsNotRetriedByDriver(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes++; w.Write([]byte(`{"success":true,"result":`)) }))
	defer server.Close()
	c := NewClient(Auth{Token: "fixture"})
	c.base = server.URL
	c.HTTP = server.Client()
	if _, e := c.CreateRecord(context.Background(), "zone", Record{}); e == nil {
		t.Fatal("ambiguous write accepted")
	}
	if writes != 1 {
		t.Fatal("blind retry", writes)
	}
}
func TestCloudflareNamesLifetimeAndOwnership(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		h, e := Hostname("example.com")
		if e != nil || seen[h] || len(strings.Split(h, ".")[0]) > 63 {
			t.Fatal("hostname", h, e)
		}
		seen[h] = true
		d, e := Lifetime(2700, 3600)
		if e != nil || d < 45*time.Minute || d > time.Hour {
			t.Fatal(d, e)
		}
	}
	for _, bad := range []string{"example.com;id", "-example.com", "a..example.com", "Example.com", "127.0.0.1"} {
		if _, e := Hostname(bad); e == nil {
			t.Fatal("unsafe zone", bad)
		}
	}
	a := Record{Name: "test.example.com", Type: "A", Content: "192.0.2.1", Comment: "owned", Proxied: true}
	b := a
	b.Comment = "foreign"
	if Owns(a, b) {
		t.Fatal("foreign record owned")
	}
	b = a
	b.Content = "192.0.2.2"
	if Owns(a, b) {
		t.Fatal("different origin owned")
	}
}

func TestCloudflarePermissionErrorsIdentifyOperationWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"sensitive-fixture-token domain.example.com"}]}`))
	}))
	defer server.Close()
	c := NewClient(Auth{Token: "sensitive-fixture-token"})
	c.base = server.URL
	c.HTTP = server.Client()
	for _, tc := range []struct {
		name, want string
		run        func() error
	}{
		{"dns read", "cloudflare_dns_read_permission_denied", func() error { _, e := c.Records(context.Background(), "zone", "domain.example.com"); return e }},
		{"dns write", "cloudflare_dns_write_permission_denied", func() error { _, e := c.CreateRecord(context.Background(), "zone", Record{}); return e }},
		{"certificate", "cloudflare_certificate_permission_denied", func() error { _, e := c.Certificates(context.Background(), "zone"); return e }},
		{"settings", "cloudflare_ssl_permission_denied", func() error { _, e := c.Setting(context.Background(), "zone", "ssl"); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.run()
			if e == nil || e.Error() != tc.want || ErrorCode(e) != tc.want {
				t.Fatal(e)
			}
		})
	}
}
