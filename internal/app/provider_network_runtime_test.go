package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

func TestProviderNetworkRuntimeDirectIsAccountScoped(t *testing.T) {
	c := Container{}
	rt, err := c.buildProviderNetworkRuntime(context.Background(), AccountConfig{
		ID:      "account-a",
		Network: network.Profile{AccountID: "account-a", Mode: network.RouteDirect},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Client == nil || rt.Gateway != nil {
		t.Fatalf("unexpected runtime: %+v", rt)
	}
	if rt.Generation != 0 {
		t.Fatalf("direct runtime must not claim a proxy generation, got=%d", rt.Generation)
	}
	if err := rt.Gate.AllowMutation(); err != nil {
		t.Fatalf("direct gate rejected: %v", err)
	}
}

func TestProviderNetworkRuntimeProxyRequiredUsesSharedGateway(t *testing.T) {
	c := Container{}
	p := &network.Proxy{
		ID: "proxy-a", Type: network.ProxyHTTP,
		Host: "127.0.0.1", Port: 8080, Status: network.StatusHealthy,
	}
	id := p.ID
	rt, err := c.buildProviderNetworkRuntime(context.Background(), AccountConfig{
		ID:      "account-a",
		Network: network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &id},
		Proxy:   p,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.CloseIdleConnections()
	if rt.Client == nil || rt.Gateway == nil || rt.Client != rt.Gateway.Client {
		t.Fatal("provider must receive the canonical account proxy client")
	}
	if rt.Generation < 1 {
		t.Fatalf("proxy runtime must carry a positive transport generation, got=%d", rt.Generation)
	}
	if err := rt.Gateway.Validate("account-a"); err != nil {
		t.Fatalf("gateway account binding failed: %v", err)
	}
	if err := rt.Gate.AllowMutation(); err != nil {
		t.Fatalf("healthy proxy gate rejected: %v", err)
	}
}

func TestProviderNetworkRuntimeProxyRequiredFailsClosed(t *testing.T) {
	c := Container{}
	id := "proxy-a"
	if _, err := c.buildProviderNetworkRuntime(context.Background(), AccountConfig{
		ID:      "account-a",
		Network: network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &id},
	}); !errors.Is(err, ErrNetworkNotReady) {
		t.Fatalf("missing proxy error=%v", err)
	}

	p := &network.Proxy{
		ID: id, Type: network.ProxyHTTP,
		Host: "127.0.0.1", Port: 8080, Status: network.StatusDown,
	}
	if _, err := c.buildProviderNetworkRuntime(context.Background(), AccountConfig{
		ID:      "account-a",
		Network: network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &id},
		Proxy:   p,
	}); !errors.Is(err, network.ErrProxyConfigInvalid) {
		t.Fatalf("down proxy must fail closed, err=%v", err)
	}
}
