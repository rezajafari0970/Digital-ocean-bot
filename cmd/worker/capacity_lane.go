package main

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
)

type capacityScheduleOptions struct {
	Slots                 int
	Poll, Retry, Deadline time.Duration
}

var capacityScheduleDefaults = capacityScheduleOptions{2, time.Second, time.Second, 20 * time.Second}

// Discover new panels as slots become free, rather than waiting for a complete
// fleet batch. The existing shared admission budget and durable client executor
// remain authoritative. Two assignments avoid queuing eight requests behind the
// two-slot bulk budget. New panels sort ahead of previously attempted panels.
func runCapacityFill(parent context.Context,
	admit func(context.Context, func(context.Context) error) error,
	discover func(context.Context) ([]readyworker.Panel, error),
	maintain func(context.Context, readyworker.Panel) error, o capacityScheduleOptions) {
	if o.Slots < 1 || o.Poll <= 0 || o.Retry <= 0 || o.Deadline <= 0 {
		panic("invalid capacity schedule")
	}
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	done := make(chan string, o.Slots)
	active := map[string]bool{}
	last := map[string]time.Time{}
	next := map[string]time.Time{}
	timer := time.NewTimer(0)
	defer timer.Stop()
	reset := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(o.Poll)
	}
	for {
		supervision.Pulse(ctx)
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return
		case id := <-done:
			delete(active, id)
			next[id] = time.Now().Add(o.Retry)
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		if len(active) < o.Slots {
			scan, stop := context.WithTimeout(ctx, 2*time.Second)
			panels, err := discover(scan)
			stop()
			if err == nil {
				eligible := make(map[string]bool, len(panels))
				for _, p := range panels {
					eligible[p.ID] = true
				}
				for id := range last {
					if !eligible[id] && !active[id] {
						delete(last, id)
						delete(next, id)
					}
				}
				sort.SliceStable(panels, func(i, j int) bool {
					a, b := last[panels[i].ID], last[panels[j].ID]
					if a.Equal(b) {
						return panels[i].ID < panels[j].ID
					}
					return a.Before(b)
				})
				for _, p := range panels {
					if len(active) >= o.Slots {
						break
					}
					if active[p.ID] || time.Now().Before(next[p.ID]) {
						continue
					}
					active[p.ID] = true
					last[p.ID] = time.Now()
					wg.Add(1)
					go func(p readyworker.Panel) {
						defer wg.Done()
						_ = admit(ctx, func(owned context.Context) error {
							work, stop := context.WithTimeout(owned, o.Deadline)
							defer stop()
							if err := work.Err(); err != nil {
								return err
							}
							return maintain(work, p)
						})
						// Buffer equals outstanding assignments so shutdown always joins.
						done <- p.ID
					}(p)
				}
			}
		}
		reset()
	}
}
