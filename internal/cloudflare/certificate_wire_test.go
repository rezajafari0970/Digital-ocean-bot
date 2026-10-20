package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestCloudflareCertificateWireDatesAndPagination(t *testing.T) {
	const expiry = "2026-10-27 13:52:00 +0000 UTC"
	writes, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes++
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"id": "issued", "certificate": "fixture-pem", "expires_on": expiry}})
			return
		}
		reads++
		if r.URL.Query().Get("per_page") != "50" || r.URL.Query().Get("zone_id") != "fixture-zone" {
			t.Error("missing scoped pagination")
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{map[string]any{"id": strconv.Itoa(page), "certificate": "fixture-pem", "expires_on": expiry}}, "result_info": map[string]int{"total_pages": 2}})
	}))
	defer server.Close()
	c := NewClient(Auth{Token: "fixture"})
	c.base = server.URL
	c.HTTP = server.Client()
	cert, e := c.Issue(context.Background(), "fixture-csr", []string{"*.example.com"})
	if e != nil || cert.ID != "issued" || cert.ExpiresOn != expiry || writes != 1 {
		t.Fatal("real wire date rejected or issue replayed", e, writes)
	}
	all, e := c.Certificates(context.Background(), "fixture-zone")
	if e != nil || len(all) != 2 || reads != 2 || all[0].CSR != "" || all[1].ID != "2" {
		t.Fatal("incomplete real certificate list", e, len(all), reads)
	}
}

func TestCloudflareIncompleteCertificateListCannotProveAbsence(t *testing.T) {
	for _, mode := range []string{"repeated-page", "failed-page"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "failed-page" && r.URL.Query().Get("page") == "2" {
					w.WriteHeader(503)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{map[string]any{"id": "repeated", "certificate": "fixture"}}, "result_info": map[string]int{"total_pages": 2}})
			}))
			defer server.Close()
			c := NewClient(Auth{Token: "fixture"})
			c.base = server.URL
			c.HTTP = server.Client()
			all, e := c.Certificates(context.Background(), "fixture-zone")
			if e == nil || all != nil {
				t.Fatal("partial list accepted as authoritative", e, all)
			}
		})
	}
}
