package digitalocean

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
	"io"
	"net/http"
	"strings"
	"testing"
)

type secretStub struct{ token []byte }

func (s secretStub) Get(context.Context, string, string) ([]byte, error) {
	return append([]byte(nil), s.token...), nil
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientLoadsTokenAndDecodesAccount(t *testing.T) {
	h := rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("missing auth")
		}
		body := `{"account":{"uuid":"u","email":"e","status":"active","droplet_limit":10,"volume_limit":5,"reserved_ip_limit":2}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	c, err := NewClient(accounts.NewContext("a"), "token", secretStub{[]byte("token")}, &http.Client{Transport: h})
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL = "https://example.invalid/v2"
	a, err := c.GetAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.UUID != "u" || a.DropletLimit != 10 {
		t.Fatalf("bad account: %#v", a)
	}
}
