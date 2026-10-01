package app

import (
	"context"
	"errors"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var errFastObservationUnsupported = errors.New("fast provider observation unsupported")

func fastProviderObservation(ctx context.Context, driver providers.Driver, previous providers.Observation) (providers.Observation, error) {
	ar, ok := driver.(providers.AccountReader)
	if !ok {
		return providers.Observation{}, errFastObservationUnsupported
	}
	ir, ok := driver.(providers.InventoryReader)
	if !ok {
		return providers.Observation{}, errFastObservationUnsupported
	}
	account, err := ar.Account(ctx)
	if err != nil {
		return providers.Observation{}, err
	}
	inventory, err := ir.Inventory(ctx)
	if err != nil {
		return providers.Observation{}, err
	}
	now := time.Now().UTC()
	return providers.Observation{
		Account:    account,
		Capacity:   providers.Capacity{LimitKnown: false, ComputeInUse: len(inventory.Servers), ObservedAt: now},
		Catalog:    previous.Catalog,
		Inventory:  inventory,
		ObservedAt: now,
	}, nil
}
