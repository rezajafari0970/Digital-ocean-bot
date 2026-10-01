package app

import (
	"context"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type fastObserveDriver struct {
	account providers.Account
	inv     providers.Inventory
}

func (fastObserveDriver) Name() string { return "vultr" }
func (fastObserveDriver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Account: true, Inventory: true}
}
func (fastObserveDriver) Health(context.Context) error                         { return nil }
func (d fastObserveDriver) Account(context.Context) (providers.Account, error) { return d.account, nil }
func (fastObserveDriver) Capacity(context.Context) (providers.Capacity, error) {
	return providers.Capacity{}, nil
}
func (d fastObserveDriver) Inventory(context.Context) (providers.Inventory, error) { return d.inv, nil }

func TestFastProviderObservationPreservesCatalog(t *testing.T) {
	prev := providers.Observation{Catalog: providers.Catalog{Regions: []providers.Region{{ID: "ewr"}}, Plans: []providers.Plan{{ID: "vc2-1c-1gb"}}, Images: []providers.Image{{ID: "2284"}}}}
	d := fastObserveDriver{account: providers.Account{ID: "acct", Status: "active"}, inv: providers.Inventory{Servers: []providers.Server{{ID: "1"}, {ID: "2"}}}}
	got, err := fastProviderObservation(context.Background(), d, prev)
	if err != nil {
		t.Fatal(err)
	}
	if got.Capacity.ComputeInUse != 2 || got.Capacity.LimitKnown {
		t.Fatalf("capacity=%+v", got.Capacity)
	}
	if len(got.Catalog.Regions) != 1 || len(got.Catalog.Plans) != 1 || len(got.Catalog.Images) != 1 {
		t.Fatalf("catalog=%+v", got.Catalog)
	}
}
