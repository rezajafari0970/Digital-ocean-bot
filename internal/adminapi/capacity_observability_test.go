package adminapi

import (
	"testing"
	"time"
)

func TestCapacityEvidenceStates(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name, source, want string
		lower              int
		inFlight           bool
		after              *time.Time
	}{
		{"no evidence", "", "UNKNOWN", 0, false, nil},
		{"inventory lower bound", "vultr_api_lower_bound", "PROVEN_LOWER_BOUND", 6, false, nil},
		{"probe success is not exact", "vultr_api_probe_success", "PROVEN_LOWER_BOUND", 11, false, &now},
		{"saturation proves exact ceiling", "vultr_api_saturation", "EXACT", 10, false, &now},
		{"legacy console evidence is no longer authoritative", "vultr_console", "PROVEN_LOWER_BOUND", 10, false, nil},
		{"claimed probe", "vultr_api_probe", "PROBING", 10, true, &now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveCapacityEvidence(tt.source, tt.lower, tt.inFlight, tt.after)
			if got.State != tt.want || got.LowerBound != tt.lower {
				t.Fatalf("got=%+v want state=%s lower=%d", got, tt.want, tt.lower)
			}
		})
	}
}
