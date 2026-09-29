package app

import (
	"testing"
	"time"
)

func TestCreateCapacityAvailable(t *testing.T) {
	x := CreateCapacity{LimitKnown: true, Limit: 10, ProviderDroplets: 8, PendingCreates: 1, SnapshotAt: time.Now()}
	if x.Available() != 1 {
		t.Fatalf("available=%d", x.Available())
	}
	x.PendingCreates = 4
	if x.Available() != 0 {
		t.Fatalf("negative capacity not clamped")
	}
}

func TestCreateCapacityUnknownLimitAllowsCreate(t *testing.T) {
	x := CreateCapacity{LimitKnown: false, ProviderDroplets: 500, PendingCreates: 2, SnapshotAt: time.Now()}
	if x.Available() <= 0 {
		t.Fatalf("unknown provider limit must not be treated as zero: %+v", x)
	}
}
