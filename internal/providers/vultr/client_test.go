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
