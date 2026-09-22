package trafficguard

import (
	"math"
	"time"
)

func Analyze(previous, current Sample, baseline Baseline, policy Policy) (Baseline, Decision) {
	if policy.Alpha <= 0 || policy.Alpha > 1 {
		policy.Alpha = .2
	}
	if policy.MinSamples < 1 {
		policy.MinSamples = 5
	}
	if policy.SigmaMultiplier <= 0 {
		policy.SigmaMultiplier = 4
	}
	if policy.Confirmations < 1 {
		policy.Confirmations = 3
	}
	dt := current.At.Sub(previous.At).Seconds()
	if dt <= 0 {
		return baseline, Decision{Reason: "invalid_interval"}
	}
	delta := current.Total() - previous.Total()
	if delta < 0 {
		return baseline, Decision{Reason: "counter_reset"}
	}
	rate := float64(delta) / dt
	threshold := policy.MinBytesPerSec
	if baseline.Samples >= policy.MinSamples {
		dynamic := baseline.EWMABytesPerSec + policy.SigmaMultiplier*math.Sqrt(math.Max(baseline.Variance, 0))
		if dynamic > threshold {
			threshold = dynamic
		}
	}
	suspicious := rate >= threshold && threshold > 0
	if policy.HardBytesPerSec > 0 && rate >= policy.HardBytesPerSec {
		suspicious = true
	}
	if suspicious {
		baseline.ConsecutiveAnomalies++
	} else {
		baseline.ConsecutiveAnomalies = 0
	}
	confirmed := baseline.ConsecutiveAnomalies >= policy.Confirmations
	baseline = updateBaseline(baseline, rate, policy.Alpha, current.At, !suspicious)
	return baseline, Decision{Suspicious: suspicious, Confirmed: confirmed, RateBytesPerSec: rate, Threshold: threshold, Reason: reason(suspicious, confirmed), Action: policy.Action}
}

func updateBaseline(b Baseline, rate, alpha float64, at time.Time, learn bool) Baseline {
	if !learn {
		return b
	}
	if b.Samples == 0 {
		b.EWMABytesPerSec = rate
		b.Variance = 0
	} else {
		diff := rate - b.EWMABytesPerSec
		b.EWMABytesPerSec += alpha * diff
		b.Variance = (1 - alpha) * (b.Variance + alpha*diff*diff)
	}
	b.Samples++
	b.UpdatedAt = at
	return b
}
func reason(s, c bool) string {
	if c {
		return "confirmed_anomaly"
	}
	if s {
		return "candidate_anomaly"
	}
	return "normal"
}
