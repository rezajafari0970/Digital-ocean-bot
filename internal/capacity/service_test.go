package capacity

import "testing"

func TestAvailable(t *testing.T) {
	if got := (Snapshot{Limit: 10, LimitKnown: true, InUse: 8, Pending: 1}).Available(); got != 1 {
		t.Fatalf("got=%d", got)
	}
	if got := (Snapshot{Limit: 1, LimitKnown: true, InUse: 2}).Available(); got != 0 {
		t.Fatalf("got=%d", got)
	}
}

func TestAvailableUnknownLimit(t *testing.T) {
	if got := (Snapshot{LimitKnown: false, InUse: 3, Pending: 1}).Available(); got < 1000000 {
		t.Fatalf("got=%d", got)
	}
}
