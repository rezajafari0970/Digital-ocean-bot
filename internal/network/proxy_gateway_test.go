package network

import "testing"

func TestHTTPGatewayIsAccountScoped(t *testing.T) {
	p := Proxy{ID: "p1", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080, Status: StatusHealthy}
	a, err := NewProxyGateway("a", p, ProxyCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewProxyGateway("b", p, ProxyCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Client == b.Client || a.Transport == b.Transport {
		t.Fatal("gateway state shared")
	}
	if err := a.Validate("b"); err == nil {
		t.Fatal("cross-account gateway use must fail")
	}
}

func TestGatewayDirectDialFailsClosed(t *testing.T) {
	p := Proxy{ID: "p1", Type: ProxyHTTP, Host: "127.0.0.1", Port: 8080, Status: StatusHealthy}
	g, err := NewProxyGateway("a", p, ProxyCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.DirectDial(nil, "tcp", "example.com:443"); err == nil {
		t.Fatal("direct dial must fail")
	}
}
