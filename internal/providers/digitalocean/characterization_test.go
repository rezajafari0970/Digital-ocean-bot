package digitalocean

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/accounts"
)

func fixtureClient(t *testing.T, handler func(*http.Request) (int, string)) *Client {
	t.Helper()
	h := rtFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization=%q", got)
		}
		code, body := handler(r)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	c, err := NewClient(accounts.NewContext("acct"), "token", secretStub{[]byte("token")}, &http.Client{Transport: h})
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL = "https://example.invalid/v2"
	return c
}

func TestGetLimitsMirrorsAccountLimits(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		if r.URL.Path != "/v2/account" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		return 200, `{"account":{"uuid":"u","status":"active","droplet_limit":17,"volume_limit":8,"reserved_ip_limit":3}}`
	})
	got, err := c.GetLimits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.DropletLimit != 17 || got.VolumeLimit != 8 || got.ReservedIPLimit != 3 {
		t.Fatalf("limits=%+v", got)
	}
}

func TestCatalogFetchesAccountRegionsSizesImages(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/v2/account":
			return 200, `{"account":{"uuid":"u","status":"active","droplet_limit":10}}`
		case "/v2/regions":
			return 200, `{"regions":[{"slug":"fra1","name":"Frankfurt","available":true}]}`
		case "/v2/sizes":
			return 200, `{"sizes":[{"slug":"s-1","memory":1024,"vcpus":1,"disk":25,"available":true,"regions":["fra1"]}]}`
		case "/v2/images":
			if r.URL.Query().Get("type") != "distribution" {
				t.Fatal("missing distribution filter")
			}
			return 200, `{"images":[{"id":24,"name":"24.04 (LTS) x64","distribution":"Ubuntu","slug":"ubuntu-24-04-x64","public":true,"status":"available"}]}`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return 500, ""
		}
	})
	got, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Account.DropletLimit != 10 || len(got.Regions) != 1 || len(got.Sizes) != 1 || len(got.Images) != 1 {
		t.Fatalf("catalog=%+v", got)
	}
	for _, p := range []string{"/v2/account", "/v2/regions", "/v2/sizes", "/v2/images"} {
		if seen[p] != 1 {
			t.Fatalf("%s calls=%d", p, seen[p])
		}
	}
}

func TestDiscoverIsFailFast(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		switch r.URL.Path {
		case "/v2/account":
			return 200, `{"account":{"uuid":"u","status":"active","droplet_limit":10}}`
		case "/v2/regions":
			return 200, `{"regions":[]}`
		case "/v2/sizes":
			return 200, `{"sizes":[]}`
		case "/v2/images":
			return 200, `{"images":[]}`
		case "/v2/droplets":
			return 200, `{"droplets":[]}`
		case "/v2/projects":
			return 500, `{"id":"server_error","message":"boom"}`
		default:
			t.Fatalf("request after discovery failure: %s", r.URL.Path)
			return 500, ""
		}
	})
	if _, err := c.Discover(context.Background()); err == nil {
		t.Fatal("expected fail-fast discovery error")
	}
}

func TestDropletMutationAndReadCharacterization(t *testing.T) {
	var deleteSeen bool
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/droplets":
			return 202, `{"droplet":{"id":42,"name":"n","status":"new","region":{"slug":"fra1"},"networks":{"v4":[]}}}`
		case r.Method == "GET" && r.URL.Path == "/v2/droplets/42":
			return 200, `{"droplet":{"id":42,"name":"n","status":"active","region":{"slug":"fra1"},"networks":{"v4":[{"ip_address":"203.0.113.7","type":"public"}]}}}`
		case r.Method == "DELETE" && r.URL.Path == "/v2/droplets/42":
			deleteSeen = true
			return 204, ""
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
			return 500, ""
		}
	})
	created, err := c.CreateDroplet(context.Background(), CreateDropletRequest{Name: "n", Region: "fra1", Size: "s-1", Image: "ubuntu"})
	if err != nil || created.ID != 42 {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	got, err := c.GetDroplet(context.Background(), 42)
	if err != nil || got.Status != "active" || got.PublicIPv4 != "203.0.113.7" {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if err := c.DeleteDroplet(context.Background(), 42); err != nil {
		t.Fatal(err)
	}
	if !deleteSeen {
		t.Fatal("delete not issued")
	}
}

func TestSSHKeyAndActionCharacterization(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/account/keys":
			return 201, `{"ssh_key":{"id":9,"name":"k","fingerprint":"fp","public_key":"ssh-ed25519 AAA"}}`
		case r.Method == "DELETE" && r.URL.Path == "/v2/account/keys/9":
			return 204, ""
		case r.Method == "GET" && r.URL.Path == "/v2/actions/77":
			return 200, `{"action":{"id":77,"status":"completed","type":"create","started_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:00:01Z"}}`
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
			return 500, ""
		}
	})
	k, err := c.CreateSSHKey(context.Background(), "k", "ssh-ed25519 AAA")
	if err != nil || k.ID != 9 {
		t.Fatalf("key=%+v err=%v", k, err)
	}
	if err := c.DeleteSSHKey(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	a, err := c.GetAction(context.Background(), 77)
	if err != nil || a.ID != 77 || a.Status != "completed" {
		t.Fatalf("action=%+v err=%v", a, err)
	}
}
