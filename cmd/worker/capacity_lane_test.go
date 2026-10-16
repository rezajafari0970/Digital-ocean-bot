package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
)

func TestCapacityDiscoversNewPanelBeforeSlowPeerFinishes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan struct{})
	var added atomic.Bool
	seen := make(chan string, 32)
	releaseFast := make(chan struct{})
	budget := worker.NewWorkBudget(2)
	go func() {
		defer close(ended)
		runCapacityFill(ctx, budget.Do, func(context.Context) ([]readyworker.Panel, error) {
			p := []readyworker.Panel{{ID: "a-slow"}, {ID: "b-fast"}}
			if added.Load() {
				p = append(p, readyworker.Panel{ID: "z-new"})
			}
			return p, nil
		}, func(c context.Context, p readyworker.Panel) error {
			select {
			case seen <- p.ID:
			default:
			}
			if p.ID == "a-slow" {
				<-c.Done()
				return c.Err()
			}
			if p.ID == "b-fast" {
				select {
				case <-releaseFast:
				case <-c.Done():
					return c.Err()
				}
			}
			return nil
		}, capacityScheduleOptions{2, 5 * time.Millisecond, 20 * time.Millisecond, 2 * time.Second})
	}()
	defer func() {
		cancel()
		select {
		case <-ended:
		case <-time.After(time.Second):
			t.Error("capacity shutdown did not join")
		}
	}()
	start := map[string]bool{}
	for len(start) < 2 {
		select {
		case id := <-seen:
			start[id] = true
		case <-time.After(time.Second):
			t.Fatal("initial panels not admitted")
		}
	}
	added.Store(true)
	close(releaseFast)
	select {
	case id := <-seen:
		if id != "z-new" {
			t.Fatalf("previously attempted panel %s ran ahead of new discovery", id)
		}
	case <-time.After(time.Second):
		t.Fatal("new panel waited for the slow fleet peer")
	}
	if budget.Snapshot()["active"] > 2 {
		t.Fatal("admission limit exceeded")
	}
}

func TestCapacityDeadlineBeginsAfterAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	gate, waiting := make(chan struct{}), make(chan struct{}, 1)
	workSeen := make(chan time.Duration, 1)
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		runCapacityFill(ctx, func(c context.Context, fn func(context.Context) error) error {
			select {
			case waiting <- struct{}{}:
			default:
			}
			select {
			case <-gate:
				return fn(c)
			case <-c.Done():
				return c.Err()
			}
		}, func(context.Context) ([]readyworker.Panel, error) { return []readyworker.Panel{{ID: "new"}}, nil },
			func(c context.Context, _ readyworker.Panel) error {
				deadline, ok := c.Deadline()
				if !ok {
					workSeen <- 0
				} else {
					workSeen <- time.Until(deadline)
				}
				<-c.Done()
				return c.Err()
			}, capacityScheduleOptions{1, 5 * time.Millisecond, time.Second, 500 * time.Millisecond})
	}()
	defer func() {
		cancel()
		select {
		case <-ended:
		case <-time.After(time.Second):
			t.Error("capacity shutdown did not join")
		}
	}()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("admission not reached")
	}
	time.Sleep(200 * time.Millisecond)
	close(gate)
	select {
	case remaining := <-workSeen:
		if remaining < 350*time.Millisecond {
			t.Fatalf("queue consumed operation deadline: %s", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("work not admitted")
	}
}
