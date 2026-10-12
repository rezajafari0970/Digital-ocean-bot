package advancedreality

import (
	"math"
	"time"
)

// Window contains aggregates only. No address, UUID, SNI, payload or key.
type Window struct {
	At                time.Time `json:"at"`
	Samples           int       `json:"samples"`
	CPU               float64   `json:"cpu_percent"`
	Memory            float64   `json:"memory_percent"`
	ErrorRate         float64   `json:"error_rate"`
	BaselineErrorRate float64   `json:"baseline_error_rate"`
	RTTP95MS          float64   `json:"rtt_p95_ms"`
	BaselineRTTP95MS  float64   `json:"baseline_rtt_p95_ms"`
	Complete          bool      `json:"complete"`
}
type PolicyState struct {
	LastObservation time.Time `json:"last_observation"`
	LastChange      time.Time `json:"last_change"`
	PressureWindows int       `json:"pressure_windows"`
	HealthyWindows  int       `json:"healthy_windows"`
	Conservative    bool      `json:"conservative"`
}
type Decision struct {
	Action   string      `json:"action"`
	Reason   string      `json:"reason"`
	State    PolicyState `json:"state"`
	CanApply bool        `json:"can_apply"`
}

// Recommend is a shadow evaluator. It never changes a socket, configuration or
// deadline. Three sustained windows enter a resource-conservative proposal;
// five healthy windows leave it, with a five-minute cooldown. Failed evidence
// breaks the run. A replayed window cannot satisfy the hysteresis counter.
func Recommend(s PolicyState, w Window, now time.Time) Decision {
	d := Decision{Action: "HOLD", Reason: "stable", State: s}
	valid := w.Complete && w.Samples >= 100 && !w.At.IsZero() && !w.At.After(now) && now.Sub(w.At) <= 60*time.Second && s.PressureWindows >= 0 && s.PressureWindows <= 3 && s.HealthyWindows >= 0 && s.HealthyWindows <= 5 && !s.LastChange.After(now)
	for _, x := range []float64{w.CPU, w.Memory, w.ErrorRate, w.BaselineErrorRate, w.RTTP95MS, w.BaselineRTTP95MS} {
		if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 {
			valid = false
		}
	}
	valid = valid && w.CPU <= 100 && w.Memory <= 100 && w.ErrorRate <= 1 && w.BaselineErrorRate <= 1 && w.BaselineRTTP95MS > 0
	if !valid {
		d.Action = "BLOCKED"
		d.Reason = "missing_stale_or_invalid_evidence"
		d.State.PressureWindows = 0
		d.State.HealthyWindows = 0
		return d
	}
	if !w.At.After(s.LastObservation) {
		d.Action = "BLOCKED"
		d.Reason = "duplicate_or_out_of_order_window"
		return d
	}
	if !s.LastObservation.IsZero() && w.At.Sub(s.LastObservation) > 60*time.Second {
		s.PressureWindows = 0
		s.HealthyWindows = 0
		d.State.PressureWindows = 0
		d.State.HealthyWindows = 0
	}
	d.State.LastObservation = w.At
	if w.ErrorRate > w.BaselineErrorRate+0.02 || w.RTTP95MS > w.BaselineRTTP95MS*1.25 {
		d.Action = "ROLLBACK_PROPOSAL"
		d.Reason = "health_regression"
		d.State.PressureWindows = 0
		d.State.HealthyWindows = 0
		return d
	}
	pressure := w.CPU >= 85 || w.Memory >= 85
	healthy := w.CPU < 65 && w.Memory < 70
	if pressure {
		d.State.PressureWindows = min(3, s.PressureWindows+1)
		d.State.HealthyWindows = 0
	} else if healthy {
		d.State.HealthyWindows = min(5, s.HealthyWindows+1)
		d.State.PressureWindows = 0
	} else {
		d.State.PressureWindows = 0
		d.State.HealthyWindows = 0
	}
	if !s.LastChange.IsZero() && now.Sub(s.LastChange) < 5*time.Minute {
		d.Reason = "cooldown"
		return d
	}
	if !s.Conservative && d.State.PressureWindows >= 3 {
		d.Action = "REDUCE_NEW_ADMISSION_PROPOSAL"
		d.Reason = "sustained_resource_pressure"
		d.State.Conservative = true
		d.State.LastChange = now
	}
	if s.Conservative && d.State.HealthyWindows >= 5 {
		d.Action = "RESTORE_ADMISSION_PROPOSAL"
		d.Reason = "sustained_recovery"
		d.State.Conservative = false
		d.State.LastChange = now
	}
	return d
}
