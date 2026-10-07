package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"log"
	"sync"
	"time"
)

type Handler interface {
	RecoverOperation(context.Context, RecoveryItem) error
	RecoverDeployment(context.Context, RecoveryItem) error
}
type BackoffBypasser interface {
	BypassDeploymentBackoff(context.Context, RecoveryItem) bool
}
type Worker struct {
	checkpointCursor *pendingRecovery
	Store            RecoveryStore
	Handler          Handler
	Lifecycle        *LifecycleWorker
	Failures         FailureStore
	Interval         time.Duration
	Batch            int
	Progress         func(time.Time)
	// Admission covers checkpoint session, native handler and durable completion.
	Admit func(context.Context, func(context.Context) error) error
}

// A scan that began before an async persistence failure cannot publish health
// over that failure. A later successful reconciliation+scan restores progress.
type recoveryProgress struct {
	mu         sync.Mutex
	generation uint64
	pending    map[string]bool
	publish    func(time.Time)
}

func (p *recoveryProgress) start(active func(string) bool) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.pending {
		if active != nil && !active(key) {
			delete(p.pending, key)
		}
	}
	return p.generation
}
func (p *recoveryProgress) fail(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.generation++
	if key != "" {
		if p.pending == nil {
			p.pending = make(map[string]bool)
		}
		p.pending[key] = true
	}
	if p.publish != nil {
		p.publish(time.Unix(0, 0))
	}
}
func (p *recoveryProgress) success(g uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.generation == g && len(p.pending) == 0 && p.publish != nil {
		p.publish(time.Now())
	}
}

func (w Worker) Run(ctx context.Context) error {
	w.checkpointCursor = &pendingRecovery{}
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	dispatcher := NewDispatcher(6, 2)
	defer dispatcher.Wait()
	progress := recoveryProgress{publish: w.Progress}
	deferredCursor := ""
	for {
		supervision.Pulse(ctx)
		generation := progress.start(dispatcher.Active)
		err := w.round(ctx, dispatcher.Active, func(key, account string, fn func() error) {
			dispatcher.Submit(ctx, key, account, func() {
				if err := fn(); err != nil {
					progress.fail(key)
					if ctx.Err() == nil {
						log.Printf("recovery completion pending kind/item=%s: %v", key, err)
					}
				}
			})
		}, &deferredCursor)
		if err != nil {
			progress.fail("")
			if ctx.Err() == nil {
				log.Printf("recovery discovery: %v", err)
			}
		} else {
			progress.success(generation)
		}
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w Worker) Once(ctx context.Context) error {
	var outcomes error
	err := w.round(ctx, nil, func(_, _ string, fn func() error) { outcomes = errors.Join(outcomes, fn()) })
	return errors.Join(err, outcomes)
}
func (w Worker) process(ctx context.Context, kind string, x RecoveryItem) error {
	return runCheckpointedRecovery(ctx, w.Store.DB, kind, x, func(ctx context.Context) (bool, error) { return w.recoveryDue(ctx, kind, x) }, func(ctx context.Context) error {
		if kind == "operation" {
			return w.Handler.RecoverOperation(ctx, x)
		}
		return w.Handler.RecoverDeployment(ctx, x)
	}, w.Admit)
}
func (w Worker) round(ctx context.Context, active func(string) bool, submit func(string, string, func() error), cursor ...*string) error {
	if w.Store.DB == nil || w.Failures.DB == nil || w.Handler == nil {
		return errors.New("recovery worker configuration required")
	}
	scanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if w.checkpointCursor == nil {
		w.checkpointCursor = &pendingRecovery{}
	}
	reconcileErr := reconcileRecoveryCheckpoints(scanCtx, w.Store.DB, active, "recovery", w.checkpointCursor)
	if reconcileErr != nil && !errors.Is(reconcileErr, ErrRecoveryCheckpointPending) {
		return reconcileErr
	}
	if err := w.Failures.ClearResolvedChecked(scanCtx); err != nil {
		// Pruning terminal diagnostics is not admission. Every candidate still
		// performs its checked due read and durable checkpoint before native work.
		reconcileErr = errors.Join(reconcileErr, fmt.Errorf("resolved recovery pruning: %w", err))
	}
	ops, err := w.Store.Operations(scanCtx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range ops {
		x := x
		due, err := w.recoveryDue(scanCtx, "operation", x)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		submit("operation:"+x.ID, x.AccountID, func() error { return w.process(ctx, "operation", x) })
	}
	if w.Lifecycle != nil {
		if err := w.Lifecycle.Once(ctx); err != nil {
			return err
		}
	}
	offeredDeployments := make(map[string]bool)
	deployments, err := w.Store.Deployments(scanCtx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range deployments {
		x := x
		due, err := w.recoveryDue(scanCtx, "deployment", x)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		offeredDeployments[x.ID] = true
		submit("deployment:"+x.ID, x.AccountID, func() error { return w.process(ctx, "deployment", x) })
	}
	// Normally due work has its own window. Deferred candidates use bounded
	// keyset continuation so a blocked prefix cannot hide a native bypass.
	after := ""
	if len(cursor) > 0 {
		after = *cursor[0]
	}
	offered := 0
	for page := 0; page < 8; page++ {
		deferred, err := w.Store.DeferredDeploymentPage(scanCtx, 100, after)
		if err != nil {
			return errors.Join(reconcileErr, err)
		}
		if len(deferred) == 0 {
			if len(cursor) > 0 {
				*cursor[0] = ""
			}
			break
		}
		for _, x := range deferred {
			x := x
			if offeredDeployments[x.ID] {
				after = x.ID
				if len(cursor) > 0 {
					*cursor[0] = after
				}
				continue
			}
			due, err := w.recoveryDue(scanCtx, "deployment", x)
			if err != nil {
				return errors.Join(reconcileErr, err)
			}
			after = x.ID
			if len(cursor) > 0 {
				*cursor[0] = after
			}
			if due {
				submit("deployment:"+x.ID, x.AccountID, func() error { return w.process(ctx, "deployment", x) })
				offered++
			}
			if offered >= 100 {
				return reconcileErr
			}
		}
		if len(deferred) < 100 {
			if len(cursor) > 0 {
				*cursor[0] = ""
			}
			break
		}
	}
	return reconcileErr
}

// Interrupted completion is never an installer backoff-bypass authorization.
func (w Worker) recoveryDue(ctx context.Context, kind string, x RecoveryItem) (bool, error) {
	due, err := w.Failures.DueChecked(ctx, kind, x.ID)
	if err != nil || due || kind != "deployment" {
		return due, err
	}
	var reason string
	if err = w.Failures.DB.QueryRowContext(ctx, "SELECT last_error FROM worker_item_failures WHERE kind=$1 AND item_id=$2", kind, x.ID).Scan(&reason); err != nil {
		return false, err
	}
	if reason == (recoveryInterrupted{}).Error() {
		return false, nil
	}
	b, ok := w.Handler.(BackoffBypasser)
	return ok && b.BypassDeploymentBackoff(ctx, x), nil
}
