package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyHealthValidatesExitIP(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"198.51.100.10"}`))
	}))
	defer s.Close()
	g := &Gateway{AccountID: "a", Client: s.Client()}
	r := CheckProxy(context.Background(), g, s.URL, "198.51.100.10", "203.0.113.9")
	if r.Status != StatusHealthy {
		t.Fatalf("expected healthy: %s", r.Error)
	}
}

func TestProxyHealthRejectsUnexpectedExitIP(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ip":"198.51.100.11"}`)) }))
	defer s.Close()
	g := &Gateway{AccountID: "a", Client: s.Client()}
	r := CheckProxy(context.Background(), g, s.URL, "198.51.100.10", "203.0.113.9")
	if r.Status == StatusHealthy {
		t.Fatal("unexpected exit IP must fail")
	}
}
