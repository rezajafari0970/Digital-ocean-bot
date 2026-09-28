package readyworker

import (
	"context"
	"errors"
)

var ErrAdapterConfig = errors.New("ready panel adapter invalid config")

type EligibilitySource interface {
	EligibleReadyPanels(context.Context) ([]Panel, error)
}

type Bootstrapper interface {
	BootstrapPanel(context.Context, Panel) error
}

type DesiredReconciler interface {
	ReconcilePanel(context.Context, Panel, bool) error
}

type PostReconciler interface {
	ReconcilePanel(context.Context, Panel, bool) error
}

type ProductionAdapter struct {
	Eligibility  EligibilitySource
	Bootstrapper Bootstrapper
	Desired      DesiredReconciler
	Post         PostReconciler
}

func (a ProductionAdapter) Eligible(ctx context.Context) ([]Panel, error) {
	if a.Eligibility == nil {
		return nil, ErrAdapterConfig
	}
	return a.Eligibility.EligibleReadyPanels(ctx)
}

func (a ProductionAdapter) Bootstrap(ctx context.Context, panel Panel) error {
	if a.Bootstrapper == nil {
		return ErrAdapterConfig
	}
	return a.Bootstrapper.BootstrapPanel(ctx, panel)
}

func (a ProductionAdapter) Reconcile(ctx context.Context, panel Panel, dryRun bool) error {
	if a.Desired == nil {
		return ErrAdapterConfig
	}
	if err := a.Desired.ReconcilePanel(ctx, panel, dryRun); err != nil {
		return err
	}
	if a.Post != nil {
		return a.Post.ReconcilePanel(ctx, panel, dryRun)
	}
	return nil
}
