package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"testing"
	"time"
)

func TestPanelMaintenanceCannotStarveGuardianLanes(t *testing.T) {
	parent, poll, maintenance := newPanelWorkBudgets(worker.RolePanels, 2)
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	maintStarted := make(chan struct{}, 8)
	pollStarted := make(chan struct{}, 2)
	var wg sync.WaitGroup
	defer func() { cancel(); close(release); wg.Wait() }()
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = maintenance(ctx, func(context.Context) error { maintStarted <- struct{}{}; <-release; return nil })
		}()
	}
	for range 2 {
		select {
		case <-maintStarted:
		case <-time.After(time.Second):
			t.Fatal("no maintenance progress")
		}
	}
	select {
	case <-maintStarted:
		t.Fatal("maintenance occupied guardian reservation")
	case <-time.After(50 * time.Millisecond):
	}
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = poll.Do(ctx, func(context.Context) error { pollStarted <- struct{}{}; <-release; return nil })
		}()
	}
	for range 2 {
		select {
		case <-pollStarted:
		case <-time.After(time.Second):
			t.Fatal("periodic proof starved behind maintenance")
		}
	}
	// Initial provisioning has its own one-slot production lane. Even with it
	// and both maintenance classes full, five parent positions remain available.
	bootstrap := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = parent.Do(ctx, func(context.Context) error { close(bootstrap); <-release; return nil })
	}()
	<-bootstrap
	guard := make(chan struct{}, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = parent.Do(ctx, func(context.Context) error { guard <- struct{}{}; <-release; return nil })
		}()
	}
	for range 5 {
		select {
		case <-guard:
		case <-time.After(time.Second):
			t.Fatal("guardian starved")
		}
	}
	if parent.Snapshot()["active"] != 10 {
		t.Fatal("shared parent limit/ownership incorrect", parent.Snapshot())
	}
}

func TestPanelAdmissionAccountsForProofReservations(t *testing.T) {
	for _, tt := range []struct{ proofs, slots int }{{2, 10}, {16, 9}, {18, 8}} {
		parent, _, _ := newPanelWorkBudgets(worker.RolePanels, tt.proofs)
		if got := int(parent.Snapshot()["limit"]); got != tt.slots {
			t.Fatalf("proofs=%d got=%d want=%d", tt.proofs, got, tt.slots)
		}
	}
}
