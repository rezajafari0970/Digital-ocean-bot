package capacity

import "testing"

func TestNetOccupancyReplacementPairsAreNetNeutral(t *testing.T) {
	tests := []struct {
		name                                    string
		managed, pending, provider, pairs, want int
	}{
		{"normal pending counts", 14, 1, 14, 0, 15},
		{"provider high watermark counts", 14, 0, 15, 0, 15},
		{"precreate replacement is neutral", 14, 1, 14, 1, 14},
		{"materialized replacement is neutral", 15, 0, 15, 1, 14},
		{"external provider usage still counts", 15, 0, 16, 1, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NetOccupancy(tt.managed, tt.pending, tt.provider, tt.pairs); got != tt.want {
				t.Fatalf("got=%d want=%d", got, tt.want)
			}
		})
	}
}
