package update

import (
	"context"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

var (
	ErrDependency  = errors.New("update dependency unavailable")
	ErrNotReady    = errors.New("panel not ready")
	ErrNotOwned    = errors.New("managed inbound not uniquely owned")
	ErrUnconfirmed = errors.New("inbound update unconfirmed")
)

type Request struct {
	ManagedKey string
	Port       int
}

type Result struct {
	Updated   bool
	Confirmed bool
	RemoteID  int64
}

type Dependencies interface {
	Ready(context.Context) (bool, error)
	RefreshInventory(context.Context) ([]inventory.InboundRecord, error)
	Build(context.Context, Request) (realityconfig.Payload, error)
	Update(context.Context, int64, realityconfig.Payload) error
	Verify(context.Context, int64, realityconfig.Payload) error
}

func Run(ctx context.Context, d Dependencies, r Request) (Result, error) {
	if d == nil || r.ManagedKey == "" || r.Port < 1 || r.Port > 65535 {
		return Result{}, ErrDependency
	}
	ready, err := d.Ready(ctx)
	if err != nil {
		return Result{}, err
	}
	if !ready {
		return Result{}, ErrNotReady
	}
	before, err := d.RefreshInventory(ctx)
	if err != nil {
		return Result{}, err
	}
	owned := ownedRecords(before, r.ManagedKey)
	if len(owned) != 1 || owned[0].RemoteID <= 0 {
		return Result{}, ErrNotOwned
	}
	old := owned[0]
	if old.Port != r.Port {
		return Result{}, ErrNotOwned
	}
	payload, err := d.Build(ctx, r)
	if err != nil {
		return Result{}, err
	}
	updateErr := d.Update(ctx, old.RemoteID, payload)
	after, invErr := d.RefreshInventory(ctx)
	if invErr != nil {
		if updateErr != nil {
			return Result{}, updateErr
		}
		return Result{}, ErrUnconfirmed
	}
	now := ownedRecords(after, r.ManagedKey)
	if len(now) == 1 && now[0].RemoteID == old.RemoteID && now[0].RawHash != "" && now[0].RawHash != old.RawHash {
		if err := d.Verify(ctx, old.RemoteID, payload); err != nil {
			return Result{}, err
		}
		return Result{Updated: true, Confirmed: true, RemoteID: old.RemoteID}, nil
	}
	if updateErr != nil {
		return Result{}, updateErr
	}
	return Result{}, ErrUnconfirmed
}

func ownedRecords(xs []inventory.InboundRecord, key string) []inventory.InboundRecord {
	var out []inventory.InboundRecord
	for _, x := range xs {
		if x.Remark == key {
			out = append(out, x)
		}
	}
	return out
}
