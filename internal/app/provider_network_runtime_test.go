package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"

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

func TestErrRuntimeGenerationObsoleteRetryDelay(t *testing.T) {
	for _, err := range []error{
		ErrRuntimeGenerationObsolete,
		fmt.Errorf("mutation rejected: %w", ErrRuntimeGenerationObsolete),
	} {
		var hint interface{ RetryDelay() time.Duration }
		if !errors.As(err, &hint) {
			t.Fatalf("error %v does not expose a retry delay", err)
		}
		if delay := hint.RetryDelay(); delay != time.Second {
			t.Fatalf("retry delay=%v, want=%v", delay, time.Second)
		}
	}
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

type fakeAccountRuntimeStatusStore struct {
	status     string
	detail     string
	markCalls  int
	clearCalls int
	markErr    error
	clearErr   error
}

func (s *fakeAccountRuntimeStatusStore) MarkStaleGeneration(context.Context, string) error {
	s.markCalls++
	if s.markErr != nil {
		return s.markErr
	}
	if s.status == accountRuntimeStatusReady || s.status == accountRuntimeStatusStaleGeneration {
		s.status = accountRuntimeStatusStaleGeneration
		s.detail = accountRuntimeStatusStaleGenerationDetail
	}
	return nil
}

func (s *fakeAccountRuntimeStatusStore) ClearStaleGeneration(context.Context, string) error {
	s.clearCalls++
	if s.clearErr != nil {
		return s.clearErr
	}
	if s.status == accountRuntimeStatusStaleGeneration {
		s.status = accountRuntimeStatusReady
		s.detail = ""
	}
	return nil
}

func TestAccountRuntimeMutationGenerationStatus(t *testing.T) {
	ctx := context.Background()
	proxyID := "proxy-a"
	newRuntime := func(generation int64, status *fakeAccountRuntimeStatusStore) AccountRuntime {
		return AccountRuntime{
			Config: AccountConfig{
				ID:       "account-a",
				Provider: "provider-a",
				Network:  network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &proxyID},
			},
			TransportGeneration: 7,
			GenerationStore:     &fakeRuntimeGenerationStore{generation: generation, found: true},
			runtimeStatusStore:  status,
		}
	}

	status := &fakeAccountRuntimeStatusStore{status: accountRuntimeStatusReady}
	if err := newRuntime(8, status).CheckMutationGeneration(ctx); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("stale generation error=%v", err)
	}
	if status.status != accountRuntimeStatusStaleGeneration || status.detail != accountRuntimeStatusStaleGenerationDetail {
		t.Fatalf("stale status=%q detail=%q", status.status, status.detail)
	}

	if err := newRuntime(8, status).CheckMutationGeneration(ctx); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("repeated stale generation error=%v", err)
	}
	if status.status != accountRuntimeStatusStaleGeneration || status.detail != accountRuntimeStatusStaleGenerationDetail {
		t.Fatalf("repeated stale status=%q detail=%q", status.status, status.detail)
	}

	if err := newRuntime(7, status).CheckMutationGeneration(ctx); err != nil {
		t.Fatalf("fresh generation rejected: %v", err)
	}
	if status.status != accountRuntimeStatusReady || status.detail != "" {
		t.Fatalf("recovered status=%q detail=%q", status.status, status.detail)
	}

	providerStatus := &fakeAccountRuntimeStatusStore{status: "PROVIDER_ERROR", detail: "provider failure"}
	if err := newRuntime(8, providerStatus).CheckMutationGeneration(ctx); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("stale generation with provider error=%v", err)
	}
	if providerStatus.status != "PROVIDER_ERROR" || providerStatus.detail != "provider failure" {
		t.Fatalf("stale rejection overwrote provider state: status=%q detail=%q", providerStatus.status, providerStatus.detail)
	}
	if err := newRuntime(7, providerStatus).CheckMutationGeneration(ctx); err != nil {
		t.Fatalf("fresh generation with provider error rejected: %v", err)
	}
	if providerStatus.status != "PROVIDER_ERROR" || providerStatus.detail != "provider failure" {
		t.Fatalf("fresh generation cleared provider state: status=%q detail=%q", providerStatus.status, providerStatus.detail)
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

type concurrentRuntimeIsolationStore struct {
	mu          sync.Mutex
	generations map[string]int64
	statuses    map[string]string
}

func runtimeIsolationKey(accountID, proxyID, provider string) string {
	return accountID + "\x00" + proxyID + "\x00" + provider
}

func (s *concurrentRuntimeIsolationStore) setGeneration(accountID, proxyID, provider string, generation int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generations[runtimeIsolationKey(accountID, proxyID, provider)] = generation
}

func (s *concurrentRuntimeIsolationStore) CurrentGeneration(_ context.Context, accountID, proxyID, provider string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	generation, found := s.generations[runtimeIsolationKey(accountID, proxyID, provider)]
	return generation, found, nil
}

func (s *concurrentRuntimeIsolationStore) MarkStaleGeneration(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses[accountID] == accountRuntimeStatusReady || s.statuses[accountID] == accountRuntimeStatusStaleGeneration {
		s.statuses[accountID] = accountRuntimeStatusStaleGeneration
	}
	return nil
}

func (s *concurrentRuntimeIsolationStore) ClearStaleGeneration(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses[accountID] == accountRuntimeStatusStaleGeneration {
		s.statuses[accountID] = accountRuntimeStatusReady
	}
	return nil
}

func (s *concurrentRuntimeIsolationStore) status(accountID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statuses[accountID]
}

type concurrentObservationIsolationStore struct {
	mu          sync.Mutex
	generations map[string]int64
}

func (s *concurrentObservationIsolationStore) ApplyObservationForGeneration(
	_ context.Context,
	accountID, proxyID, provider string,
	generation int64,
	result network.HealthResult,
	_ proxycontrol.Policy,
) (proxycontrol.State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, found := s.generations[runtimeIsolationKey(accountID, proxyID, provider)]
	return proxycontrol.State{HealthState: result.Status}, found && current == generation, nil
}

func TestConcurrentProviderRuntimeGenerationsRemainAccountScoped(t *testing.T) {
	ctx := context.Background()
	store := &concurrentRuntimeIsolationStore{
		generations: map[string]int64{},
		statuses: map[string]string{
			"account-a": accountRuntimeStatusReady,
			"account-b": accountRuntimeStatusStaleGeneration,
		},
	}
	proxyA, proxyB := "proxy-a", "proxy-b"
	runtimeA := AccountRuntime{
		Config: AccountConfig{
			ID:       "account-a",
			Provider: "provider-a",
			Network:  network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &proxyA},
		},
		TransportGeneration: 7,
		GenerationStore:     store,
		runtimeStatusStore:  store,
	}
	runtimeB := AccountRuntime{
		Config: AccountConfig{
			ID:       "account-b",
			Provider: "provider-b",
			Network:  network.Profile{AccountID: "account-b", Mode: network.RouteProxyRequired, ProxyID: &proxyB},
		},
		TransportGeneration: 12,
		GenerationStore:     store,
		runtimeStatusStore:  store,
	}

	start := make(chan struct{})
	results := make(chan struct {
		accountID string
		err       error
	}, 2)
	go func() {
		<-start
		store.setGeneration("account-a", proxyA, "provider-a", 8)
		results <- struct {
			accountID string
			err       error
		}{"account-a", runtimeA.CheckMutationGeneration(ctx)}
	}()
	go func() {
		<-start
		store.setGeneration("account-b", proxyB, "provider-b", 12)
		results <- struct {
			accountID string
			err       error
		}{"account-b", runtimeB.CheckMutationGeneration(ctx)}
	}()
	close(start)

	for range 2 {
		result := <-results
		switch result.accountID {
		case "account-a":
			if !errors.Is(result.err, ErrRuntimeGenerationObsolete) {
				t.Fatalf("stale account error=%v, want ErrRuntimeGenerationObsolete", result.err)
			}
		case "account-b":
			if result.err != nil {
				t.Fatalf("fresh account rejected: %v", result.err)
			}
		default:
			t.Fatalf("unexpected account result %q", result.accountID)
		}
	}
	if status := store.status("account-a"); status != accountRuntimeStatusStaleGeneration {
		t.Fatalf("stale account status=%q, want=%q", status, accountRuntimeStatusStaleGeneration)
	}
	if status := store.status("account-b"); status != accountRuntimeStatusReady {
		t.Fatalf("fresh account status=%q, want=%q", status, accountRuntimeStatusReady)
	}

	observations := &concurrentObservationIsolationStore{generations: map[string]int64{
		runtimeIsolationKey("account-a", proxyA, "provider-a"): 8,
		runtimeIsolationKey("account-b", proxyB, "provider-b"): 12,
	}}
	healthyA, healthyB := true, true
	observationStart := make(chan struct{})
	observationResults := make(chan struct {
		accountID string
		err       error
	}, 2)
	go func() {
		<-observationStart
		err := persistProviderRuntimeObservation(
			ctx, observations, "account-a", proxyA, "provider-a", 7,
			network.HealthResult{Status: network.StatusDown}, proxycontrol.DefaultPolicy(), &healthyA,
		)
		observationResults <- struct {
			accountID string
			err       error
		}{"account-a", err}
	}()
	go func() {
		<-observationStart
		err := persistProviderRuntimeObservation(
			ctx, observations, "account-b", proxyB, "provider-b", 12,
			network.HealthResult{Status: network.StatusDown}, proxycontrol.DefaultPolicy(), &healthyB,
		)
		observationResults <- struct {
			accountID string
			err       error
		}{"account-b", err}
	}()
	close(observationStart)
	for range 2 {
		if result := <-observationResults; result.err != nil {
			t.Fatalf("account %s observation failed: %v", result.accountID, result.err)
		}
	}
	if !healthyA {
		t.Fatal("stale account observation changed runtime health")
	}
	if healthyB {
		t.Fatal("fresh account observation did not change runtime health")
	}
}

