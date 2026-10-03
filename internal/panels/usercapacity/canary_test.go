package usercapacity

import (
	"testing"
	"time"
)

func TestCanaryBounds(t *testing.T) {
	tests := []struct {
		target, rate int
		ttl          time.Duration
		valid        bool
	}{
		{2, 1, time.Minute, true},
		{10000, 100, 15 * time.Minute, true},
		{1, 1, time.Minute, false},
		{10001, 1, time.Minute, false},
		{20, 101, time.Minute, false},
		{20, 10, 30 * time.Second, false},
	}
	for _, tc := range tests {
		valid := tc.target >= 2 && tc.target <= 10000 && tc.rate >= 1 && tc.rate <= 100 &&
			tc.ttl >= time.Minute && tc.ttl <= 15*time.Minute
		if valid != tc.valid {
			t.Fatalf("case %+v valid=%v", tc, valid)
		}
	}
}

func TestCompactPanelID(t *testing.T) {
	if got := compactPanelID("249a0885-6310-4103-81f4-0ab06071ea1f"); got != "249a08856310410381f40ab06071ea1f" {
		t.Fatalf("compact=%s", got)
	}
}
