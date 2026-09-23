package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRotatingExitAllowed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ip":"198.51.100.77"}`))
	}))
	defer s.Close()
	g := &Gateway{AccountID: "a", Client: s.Client()}
	r := CheckProxy(context.Background(), g, s.URL, "", "203.0.113.9")
	if r.Status != StatusHealthy {
		t.Fatal(r.Error)
	}
}
func TestRotatingExitStillRejectsServerIP(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer s.Close()
	g := &Gateway{AccountID: "a", Client: s.Client()}
	r := CheckProxy(context.Background(), g, s.URL, "", "203.0.113.9")
	if r.Status == StatusHealthy {
		t.Fatal("server IP leak accepted")
	}
}
