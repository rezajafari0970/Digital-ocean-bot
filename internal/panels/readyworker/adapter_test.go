package readyworker

import (
	"context"
	"testing"
)

type sourceFake struct{}

func (sourceFake) EligibleReadyPanels(context.Context) ([]Panel, error) {
	return []Panel{{ID: "p1"}}, nil
}

type bootstrapFake struct{ called bool }

func (b *bootstrapFake) BootstrapPanel(context.Context, Panel) error { b.called = true; return nil }

type desiredFake struct {
	called bool
	dry    bool
}

func (d *desiredFake) ReconcilePanel(_ context.Context, _ Panel, dry bool) error {
	d.called = true
	d.dry = dry
	return nil
}

func TestProductionAdapterPropagatesDryRun(t *testing.T) {
	b := &bootstrapFake{}
	d := &desiredFake{}
	a := ProductionAdapter{Eligibility: sourceFake{}, Bootstrapper: b, Desired: d}
	rs, e := (Worker{Adapter: a, Concurrency: 1, DryRun: true}).Run(context.Background())
	if e != nil || len(rs) != 1 || !b.called || !d.called || !d.dry || !rs[0].Reconciled {
		t.Fatalf("%+v %v", rs, e)
	}
}
