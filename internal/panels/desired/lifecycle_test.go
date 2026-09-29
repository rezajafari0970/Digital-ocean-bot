package desired

import "testing"

func TestOperationalLifecycle(t *testing.T) {
	tests := map[string]bool{"READY": true, "EXPIRING": true, "RETIRING": true, "DELETING": false, "DELETED": false, "FAILED": false, "": false}
	for state, want := range tests {
		if got := operationalLifecycle(state); got != want {
			t.Fatalf("%s got=%v want=%v", state, got, want)
		}
	}
}
