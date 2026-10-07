package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
)

type LifecycleHandler interface {
	ProcessLifecycle(context.Context, droplets.LifecycleItem) error
}
type LifecycleSource interface {
	Due(context.Context, time.Time, int) ([]droplets.LifecycleItem, error)
}
type LifecycleWorker struct {
	Store       LifecycleSource
	Handler     LifecycleHandler
	Failures    FailureStore
	Batch       int
	Interval    time.Duration
	ItemTimeout time.Duration
	Concurrency int
	Progress    func(time.Time)
	Dispatcher  *Dispatcher
	Admit       func(context.Context, func(context.Context) error) error
}

func (w LifecycleWorker) timeout() time.Duration {
	if w.ItemTimeout > 0 {
		return w.ItemTimeout
	}
	return 90 * time.Second
}
func (w LifecycleWorker) process(ctx context.Context, item droplets.LifecycleItem) error {
	return runCheckpointedRecovery(ctx, w.Failures.DB, "lifecycle", RecoveryItem{ID: item.ID, AccountID: item.AccountID},
		func(ctx context.Context) (bool, error) { return w.Failures.DueChecked(ctx, "lifecycle", item.ID) },
		func(ctx context.Context) error {
			itemCtx, cancel := context.WithTimeout(ctx, w.timeout())
			defer cancel()
			err := w.Handler.ProcessLifecycle(itemCtx, item)
			if err == nil {
				err = itemCtx.Err()
			}
			return err
		}, w.Admit)
}
func (w LifecycleWorker) reconcile(ctx context.Context, active func(string) bool) error {
	if w.Failures.DB == nil || w.Store == nil || w.Handler == nil {
		return errors.New("lifecycle worker configuration required")
	}
	return reconcileRecoveryCheckpoints(ctx, w.Failures.DB, active, "lifecycle")
}
func (w LifecycleWorker) discover(ctx context.Context) ([]droplets.LifecycleItem, error) {
	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return w.Store.Due(scanCtx, time.Now().UTC(), w.Batch)
}
func (w LifecycleWorker) submitRound(ctx context.Context, d *Dispatcher, offset int, progress *recoveryProgress) (int, error) {
	scanCtx, scanCancel := context.WithTimeout(ctx, 5*time.Second)
	defer scanCancel()
	if err := w.reconcile(scanCtx, d.Active); err != nil {
		return offset, err
	}
	items, err := w.discover(scanCtx)
	if err != nil {
		return offset, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	start := offset % len(items)
	for i := 0; i < len(items); i++ {
		item := items[(start+i)%len(items)]
		checkCtx, cancel := context.WithTimeout(scanCtx, 3*time.Second)
		due, err := w.Failures.DueChecked(checkCtx, "lifecycle", item.ID)
		cancel()
		if err != nil {
			return offset, err
		}
		if !due {
			continue
		}
		d.Submit(ctx, "lifecycle:"+item.ID, item.AccountID, func() {
			if err := w.process(ctx, item); err != nil {
				progress.fail("lifecycle:" + item.ID)
				if ctx.Err() == nil {
					log.Printf("lifecycle item %s account=%s state=%s: %v", item.ID, item.AccountID, item.State, err)
				}
			}
		})
	}
	// Rotate the first offered account each round; persistently due older items
	// must not monopolize all free lanes inside the bounded discovery window.
	return (start + 1) % len(items), nil
}
func (w LifecycleWorker) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	concurrency := w.Concurrency
	if concurrency <= 0 {
		concurrency = 6
	}
	d := w.Dispatcher
	if d == nil {
		d = NewDispatcher(concurrency, 1)
	}
	defer d.Wait()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	offset := 0
	progress := recoveryProgress{publish: w.Progress}
	for {
		generation := progress.start(d.Active)
		var err error
		offset, err = w.submitRound(ctx, d, offset, &progress)
		if err != nil {
			progress.fail("")
			if ctx.Err() == nil {
				log.Printf("lifecycle discovery: %v", err)
			}
		} else {
			progress.success(generation)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Once is retained for synchronous bounded callers and deterministic tests.
func (w LifecycleWorker) Once(ctx context.Context) error {
	reconcileCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := w.reconcile(reconcileCtx, nil)
	cancel()
	if err != nil {
		return err
	}
	items, err := w.discover(ctx)
	if err != nil {
		return err
	}
	var outcomes error
	for _, item := range items {
		due, err := w.Failures.DueChecked(ctx, "lifecycle", item.ID)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		outcomes = errors.Join(outcomes, w.process(ctx, item))
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return outcomes
}
