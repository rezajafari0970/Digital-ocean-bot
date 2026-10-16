package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestProofAdmissionPrecedesDiscoveryAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := worker.NewWorkBudget(1)
		held := make(chan struct{})
		release := make(chan struct{})
		go budget.Do(context.Background(), func(context.Context) error { close(held); <-release; return nil })
		<-held
		var discovered, claimed, completed atomic.Int64
		l := proofLane{
			admit: budget.Do,
			discover: func(ctx context.Context, _ string) (readyworker.Panel, bool, error) {
				discovered.Add(1)
				return readyworker.Panel{ID: "one"}, true, ctx.Err()
			},
			reserve: func(context.Context, readyworker.Panel) (worker.OutcomeReservation, bool, error) {
				claimed.Add(1)
				return worker.OutcomeReservation{Version: time.Now()}, true, nil
			},
			reconcile: func(c context.Context, _ readyworker.Panel) error {
				d, ok := c.Deadline()
				if !ok || time.Until(d) != 45*time.Second {
					t.Error("deadline consumed before admission", time.Until(d))
				}
				return nil
			},
			complete: func(context.Context, readyworker.Panel, worker.OutcomeReservation, error) error {
				completed.Add(1)
				return nil
			},
		}
		done := make(chan error, 1)
		go func() { _, _, e := l.attempt(context.Background(), ""); done <- e }()
		time.Sleep(50 * time.Second)
		if discovered.Load() != 0 || claimed.Load() != 0 {
			t.Fatal("queued proof touched SQL")
		}
		close(release)
		if e := <-done; e != nil {
			t.Fatal(e)
		}
		if completed.Load() != 1 {
			t.Fatal("outcome not joined")
		}
	})
}
func TestProofCancellationRetainsPermitThroughCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := worker.NewWorkBudget(1)
		ctx, cancel := context.WithCancel(context.Background())
		entered, join := make(chan struct{}), make(chan struct{})
		receipt, release := make(chan struct{}), make(chan struct{})
		l := proofLane{admit: budget.Do,
			discover: func(context.Context, string) (readyworker.Panel, bool, error) {
				return readyworker.Panel{ID: "one"}, true, nil
			},
			reserve: func(context.Context, readyworker.Panel) (worker.OutcomeReservation, bool, error) {
				return worker.OutcomeReservation{Version: time.Now()}, true, nil
			},
			reconcile: func(c context.Context, _ readyworker.Panel) error { close(entered); <-c.Done(); <-join; return c.Err() },
			complete: func(c context.Context, _ readyworker.Panel, _ worker.OutcomeReservation, e error) error {
				if c.Err() != nil || e != nil {
					t.Error("shutdown falsely failed or canceled receipt", c.Err(), e)
				}
				close(receipt)
				<-release
				return nil
			},
		}
		done := make(chan struct{})
		go func() { defer close(done); l.attempt(ctx, "") }()
		<-entered
		cancel()
		synctest.Wait()
		if budget.Snapshot()["active"] != 1 {
			t.Fatal("refunded running permit")
		}
		close(join)
		<-receipt
		if budget.Snapshot()["active"] != 1 {
			t.Fatal("refunded before conditional receipt")
		}
		close(release)
		<-done
		if budget.Snapshot()["active"] != 0 {
			t.Fatal("permit leak")
		}
	})
}
func TestProofBudgetCapsAndRetirementIsolation(t *testing.T) {
	for _, n := range []string{"2", "8", "18"} {
		live, retired, limit := newProofWorkBudgets(worker.RolePanels, n)
		if fmt.Sprint(limit) != n || live.Snapshot()["limit"] != int64(limit) || retired.Snapshot()["limit"] != 1 {
			t.Fatal(n)
		}
	}
	for _, v := range []string{"0", "19", "oops", "-1"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("accepted invalid capacity %s", v)
				}
			}()
			newProofWorkBudgets(worker.RolePanels, v)
		}()
	}
	live, _, n := newProofWorkBudgets(worker.RoleAll, "18")
	if n != 2 || live.Snapshot()["limit"] != 2 {
		t.Fatal("unqualified combined capacity")
	}
}
func TestProofFleetCapacityEnvelope(t *testing.T) {
	for _, count := range []int{89, 90} {
		for _, duration := range []time.Duration{9 * time.Second, 10 * time.Second} {
			t.Run(fmt.Sprintf("%d_%s", count, duration), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					live, retired, _ := newProofWorkBudgets(worker.RolePanels, "18")
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					type item struct {
						due, last time.Time
						held      bool
						n         int
						max       time.Duration
					}
					items := make([]item, count)
					var mu sync.Mutex
					l := proofLane{admit: live.Do,
						discover: func(c context.Context, _ string) (readyworker.Panel, bool, error) {
							mu.Lock()
							defer mu.Unlock()
							index := -1
							for i, x := range items {
								if !x.held && !x.due.After(time.Now()) && (index < 0 || x.due.Before(items[index].due)) {
									index = i
								}
							}
							if index < 0 {
								return readyworker.Panel{}, false, c.Err()
							}
							return readyworker.Panel{ID: fmt.Sprint(index)}, true, nil
						},
						reserve: func(c context.Context, p readyworker.Panel) (worker.OutcomeReservation, bool, error) {
							var i int
							fmt.Sscan(p.ID, &i)
							mu.Lock()
							defer mu.Unlock()
							if items[i].held {
								return worker.OutcomeReservation{}, false, nil
							}
							items[i].held = true
							return worker.OutcomeReservation{Version: time.Now()}, true, c.Err()
						},
						reconcile: func(c context.Context, p readyworker.Panel) error {
							select {
							case <-time.After(duration):
							case <-c.Done():
								return c.Err()
							}
							var i int
							fmt.Sscan(p.ID, &i)
							mu.Lock()
							defer mu.Unlock()
							v := &items[i]
							now := time.Now()
							if !v.last.IsZero() {
								gap := now.Sub(v.last)
								if gap > v.max {
									v.max = gap
								}
							}
							v.n++
							v.last = now
							v.due = now.Add(20 * time.Second)
							return nil
						},
						complete: func(_ context.Context, p readyworker.Panel, _ worker.OutcomeReservation, _ error) error {
							var i int
							fmt.Sscan(p.ID, &i)
							mu.Lock()
							items[i].held = false
							mu.Unlock()
							return nil
						},
					}
					var wg sync.WaitGroup
					for range 18 {
						wg.Add(1)
						go func() { defer wg.Done(); l.run(ctx) }()
					}
					wg.Add(1)
					go func() {
						defer wg.Done()
						for ctx.Err() == nil {
							_ = retired.Do(ctx, func(c context.Context) error {
								select {
								case <-time.After(30 * time.Second):
									return nil
								case <-c.Done():
									return c.Err()
								}
							})
						}
					}()
					time.Sleep(180 * time.Second)
					cancel()
					wg.Wait()
					for i, v := range items {
						if v.n < 3 || v.max > 55*time.Second {
							t.Fatalf("panel %d n=%d max=%s", i, v.n, v.max)
						}
					}
					if live.Snapshot()["active"] != 0 || retired.Snapshot()["active"] != 0 {
						t.Fatal("unjoined workers")
					}
				})
			})
		}
	}
}
func TestProofClaimFailurePacesOutsideAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := worker.NewWorkBudget(1)
		l := proofLane{admit: budget.Do,
			discover: func(context.Context, string) (readyworker.Panel, bool, error) {
				return readyworker.Panel{ID: "bad"}, true, nil
			},
			reserve: func(context.Context, readyworker.Panel) (worker.OutcomeReservation, bool, error) {
				return worker.OutcomeReservation{}, false, errors.New("claim write failed")
			},
		}
		next, progress, e := l.attempt(context.Background(), "")
		if e == nil || progress || next != "bad" || budget.Snapshot()["active"] != 0 {
			t.Fatal(next, progress, e)
		}
	})
}
