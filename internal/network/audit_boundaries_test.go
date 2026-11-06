package network

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAuditRouteRequiresAssignedProxyIdentity(t *testing.T) {
	for _, tc := range []struct{ assigned, actual string }{{"assigned", "other"}, {"", ""}, {" ", " "}, {"assigned", ""}} {
		t.Run(tc.assigned+"/"+tc.actual, func(t *testing.T) {
			profile := Profile{Mode: RouteProxyRequired, ProxyID: &tc.assigned}
			proxy := &Proxy{ID: tc.actual, Status: StatusHealthy}
			if ValidateRoute(profile, proxy) == nil {
				t.Fatal("unassigned proxy admitted")
			}
			gate := AccountGate{Profile: profile, Proxy: proxy, Health: HealthState{Status: StatusHealthy}}
			if gate.AllowMutation() == nil {
				t.Fatal("unassigned proxy admitted a mutation")
			}
		})
	}
}

type auditRoundTrip func(*http.Request) (*http.Response, error)

func (f auditRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type auditBody struct {
	io.Reader
	read   int
	closed bool
}

func (r *auditBody) Read(p []byte) (int, error) { n, e := r.Reader.Read(p); r.read += n; return n, e }
func (r *auditBody) Close() error               { r.closed = true; return nil }

func TestAuditProxyHealthBoundsAndCompleteJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		healthy    bool
	}{
		{"normal", `{"ip":"198.51.100.10"}`, true},
		{"oversize", `{"ip":"198.51.100.10","padding":"` + strings.Repeat("x", 2<<20) + `"}`, false},
		{"second_object", `{"ip":"198.51.100.10"}{"ip":"203.0.113.9"}`, false},
		{"trailing_garbage", `{"ip":"198.51.100.10"}not-json`, false},
		{"oversize_tail", `{"ip":"198.51.100.10"}` + strings.Repeat(" ", 2<<20), false},
		{"null", `null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &auditBody{Reader: strings.NewReader(tc.body)}
			client := &http.Client{Transport: auditRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
			})}
			got := CheckProxy(context.Background(), &Gateway{Client: client}, "https://fixture.invalid/ip", "198.51.100.10", "203.0.113.9")
			if (got.Status == StatusHealthy) != tc.healthy {
				t.Fatalf("unexpected health: %s", got.Status)
			}
			if !body.closed {
				t.Fatal("probe response was not closed")
			}
			if body.read > 16*1024+1 {
				t.Fatalf("unbounded health response: %d bytes", body.read)
			}
		})
	}
}

func TestAuditCancelledHealthProbeDoesNotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client := &http.Client{Transport: auditRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ip":"198.51.100.10"}`)), Header: make(http.Header)}, nil
	})}
	got := CheckProxy(ctx, &Gateway{Client: client}, "https://fixture.invalid/ip", "", "203.0.113.9")
	if calls != 0 || got.Status == StatusHealthy {
		t.Fatalf("cancelled probe dispatched=%d status=%s", calls, got.Status)
	}
}
