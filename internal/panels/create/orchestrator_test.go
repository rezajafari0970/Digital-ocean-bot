package create

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"testing"
)

type fake struct {
	ready  bool
	inv    [][]inventory.InboundRecord
	ports  []int
	addErr error
	adds   int
}

func (f *fake) Ready(context.Context) (bool, error) { return f.ready, nil }
func (f *fake) RefreshInventory(context.Context) ([]inventory.InboundRecord, error) {
	if len(f.inv) == 0 {
		return nil, nil
	}
	x := f.inv[0]
	if len(f.inv) > 1 {
		f.inv = f.inv[1:]
	}
	return x, nil
}
func (f *fake) OccupiedPorts(context.Context) ([]int, error) { return f.ports, nil }
func (f *fake) Build(context.Context, Request) (realityconfig.Payload, error) {
	return realityconfig.Payload{Remark: "built"}, nil
}
func (f *fake) Add(context.Context, realityconfig.Payload) error           { f.adds++; return f.addErr }
func (f *fake) Verify(context.Context, int64, realityconfig.Payload) error { return nil }
func TestNoopExisting(t *testing.T) {
	f := &fake{ready: true, inv: [][]inventory.InboundRecord{{{RemoteID: 7, Remark: "k"}}}}
	r, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if e != nil || !r.Noop || f.adds != 0 {
		t.Fatalf("%+v %v adds=%d", r, e, f.adds)
	}
}
func TestPortCollisionFailsClosed(t *testing.T) {
	f := &fake{ready: true, ports: []int{443}}
	_, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if !errors.Is(e, ErrPortOccupied) || f.adds != 0 {
		t.Fatalf("%v", e)
	}
}
func TestUnknownOutcomeRecoveredByPostflight(t *testing.T) {
	f := &fake{ready: true, inv: [][]inventory.InboundRecord{nil, {{RemoteID: 7, Remark: "k"}}}, addErr: errors.New("timeout")}
	r, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if e != nil || !r.Confirmed || !r.Created || f.adds != 1 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestSuccessfulButMissingIsUnconfirmed(t *testing.T) {
	f := &fake{ready: true, inv: [][]inventory.InboundRecord{nil, nil}}
	_, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if !errors.Is(e, ErrUnconfirmed) {
		t.Fatalf("%v", e)
	}
}
