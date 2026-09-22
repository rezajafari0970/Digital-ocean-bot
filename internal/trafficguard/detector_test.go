package trafficguard

import (
	"testing"
	"time"
)

func TestDetectorRequiresConfirmations(t *testing.T) {
	p := Policy{MinSamples: 1, Alpha: .2, SigmaMultiplier: 3, MinBytesPerSec: 100, Confirmations: 3, Action: ActionDisable}
	base := Baseline{ClientID: "c", EWMABytesPerSec: 10, Variance: 1, Samples: 10}
	start := time.Now()
	prev := Sample{ClientID: "c", At: start}
	for i := 1; i <= 3; i++ {
		cur := Sample{ClientID: "c", DownBytes: int64(i * 1000), At: start.Add(time.Duration(i) * time.Second)}
		var d Decision
		base, d = Analyze(prev, cur, base, p)
		if i < 3 && d.Confirmed {
			t.Fatal("confirmed too early")
		}
		if i == 3 && !d.Confirmed {
			t.Fatal("not confirmed")
		}
		prev = cur
	}
}

func TestCounterResetIsNotAnomaly(t *testing.T) {
	p := Policy{MinBytesPerSec: 1}
	b, d := Analyze(Sample{UpBytes: 1000, At: time.Now()}, Sample{UpBytes: 1, At: time.Now().Add(time.Second)}, Baseline{}, p)
	if d.Suspicious || d.Reason != "counter_reset" || b.ConsecutiveAnomalies != 0 {
		t.Fatal("counter reset mishandled")
	}
}