type scriptedProxyRoundTripper struct {
	steps []func(*http.Request) (*http.Response, error)
	next  int
}

func (s *scriptedProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if s.next >= len(s.steps) {
		return nil, fmt.Errorf("unexpected round trip %d", s.next)
	}
	step := s.steps[s.next]
	s.next++
	return step(req)
}

func proxyHTTPStatus(status int) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
}

func TestProxyControlPlaneFailureRecoveryE2E(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	policy := proxycontrol.Policy{
		Health:                  network.HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: time.Second},
		CircuitFailureThreshold: 2,
		OpenDuration:            10 * time.Second,
	}
	state := proxycontrol.DefaultState("account-a", "proxy-a", "provider-a")
	script := &scriptedProxyRoundTripper{steps: []func(*http.Request) (*http.Response, error){
		proxyHTTPStatus(http.StatusProxyAuthRequired),
		func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial proxy: injected transport failure")
		},
		proxyHTTPStatus(http.StatusOK),
		proxyHTTPStatus(http.StatusOK),
	}}
	clock := func() time.Time { return now }
	transport := network.HealthReportingTransport{
		Base: script,
		Now:  clock,
		Report: func(result network.HealthResult) {
			state = proxycontrol.ApplyHealth(state, result, policy)
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://provider.invalid/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil || resp == nil || resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("407 step resp=%v err=%v", resp, err)
	}
	_ = resp.Body.Close()
	if state.CircuitState != resilience.Closed || state.HealthState != network.StatusDegraded {
		t.Fatalf("after 407 circuit=%s health=%s", state.CircuitState, state.HealthState)
	}

	now = now.Add(time.Second)
	if _, err = transport.RoundTrip(req); err == nil {
		t.Fatal("transport failure step unexpectedly succeeded")
	}
	if state.CircuitState != resilience.Open || state.HealthState != network.StatusDown || state.RetryAfter == nil {
		t.Fatalf("after transport failure circuit=%s health=%s retry=%v", state.CircuitState, state.HealthState, state.RetryAfter)
	}
	if _, err = proxycontrol.Allow(state, now); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("open circuit admission err=%v", err)
	}

	now = state.RetryAfter.Add(time.Nanosecond)
	state, err = proxycontrol.Allow(state, now)
	if err != nil || state.CircuitState != resilience.HalfOpen {
		t.Fatalf("half-open admission circuit=%s err=%v", state.CircuitState, err)
	}
	resp, err = transport.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("first recovery probe resp=%v err=%v", resp, err)
	}
	_ = resp.Body.Close()
	if state.CircuitState != resilience.HalfOpen || state.HealthState == network.StatusHealthy {
		t.Fatalf("hysteresis bypassed circuit=%s health=%s", state.CircuitState, state.HealthState)
	}

	now = now.Add(time.Second)
	resp, err = transport.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("second recovery probe resp=%v err=%v", resp, err)
	}
	_ = resp.Body.Close()
	if state.CircuitState != resilience.Closed || state.HealthState != network.StatusHealthy {
		t.Fatalf("recovery failed circuit=%s health=%s", state.CircuitState, state.HealthState)
	}

	oldGeneration := state.Generation
	state = proxycontrol.BumpGeneration(state)
	generationStore := &fakeRuntimeGenerationStore{generation: state.Generation, found: true}
	proxyID := "proxy-a"
	runtimeFor := func(generation int64) AccountRuntime {
		return AccountRuntime{
			Config:              AccountConfig{ID: "account-a", Provider: "provider-a", Network: network.Profile{AccountID: "account-a", Mode: network.RouteProxyRequired, ProxyID: &proxyID}},
			TransportGeneration: generation,
			GenerationStore:     generationStore,
		}
	}
	if err = runtimeFor(oldGeneration).CheckMutationGeneration(ctx); !errors.Is(err, ErrRuntimeGenerationObsolete) {
		t.Fatalf("old runtime error=%v", err)
	}
	if err = runtimeFor(state.Generation).CheckMutationGeneration(ctx); err != nil {
		t.Fatalf("fresh runtime rejected: %v", err)
	}
}
