package app

import "testing"

func TestDesiredServerHardCeiling(t *testing.T) {
	tests := []struct {
		name                        string
		desired, managed, preCreate int
		want                        bool
	}{
		{"empty account below desired", 5, 0, 0, true},
		{"one slot remains", 5, 4, 0, true},
		{"precreate consumes last slot", 5, 4, 1, false},
		{"managed equals desired", 5, 5, 0, false},
		{"managed exceeds desired", 5, 6, 0, false},
		{"multiple pending reach desired", 5, 2, 3, false},
		{"zero desired never creates", 0, 0, 0, false},
		{"negative inputs fail closed", 5, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desiredAllowsCreate(tt.desired, tt.managed, tt.preCreate); got != tt.want {
				t.Fatalf("desiredAllowsCreate(%d,%d,%d)=%v want %v", tt.desired, tt.managed, tt.preCreate, got, tt.want)
			}
		})
	}
}

func TestDesiredReservedCreateRecheck(t *testing.T) {
	if !desiredAllowsReservedCreate(5, 4, 1) {
		t.Fatal("reserved fifth create should remain allowed")
	}
	if desiredAllowsReservedCreate(4, 4, 1) {
		t.Fatal("lowering desired must cancel reserved fifth create before provider mutation")
	}
	if desiredAllowsReservedCreate(5, 5, 1) {
		t.Fatal("reserved create must fail closed when managed already reaches desired")
	}
}

func TestDesiredEffectiveManagedUsesProviderHighWatermark(t *testing.T) {
	if got := desiredEffectiveManaged(14, 15); got != 15 {
		t.Fatalf("got=%d want=15", got)
	}
	if got := desiredEffectiveManaged(15, 14); got != 15 {
		t.Fatalf("got=%d want=15", got)
	}
	if desiredAllowsCreate(15, desiredEffectiveManaged(14, 15), 0) {
		t.Fatal("must not backfill until provider-confirmed deletion frees the slot")
	}
}

func TestDesiredOccupancyCoversProviderIDBeforeDropletMaterialization(t *testing.T) {
	// Provider snapshot still sees 14. One create has already been accepted by
	// Vultr but its local droplet row is not materialized yet.
	occupancy := desiredEffectiveOccupancy(14, 1, 14)
	if occupancy != 15 {
		t.Fatalf("occupancy=%d want=15", occupancy)
	}
	if desiredOccupancyAllowsCreate(15, occupancy) {
		t.Fatal("second create must be blocked during provider-id/materialization gap")
	}
	if !desiredOccupancyAllowsReserved(15, occupancy) {
		t.Fatal("the already-reserved create itself must remain allowed")
	}
}

func TestDesiredOccupancyDoesNotDoubleCountProviderVisiblePending(t *testing.T) {
	// The provider may already report the accepted server while local materialization
	// is still pending. max(local committed, provider) avoids double counting it.
	if got := desiredEffectiveOccupancy(14, 1, 15); got != 15 {
		t.Fatalf("occupancy=%d want=15", got)
	}
}
