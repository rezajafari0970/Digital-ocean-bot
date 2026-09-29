package vultr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAuthorizationAndHealth(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("auth=%q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"account":{"name":"x","email":"e@example.test","balance":1}}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "secret")
	c.base = s.URL
	d := &Driver{client: c}
	if err := d.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestGET429RetriesButMutationDoesNot(t *testing.T) {
	gets := 0
	posts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
			if gets < 2 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(429)
				_, _ = w.Write([]byte(`{"error":"slow"}`))
				return
			}
			_, _ = w.Write([]byte(`{"account":{"email":"e"}}`))
			return
		}
		posts++
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"slow"}`))
	}))
	defer s.Close()
	c := NewClient(s.Client(), "x")
	c.base = s.URL
	var a accountResponse
	if err := c.do(context.Background(), http.MethodGet, "/account", nil, &a); err != nil || gets != 2 {
		t.Fatalf("get err=%v count=%d", err, gets)
	}
	if err := c.do(context.Background(), http.MethodPost, "/instances", map[string]string{}, nil); err == nil || posts != 1 {
		t.Fatalf("post err=%v count=%d", err, posts)
	}
}
