package sanaei

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/lib/pq"
	"sync"
	"testing"
	"time"
)

func TestRuntimeMutationWaitHonorsDeadlineBeforeCallback(t *testing.T) {
	var lock sync.Mutex
	lock.Lock()
	rt := PanelRuntime{mutationMu: &lock}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	entered := false
	err := rt.WithMutation(ctx, func(context.Context) error { entered = true; return nil })
	lock.Unlock()
	if entered || !errors.Is(err, ErrRuntimeBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(entered, err)
	}
	if err := rt.WithMutation(context.Background(), func(context.Context) error { entered = true; return nil }); err != nil || !entered {
		t.Fatal(entered, err)
	}
}
func TestRuntimeAcquisitionWaitHasPreMutationProvenance(t *testing.T) {
	m := RuntimeManager{entries: map[string]*runtimeEntry{"panel": {opening: true, wait: make(chan struct{})}}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := m.Acquire(ctx, "panel"); !errors.Is(err, ErrRuntimeBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

type neverRuntimeSecret struct{}

func (neverRuntimeSecret) Get(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("secret unavailable")
}
func TestRuntimeFactorySQLFailuresNeverBecomePanelCircuit(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused/unused")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	m := RuntimeManager{Factory: RuntimeFactory{DB: db, Secrets: neverRuntimeSecret{}}}
	for i := 0; i < 8; i++ {
		_, err := m.Acquire(context.Background(), "fixture")
		if err == nil || errors.Is(err, ErrRuntimeCircuitOpen) {
			t.Fatal("SQL lost provenance", i, err)
		}
	}
	if m.circuits["fixture"].failures != 0 {
		t.Fatal("SQL incremented panel circuit")
	}
}
