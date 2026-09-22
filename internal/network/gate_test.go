package network

import "testing"

func TestAccountGateRequiresHealthyProxy(t *testing.T) {
	id := "p1"
	p := &Proxy{ID: id, Status: StatusHealthy}
	g := AccountGate{Profile: Profile{AccountID: "a", Mode: RouteProxyRequired, ProxyID: &id}, Proxy: p, Health: HealthState{Status: StatusDown}}
	if err := g.AllowMutation(); err == nil {
		t.Fatal("mutation must be blocked when proxy health is down")
	}
	g.Health.Status = StatusHealthy
	if err := g.AllowMutation(); err != nil {
		t.Fatal(err)
	}
}
