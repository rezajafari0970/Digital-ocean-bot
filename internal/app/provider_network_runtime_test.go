package app

import (
	"context"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
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

type fakeRuntimeGenerationStore struct {
	generation int64
	found      bool
	err        error
	calls      int
}

func (s *fakeRuntimeGenerationStore) CurrentGeneration(context.Context, string, string, string) (int64, bool, error) {
	s.calls++
	return s.generation, s.found, s.err
}

func TestAccountRuntimeCheckMutationGeneration(t *testing.T) {
	ctx := context.Background()
	direct := AccountRuntime{
		Config: AccountConfig{
			ID:      "account-a",
			Network: network.Profile{AccountID: "account-a", Mode: network.RouteDirect},
		},
	}
	if err := direct.CheckMutationGeneration(ctx); err != nil {
		t.Fatalf("direct runtime rejected: %v", err)
	}

	proxyID := "proxy-a"
	newRuntime := func(store runtimeGenerationStore) AccountRuntime {
		return AccountRuntime{
			Config: AccountConfig{
				ID:       "account-a",
				Provider: "provider-a",
				Network:  network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &proxyID},
			},
			TransportGeneration: 7,
			GenerationStore:     store,
		}
	}

	matching := &fakeRuntimeGenerationStore{generation: 7, found: true}
	if err := newRuntime(matching).CheckMutationGeneration(ctx); err != nil {
		t.Fatalf("matching generation rejected: %v", err)
	}
	if matching.calls != 1 {
		t.Fatalf("generation lookups=%d, want=1", matching.calls)
	}

	for name, store := range map[string]*fakeRuntimeGenerationStore{
		"mismatch": {generation: 8, found: true},
		"missing":  {},
		"error":    {err: errors.New("store unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			err := newRuntime(store).CheckMutationGeneration(ctx)
			if !errors.Is(err, ErrRuntimeGenerationObsolete) {
				t.Fatalf("error=%v, want ErrRuntimeGenerationObsolete", err)
			}
			if store.calls != 1 {
				t.Fatalf("generation lookups=%d, want=1", store.calls)
			}
		})
	}

	missingGeneration := newRuntime(matching)
	missingGeneration.TransportGeneration = 0
	if err := missingGeneration.CheckMutationGeneration(ctx); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("proxy-required generation zero error=%v", err)
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

type fakeProviderObservationStore struct {
	generation int64
	next       proxycontrol.State
	applied    bool
	err        error
}

func (s *fakeProviderObservationStore) ApplyObservationForGeneration(
	_ context.Context,
	_, _, _ string,
	generation int64,
	_ network.HealthResult,
	_ proxycontrol.Policy,
) (proxycontrol.State, bool, error) {
	s.generation = generation
	return s.next, s.applied, s.err
}

func TestPersistProviderRuntimeObservationUsesCapturedGenerationAndIgnoresStaleResult(t *testing.T) {
	store := &fakeProviderObservationStore{
		next:    proxycontrol.State{HealthState: network.StatusDown},
		applied: false,
	}
	healthy := true

	if err := persistProviderRuntimeObservation(
		context.Background(), store, "account-a", "proxy-a", "provider-a", 7,
		network.HealthResult{Status: network.StatusDown}, proxycontrol.DefaultPolicy(), &healthy,
	); err != nil {
		t.Fatal(err)
	}
	if store.generation != 7 {
		t.Fatalf("observation generation=%d, want=7", store.generation)
	}
	if !healthy {
		t.Fatal("stale observation changed runtime health")
	}

	store.applied = true
	if err := persistProviderRuntimeObservation(
		context.Background(), store, "account-a", "proxy-a", "provider-a", 7,
		network.HealthResult{Status: network.StatusDown}, proxycontrol.DefaultPolicy(), &healthy,
	); err != nil {
		t.Fatal(err)
	}
	if healthy {
		t.Fatal("matching-generation observation did not update runtime health")
	}
}
