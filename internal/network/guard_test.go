package network

import "testing"

func TestProxyRequiredFailsClosed(t *testing.T) {
	id := "proxy-1"
	p := Profile{Mode: RouteProxyRequired, ProxyID: &id}
	proxy := &Proxy{ID: id, Status: StatusDown}
	if err := ValidateRoute(p, proxy); err == nil {
		t.Fatal("expected fail-closed route rejection")
	}
}

func TestProxyRequiredAllowsHealthyProxy(t *testing.T) {
	id := "proxy-1"
	p := Profile{Mode: RouteProxyRequired, ProxyID: &id}
	proxy := &Proxy{ID: id, Status: StatusHealthy}
	if err := ValidateRoute(p, proxy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
