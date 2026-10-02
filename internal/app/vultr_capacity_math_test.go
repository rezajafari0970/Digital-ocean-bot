package app

import "testing"

func TestVultrProbeEvidenceUsesStrongestAPIEvidence(t *testing.T) {
	tests := []struct{ old, api, limit, inUse int }{
		{17, 18, 18, 18},
		{17, 20, 20, 20},
		{17, 0, 18, 18},
		{5, 4, 6, 6},
	}
	for _, tc := range tests {
		limit, inUse := vultrProbeEvidence(tc.old, tc.api)
		if limit != tc.limit || inUse != tc.inUse {
			t.Fatalf("old=%d api=%d got=(%d,%d) want=(%d,%d)", tc.old, tc.api, limit, inUse, tc.limit, tc.inUse)
		}
	}
}
