package readyworker

import (
	"context"
	"errors"
	"sync"
)

var ErrInvalidConfig = errors.New("ready panel worker invalid config")

type Panel struct{ ID string }

type PanelResult struct {
	PanelID      string
	Bootstrapped bool
	Reconciled   bool
	DryRun       bool
	Err          error
}

type Adapter interface {
	Eligible(context.Context) ([]Panel, error)
	Bootstrap(context.Context, Panel) error
	Reconcile(context.Context, Panel, bool) error
}

type Worker struct {
	Adapter     Adapter
	Concurrency int
	DryRun      bool
}

func (w Worker) Run(ctx context.Context) ([]PanelResult, error) {
	if w.Adapter == nil || w.Concurrency < 1 {
		return nil, ErrInvalidConfig
	}
	panels, err := w.Adapter.Eligible(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]PanelResult, len(panels))
	sem := make(chan struct{}, w.Concurrency)
	var wg sync.WaitGroup

	for i, panel := range panels {
		i, panel := i, panel
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := PanelResult{PanelID: panel.ID, DryRun: w.DryRun}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				result.Err = ctx.Err()
				results[i] = result
				return
			}
			if err := w.Adapter.Bootstrap(ctx, panel); err != nil {
				result.Err = err
				results[i] = result
				return
			}
			result.Bootstrapped = true
			if err := w.Adapter.Reconcile(ctx, panel, w.DryRun); err != nil {
				result.Err = err
				results[i] = result
				return
			}
			result.Reconciled = true
			results[i] = result
		}()
	}
	wg.Wait()
	return results, nil
}
