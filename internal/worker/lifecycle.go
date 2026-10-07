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
}

func (w LifecycleWorker) timeout() time.Duration {
	if w.ItemTimeout > 0 {
		return w.ItemTimeout
	}
	return 90 * time.Second
}
func (w LifecycleWorker) process(ctx context.Context, item droplets.LifecycleItem) error {
	itemCtx, cancel := context.WithTimeout(ctx, w.timeout())
	err := w.Handler.ProcessLifecycle(itemCtx, item)
	if err == nil {
		err = itemCtx.Err()
	}
	cancel()
	// The handler has returned before the lane can be released. Do not abandon
	// an uncooperative driver or permit a second call while it is still running.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err != nil {
		if ledgerErr := w.Failures.FailChecked(finishCtx, "lifecycle", item.ID, item.AccountID, err); ledgerErr != nil {
			return errors.Join(err, ledgerErr)
		}
		return err
	}
	return w.Failures.ClearChecked(finishCtx, "lifecycle", item.ID)
}
func (w LifecycleWorker) discover(ctx context.Context) ([]droplets.LifecycleItem, error) {
	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return w.Store.Due(scanCtx, time.Now().UTC(), w.Batch)
}
func (w LifecycleWorker) submitRound(ctx context.Context, d *Dispatcher, offset int) (int, error) {
	scanCtx, scanCancel := context.WithTimeout(ctx, 5*time.Second)
	defer scanCancel()
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
			if err := w.process(ctx, item); err != nil && ctx.Err() == nil {
				log.Printf("lifecycle item %s account=%s state=%s: %v", item.ID, item.AccountID, item.State, err)
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
	for {
		var err error
		offset, err = w.submitRound(ctx, d, offset)
		if err != nil && ctx.Err() == nil {
			log.Printf("lifecycle discovery: %v", err)
		}
		if err == nil && w.Progress != nil {
			w.Progress(time.Now())
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
	items, err := w.discover(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		due, err := w.Failures.DueChecked(ctx, "lifecycle", item.ID)
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		if err := w.process(ctx, item); err != nil && ctx.Err() == nil {
			log.Printf("lifecycle item %s account=%s state=%s: %v", item.ID, item.AccountID, item.State, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}
