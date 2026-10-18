package main

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"sync/atomic"
	"time"
)

type proofLane struct {
	admit     func(context.Context, func(context.Context) error) error
	discover  func(context.Context, string) (readyworker.Panel, bool, error)
	reserve   func(context.Context, readyworker.Panel) (worker.OutcomeReservation, bool, error)
	reconcile func(context.Context, readyworker.Panel) error
	complete  func(context.Context, readyworker.Panel, worker.OutcomeReservation, error) error
	metrics   *proofLaneMetrics
}

// attempt retains admission through joined reconciliation, lease cleanup and
// conditional outcome persistence. Queue time never consumes the native deadline.
func (l proofLane) attempt(ctx context.Context, after string) (next string, progress bool, err error) {
	next = after
	queued := time.Now()
	err = l.admit(ctx, func(c context.Context) error {
		admitted := time.Now()
		if l.metrics != nil {
			l.metrics.wait.observe(admitted.Sub(queued))
			defer func() { l.metrics.attempt.observe(time.Since(admitted)) }()
		}
		discovery, finish := context.WithTimeout(c, 3*time.Second)
		p, found, e := l.discover(discovery, after)
		finish()
		if e != nil || !found {
			if !found {
				next = ""
			}
			return e
		}
		next = p.ID
		claimCtx, done := context.WithTimeout(c, 3*time.Second)
		lease, claimed, e := l.reserve(claimCtx, p)
		done()
		if e != nil || !claimed {
			return e
		}
		op, stop := context.WithTimeout(c, 45*time.Second)
		started := time.Now()
		result := l.reconcile(op, p)
		stop()
		if l.metrics != nil {
			l.metrics.reconcile.observe(time.Since(started))
			if errors.Is(result, residentialsync.ErrProofBusy) {
				l.metrics.deferred.Add(1)
			} else if result != nil {
				l.metrics.failed.Add(1)
			} else {
				l.metrics.completed.Add(1)
			}
		}
		// Shutdown is neutral scheduling cancellation; do not fabricate a panel fault.
		if c.Err() != nil {
			result = nil
		}
		receipt, closeReceipt := context.WithTimeout(context.WithoutCancel(c), 3*time.Second)
		persistErr := l.complete(receipt, p, lease, result)
		closeReceipt()
		if persistErr != nil {
			return errors.Join(result, persistErr)
		}
		// Return to oldest-due selection after every claimed attempt, including failures.
		next = ""
		progress = true
		if errors.Is(result, residentialsync.ErrProofBusy) {
			return nil
		}
		return result
	})
	return
}
func (l proofLane) run(ctx context.Context) {
	after := ""
	for ctx.Err() == nil {
		supervision.Pulse(ctx)
		next, progress, err := l.attempt(ctx, after)
		after = next
		if err != nil && ctx.Err() == nil {
			log.Printf("residential proof attempt: %v", err)
		}
		if ctx.Err() != nil {
			return
		}
		if progress && err == nil {
			continue
		}
		// Release the permit before pacing collisions, idle queues or SQL failures.
		supervision.Idle(ctx)
		delay := time.Second
		if err == nil && after != "" {
			delay = 25 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type proofDuration struct {
	buckets               [7]atomic.Int64
	count, totalMS, maxMS atomic.Int64
}

func (d *proofDuration) observe(v time.Duration) {
	ms := v.Milliseconds()
	d.count.Add(1)
	d.totalMS.Add(ms)
	for old := d.maxMS.Load(); ms > old; old = d.maxMS.Load() {
		if d.maxMS.CompareAndSwap(old, ms) {
			break
		}
	}
	bounds := [6]time.Duration{time.Millisecond * 100, time.Second, time.Second * 5, time.Second * 10, time.Second * 30, time.Second * 60}
	i := 0
	for i < len(bounds) && v > bounds[i] {
		i++
	}
	d.buckets[i].Add(1)
}
func (d *proofDuration) snapshot() map[string]any {
	buckets := make([]int64, len(d.buckets))
	for i := range buckets {
		buckets[i] = d.buckets[i].Load()
	}
	return map[string]any{"count": d.count.Load(), "total_ms": d.totalMS.Load(), "max_ms": d.maxMS.Load(), "bucket_bounds_ms": []int{100, 1000, 5000, 10000, 30000, 60000}, "buckets": buckets}
}

type proofLaneMetrics struct {
	wait, attempt, reconcile    proofDuration
	completed, failed, deferred atomic.Int64
}

func (m *proofLaneMetrics) snapshot() map[string]any {
	return map[string]any{"admission_wait": m.wait.snapshot(), "admitted_attempt": m.attempt.snapshot(), "reconciliation": m.reconcile.snapshot(), "completed": m.completed.Load(), "failed": m.failed.Load(), "deferred": m.deferred.Load()}
}
