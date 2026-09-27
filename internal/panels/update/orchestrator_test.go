package update

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"testing"
)

type fake struct {
	ready         bool
	before, after []inventory.InboundRecord
	reads         int
	updated       bool
	updateErr     error
}

func (f *fake) Ready(context.Context) (bool, error) { return f.ready, nil }
func (f *fake) RefreshInventory(context.Context) ([]inventory.InboundRecord, error) {
	f.reads++
	if f.reads == 1 {
		return f.before, nil
	}
	return f.after, nil
}
func (f *fake) Build(context.Context, Request) (realityconfig.Payload, error) {
	return realityconfig.Payload{}, nil
}
func (f *fake) Update(context.Context, int64, realityconfig.Payload) error {
	f.updated = true
	return f.updateErr
}

func TestConfirmedUpdate(t *testing.T) {
	f := &fake{ready: true, before: []inventory.InboundRecord{{RemoteID: 7, Remark: "k", Port: 443, RawHash: "a"}}, after: []inventory.InboundRecord{{RemoteID: 7, Remark: "k", Port: 443, RawHash: "b"}}}
	r, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if e != nil || !r.Updated || !r.Confirmed || r.RemoteID != 7 || !f.updated {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestRejectsDuplicateOwnership(t *testing.T) {
	f := &fake{ready: true, before: []inventory.InboundRecord{{RemoteID: 7, Remark: "k", Port: 443}, {RemoteID: 8, Remark: "k", Port: 443}}}
	_, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if !errors.Is(e, ErrNotOwned) || f.updated {
		t.Fatalf("err=%v updated=%t", e, f.updated)
	}
}
func TestRecoversAmbiguousUpdate(t *testing.T) {
	f := &fake{ready: true, updateErr: errors.New("timeout"), before: []inventory.InboundRecord{{RemoteID: 7, Remark: "k", Port: 443, RawHash: "a"}}, after: []inventory.InboundRecord{{RemoteID: 7, Remark: "k", Port: 443, RawHash: "b"}}}
	r, e := Run(context.Background(), f, Request{ManagedKey: "k", Port: 443})
	if e != nil || !r.Confirmed {
		t.Fatalf("%+v %v", r, e)
	}
}
