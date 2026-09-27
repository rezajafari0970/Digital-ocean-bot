package network

import (
	"errors"
	"testing"
)

func TestProbeGatewayAllowsUnhealthyStatusButTrafficDoesNot(t *testing.T) {
	for _, status := range []ProxyStatus{StatusDown, StatusDegraded} {
		p := Proxy{ID: "p", Type: ProxySOCKS5, Host: "127.0.0.1", Port: 1080, Status: status}
		if _, err := NewProxyGateway("a", p, ProxyCredentials{}); !errors.Is(err, ErrProxyConfigInvalid) {
			t.Fatalf("traffic status=%s err=%v", status, err)
		}
		g, err := NewProxyProbeGateway("a", p, ProxyCredentials{})
		if err != nil {
			t.Fatalf("probe status=%s err=%v", status, err)
		}
		g.CloseIdleConnections()
	}
}
func TestProbeGatewayStillValidatesEndpointAndType(t *testing.T) {
	cases := []Proxy{
		{Type: ProxySOCKS5, Port: 1080, Status: StatusDown},
		{Type: ProxySOCKS5, Host: "127.0.0.1", Port: 0, Status: StatusDown},
		{Type: ProxyType("bad"), Host: "127.0.0.1", Port: 1080, Status: StatusDown},
	}
	for _, p := range cases {
		if _, err := NewProxyProbeGateway("a", p, ProxyCredentials{}); err == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
}
