package kamatera

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"testing"
)

const emptyWildcardResponse = `{"message":"No servers found (name='.*')"}`

func TestEmptyWildcardRequiresValidEmptyBasicInventory(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"confirmed empty", "[]", 200, true},
		{"contradictory list", `[{"id":"` + serverID + `","name":"existing","datacenter":"EU","power":"on"}]`, 200, false},
		{"null", "null", 200, false},
		{"trailing JSON", "[] []", 200, false},
		{"partial response", "[]", 206, false},
		{"created response", "[]", 201, false},
		{"malformed", "{", 200, false},
		{"authentication", "{}", 401, false},
		{"unavailable", "{}", 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post, get := 0, 0
			d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" && r.URL.Path == "/service/server/info" {
					post++
					x := reply(emptyWildcardResponse)
					x.StatusCode = 500
					return x, nil
				}
				if r.Method == "GET" && r.URL.Path == "/service/servers" {
					get++
					x := reply(tc.body)
					x.StatusCode = tc.status
					return x, nil
				}
				t.Fatal("unexpected request", r.Method, r.URL.Path)
				return nil, nil
			}, nil)
			inv, e := d.Inventory(context.Background())
			if (e == nil) != tc.ok || post != 1 || get < 1 || tc.ok && inv.Servers == nil {
				t.Fatal("invalid empty-inventory proof", e, post, get)
			}
			if e != nil && providers.IsClass(e, providers.ErrorNotFound) {
				t.Fatal("inventory error became deletion proof")
			}
		})
	}
}
func TestOtherServerInfoErrorsNeverBecomeEmptyInventory(t *testing.T) {
	for _, raw := range []string{`{"message":"Internal Server Error"}`, `{"message":"No servers found (name='other')"}`, `{"message":"No servers found (name='.*')","error":true}`} {
		get := 0
		d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
			if r.Method == "GET" {
				get++
			}
			x := reply(raw)
			x.StatusCode = 500
			return x, nil
		}, nil)
		if _, e := d.ListServers(context.Background()); e == nil || get != 0 {
			t.Fatal("unrelated error accepted", e, get)
		}
	}
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" {
			t.Fatal("unexpected confirmation read")
		}
		x := reply(emptyWildcardResponse)
		x.StatusCode = 500
		return x, nil
	}, nil)
	if _, e := d.GetServer(context.Background(), serverID); e == nil || providers.IsClass(e, providers.ErrorNotFound) || strings.Contains(e.Error(), "name=") {
		t.Fatal("exact-ID failure became absence", e)
	}
}

type unavailableCredential struct{ err error }

func (s unavailableCredential) Get(context.Context) ([]byte, error) {
	return nil, fmt.Errorf("private source detail: %w", s.err)
}
func TestCredentialDeadlineIsNotAuthenticationFailure(t *testing.T) {
	for _, want := range []error{context.DeadlineExceeded, context.Canceled} {
		_, e := credentials(context.Background(), unavailableCredential{want})
		if !errors.Is(e, want) || providers.IsClass(e, providers.ErrorAuthentication) || strings.Contains(e.Error(), "private source") {
			t.Fatal("deadline mislabeled or disclosed", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := credentials(ctx, credentialFixture(credentialJSON)); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled lookup succeeded", e)
	}
}
