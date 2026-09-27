package create

import (
	"context"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

var (
	ErrNotReady     = errors.New("panel not ready")
	ErrPortOccupied = errors.New("inbound port occupied")
	ErrDependency   = errors.New("create dependency unavailable")
	ErrUnconfirmed  = errors.New("inbound create unconfirmed")
)

type Request struct {
	ManagedKey string
	Port       int
}
type Result struct {
	Created   bool
	Noop      bool
	Confirmed bool
}

type Dependencies interface {
	Ready(context.Context) (bool, error)
	RefreshInventory(context.Context) ([]inventory.InboundRecord, error)
	OccupiedPorts(context.Context) ([]int, error)
	Build(context.Context, Request) (realityconfig.Payload, error)
	Add(context.Context, realityconfig.Payload) error
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
	if contains(before, r.ManagedKey) {
		return Result{Noop: true, Confirmed: true}, nil
	}
	for _, x := range before {
		if x.Port == r.Port {
			return Result{}, ErrPortOccupied
		}
	}
	ports, err := d.OccupiedPorts(ctx)
	if err != nil {
		return Result{}, err
	}
	for _, p := range ports {
		if p == r.Port {
			return Result{}, ErrPortOccupied
		}
	}
	payload, err := d.Build(ctx, r)
	if err != nil {
		return Result{}, err
	}
	addErr := d.Add(ctx, payload)
	after, invErr := d.RefreshInventory(ctx)
	if invErr != nil {
		if addErr != nil {
			return Result{}, addErr
		}
		return Result{}, ErrUnconfirmed
	}
	if contains(after, r.ManagedKey) {
		return Result{Created: !contains(before, r.ManagedKey), Confirmed: true}, nil
	}
	if addErr != nil {
		return Result{}, addErr
	}
	return Result{}, ErrUnconfirmed
}

func contains(xs []inventory.InboundRecord, key string) bool {
	for _, x := range xs {
		if x.Remark == key {
			return true
		}
	}
	return false
}
