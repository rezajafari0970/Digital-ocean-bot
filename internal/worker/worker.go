package worker

import (
	"context"
	"time"
)

type Handler interface {
	RecoverOperation(context.Context, RecoveryItem) error
	RecoverDeployment(context.Context, RecoveryItem) error
}
type Worker struct {
	Store     RecoveryStore
	Handler   Handler
	Lifecycle *LifecycleWorker
	Interval  time.Duration
	Batch     int
}

func (w Worker) Run(ctx context.Context) error {
	interval := w.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.Once(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (w Worker) Once(ctx context.Context) error {
	ops, err := w.Store.Operations(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range ops {
		if err := w.Handler.RecoverOperation(ctx, x); err != nil {
			continue
		}
	}
	if w.Lifecycle != nil {
		_ = w.Lifecycle.Once(ctx)
	}
	deployments, err := w.Store.Deployments(ctx, w.Batch)
	if err != nil {
		return err
	}
	for _, x := range deployments {
		if err := w.Handler.RecoverDeployment(ctx, x); err != nil {
			continue
		}
	}
	return nil
}
