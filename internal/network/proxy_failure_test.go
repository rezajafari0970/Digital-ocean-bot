package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGateBlocksDegradedAndDown(t *testing.T) {
	id := "p"
	p := &Proxy{ID: id, Status: StatusHealthy}
	for _, st := range []ProxyStatus{StatusDegraded, StatusDown, StatusUnknown} {
		g := AccountGate{Profile: Profile{AccountID: "a", Mode: RouteProxyRequired, ProxyID: &id}, Proxy: p, Health: HealthState{Status: st}}
		if g.AllowMutation() == nil {
			t.Fatalf("status %s must fail closed", st)
		}
	}
}
func TestRouteRejectsMissingOrUnhealthyProxy(t *testing.T) {
	id := "p"
	cases := []struct {
		name    string
		profile Profile
		proxy   *Proxy
	}{{"missing", Profile{Mode: RouteProxyRequired}, nil}, {"down", Profile{Mode: RouteProxyRequired, ProxyID: &id}, &Proxy{ID: id, Status: StatusDown}}, {"degraded", Profile{Mode: RouteProxyRequired, ProxyID: &id}, &Proxy{ID: id, Status: StatusDegraded}}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ValidateRoute(c.profile, c.proxy) == nil {
				t.Fatal("route must be rejected")
			}
		})
	}
}
func TestHealthRejectsServerIPLeak(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ip":"203.0.113.9"}`))
	}))
	defer srv.Close()
	g := &Gateway{AccountID: "a", Client: srv.Client()}
	res := CheckProxy(context.Background(), g, srv.URL, "203.0.113.9", "203.0.113.9")
	if res.Status == StatusHealthy {
		t.Fatal("server IP leak must never be healthy")
	}
}
func TestHighLatencyDegradesThenDown(t *testing.T) {
	p := HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: 100 * time.Millisecond}
	s := HealthState{Status: StatusHealthy}
	now := time.Now()
	r := HealthResult{Status: StatusHealthy, Latency: time.Second, CheckedAt: now}
	s = s.Apply(r, p)
	if s.Status != StatusDegraded {
		t.Fatalf("first slow result=%s", s.Status)
	}
	r.CheckedAt = now.Add(time.Second)
	s = s.Apply(r, p)
	if s.Status != StatusDown {
		t.Fatalf("second slow result=%s", s.Status)
	}
}
func TestRecoveryRequiresConsecutiveSuccesses(t *testing.T) {
	p := HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 3}
	s := HealthState{Status: StatusDown}
	for i := 0; i < 2; i++ {
		s = s.Apply(HealthResult{Status: StatusHealthy, Latency: time.Millisecond, CheckedAt: time.Now()}, p)
		if s.Status == StatusHealthy {
			t.Fatal("recovered too early")
		}
	}
	s = s.Apply(HealthResult{Status: StatusHealthy, Latency: time.Millisecond, CheckedAt: time.Now()}, p)
	if s.Status != StatusHealthy {
		t.Fatal("third success must recover")
	}
}
