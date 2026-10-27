package kamatera

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

type failingPriceBody struct{}

func (failingPriceBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingPriceBody) Close() error             { return nil }
func TestPublicPriceRetriesOnlyTransientFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		kind   string
		calls  int
		ok     bool
	}{
		{"network retry", 0, "transport", 2, true}, {"read retry", 200, "body", 2, true}, {"server retry", 503, "", 2, true},
		{"bounded server failure", 500, "always", 2, false}, {"redirect", 302, "", 1, false}, {"authorization", 403, "", 1, false},
		{"rate limit", 429, "", 1, false}, {"malformed price", 200, "invalid", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != pricingURL || r.Header.Get("AuthSecret") != "" || r.Header.Get("AuthClientId") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatal("unsafe retry route")
				}
				if calls > 1 && tc.kind != "always" {
					return reply(tariffFixture), nil
				}
				if tc.kind == "transport" {
					return nil, io.ErrUnexpectedEOF
				}
				x := reply("invalid")
				x.StatusCode = tc.status
				if tc.kind == "body" {
					x.Body = failingPriceBody{}
				}
				return x, nil
			}, nil)
			p, e := d.publicTariff(context.Background())
			if (e == nil) != tc.ok || calls != tc.calls || tc.ok && (p == nil || p.observed.IsZero()) {
				t.Fatal("unexpected retry outcome", e, calls)
			}
		})
	}
}
func TestPriceRetryKeepsParentDeadlineReserve(t *testing.T) {
	calls := 0
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		calls++
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Fatal("parent reserve not preserved")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2050*time.Millisecond)
	defer cancel()
	if _, e := d.publicTariff(ctx); e == nil || calls != 1 {
		t.Fatal("expired budget retried", e, calls)
	}
}
func TestPriceRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	d := fixtureDriver(t, func(*http.Request) (*http.Response, error) { calls++; cancel(); return nil, io.ErrUnexpectedEOF }, nil)
	if _, e := d.publicTariff(ctx); e == nil || calls != 1 {
		t.Fatal("cancellation retried", e, calls)
	}
}
