package worker

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"time"
)

type LifecycleHandler interface {
	ProcessLifecycle(context.Context, droplets.LifecycleItem) error
}
type LifecycleWorker struct {
	Store   droplets.LifecycleStore
	Handler LifecycleHandler
	Batch   int
}

func (w LifecycleWorker) Once(ctx context.Context) error {
	items, err := w.Store.Due(ctx, time.Now().UTC(), w.Batch)
	if err != nil {
		return err
	}
	for _, item := range items {
		_ = w.Handler.ProcessLifecycle(ctx, item)
	}
	return nil
}
