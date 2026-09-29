package capacity

import "testing"

func TestAvailable(t *testing.T) {
	if got := (Snapshot{Limit: 10, InUse: 8, Pending: 1}).Available(); got != 1 {
		t.Fatalf("got=%d", got)
	}
	if got := (Snapshot{Limit: 1, InUse: 2}).Available(); got != 0 {
		t.Fatalf("got=%d", got)
	}
}
