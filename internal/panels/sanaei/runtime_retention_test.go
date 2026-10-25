package sanaei

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuntimeFailedOpenReleasesIdentityState(t *testing.T) {
	m := &RuntimeManager{}
	for i := 0; i < 1000; i++ {
		if _, err := m.Acquire(context.Background(), fmt.Sprint("retired-", i)); err == nil {
			t.Fatal("missing factory unexpectedly opened")
		}
	}
	if m.Size() != 0 || len(m.mutationGates) != 0 || len(m.circuits) != 0 {
		t.Fatalf("failed opens retained identities: entries=%d gates=%d circuits=%d", m.Size(), len(m.mutationGates), len(m.circuits))
	}
}

func TestRuntimeAcquisitionSweepsExpiredIdentities(t *testing.T) {
	now := time.Now()
	m := &RuntimeManager{TTL: time.Minute, entries: map[string]*runtimeEntry{}, circuits: map[string]circuitState{}}
	for i := 0; i < 1000; i++ {
		id := fmt.Sprint("retired-", i)
		m.entries[id] = &runtimeEntry{runtime: &PanelRuntime{PanelID: id}, opened: now.Add(-time.Hour)}
		m.circuits[id] = circuitState{failures: 1, lastFailure: now.Add(-time.Hour)}
	}
	fresh := &PanelRuntime{PanelID: "fresh"}
	m.entries["fresh"] = &runtimeEntry{runtime: fresh, opened: now}
	m.entries["opening"] = &runtimeEntry{opening: true, opened: now.Add(-time.Hour), wait: make(chan struct{})}
	m.circuits["cooldown"] = circuitState{failures: 5, lastFailure: now, until: now.Add(time.Minute)}
	m.circuits["recent"] = circuitState{failures: 1, lastFailure: now}
	got, err := m.Acquire(context.Background(), "fresh")
	if err != nil || got != fresh || m.Size() != 2 || len(m.circuits) != 2 {
		t.Fatalf("sweep lost live state or retained expired state: %v entries=%d circuits=%d", err, m.Size(), len(m.circuits))
	}
	if _, err = m.Acquire(context.Background(), "cooldown"); !errors.Is(err, ErrRuntimeCircuitOpen) {
		t.Fatal("sweep bypassed cooldown", err)
	}
}

func TestRuntimeMutationIdentitySurvivesCacheReplacement(t *testing.T) {
	m := &RuntimeManager{TTL: time.Millisecond}
	old := &PanelRuntime{PanelID: "same", mutationManager: m}
	fresh := &PanelRuntime{PanelID: "same", mutationManager: m}
	other := &PanelRuntime{PanelID: "other", mutationManager: m}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var active atomic.Int32
	go func() {
		done <- old.WithMutation(context.Background(), func(context.Context) error {
			active.Add(1)
			defer active.Add(-1)
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first mutation did not enter")
	}
	m.Invalidate("same")
	m.PurgeExpired()
	if err := other.WithMutation(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal("independent panel blocked", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := fresh.WithMutation(ctx, func(context.Context) error { t.Error("same-panel replacement entered active mutation"); return nil })
	cancel()
	if !errors.Is(err, ErrRuntimeBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	m.mu.Lock()
	users := m.mutationGates["same"].users
	m.mu.Unlock()
	if users != 1 {
		t.Fatalf("cancelled waiter retained reference: %d", users)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		rt := old
		if i%2 == 0 {
			rt = fresh
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := rt.WithMutation(context.Background(), func(context.Context) error {
				if active.Add(1) != 1 {
					t.Error("concurrent same-panel mutations")
				}
				runtime.Gosched()
				active.Add(-1)
				return nil
			}); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if len(m.mutationGates) != 0 {
		t.Fatalf("idle gates retained: %d", len(m.mutationGates))
	}
}

func TestRuntimeMutationCancellationAndPanicReleaseGate(t *testing.T) {
	m := &RuntimeManager{}
	for i := 0; i < 1000; i++ {
		rt := &PanelRuntime{PanelID: fmt.Sprint("panel-", i), mutationManager: m}
		if err := rt.WithMutation(context.Background(), func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	rt := &PanelRuntime{PanelID: "cancelled", mutationManager: m}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rt.WithMutation(ctx, func(context.Context) error { t.Error("cancelled callback entered"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("fixture did not panic")
			}
		}()
		_ = rt.WithMutation(context.Background(), func(context.Context) error { panic("fixture") })
	}()
	if err := rt.WithMutation(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal("panic stranded gate", err)
	}
	if len(m.mutationGates) != 0 {
		t.Fatalf("churn retained %d idle identities", len(m.mutationGates))
	}
}
