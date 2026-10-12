package advancedreality

import (
	"math"
	"testing"
	"time"
)

func window(at time.Time) Window {
	return Window{At: at, Samples: 100, CPU: 90, Memory: 60, ErrorRate: .01, BaselineErrorRate: .01, RTTP95MS: 110, BaselineRTTP95MS: 100, Complete: true}
}
func TestPolicyHysteresisCooldownRecovery(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	s := PolicyState{}
	for i := range 3 {
		at := now.Add(time.Duration(i) * 30 * time.Second)
		d := Recommend(s, window(at), at)
		s = d.State
		if d.CanApply {
			t.Fatal("actuating")
		}
		if i < 2 && d.Action != "HOLD" {
			t.Fatal(d)
		}
		if i == 2 && d.Action != "REDUCE_NEW_ADMISSION_PROPOSAL" {
			t.Fatal(d)
		}
	}
	for i := 1; i <= 11; i++ {
		at := now.Add(time.Minute + time.Duration(i)*30*time.Second)
		w := window(at)
		w.CPU = 40
		d := Recommend(s, w, at)
		s = d.State
		if i < 10 && d.Action != "HOLD" {
			t.Fatal("cooldown bypass", d)
		}
		if i == 10 && d.Action != "RESTORE_ADMISSION_PROPOSAL" {
			t.Fatal(d)
		}
	}
}
func TestPolicyRejectsMissingStaleReplayAndNonfinite(t *testing.T) {
	now := time.Now()
	w := window(now)
	for _, change := range []func(*Window){func(w *Window) { w.Complete = false }, func(w *Window) { w.At = now.Add(-61 * time.Second) }, func(w *Window) { w.At = now.Add(time.Second) }, func(w *Window) { w.CPU = math.NaN() }, func(w *Window) { w.Memory = 101 }, func(w *Window) { w.Samples = 99 }, func(w *Window) { w.BaselineRTTP95MS = 0 }} {
		x := w
		change(&x)
		d := Recommend(PolicyState{PressureWindows: 2}, x, now)
		if d.Action != "BLOCKED" || d.State.PressureWindows != 0 {
			t.Fatal(d)
		}
	}
	d := Recommend(PolicyState{LastObservation: now}, w, now)
	if d.Action != "BLOCKED" {
		t.Fatal("replay counted")
	}
	d = Recommend(PolicyState{LastObservation: now.Add(-10 * time.Minute), PressureWindows: 2}, w, now)
	if d.Action != "HOLD" || d.State.PressureWindows != 1 {
		t.Fatal("gap not reset", d)
	}
}
func TestPolicyHealthRegressionOverridesCooldown(t *testing.T) {
	now := time.Now()
	w := window(now)
	w.ErrorRate = .04
	d := Recommend(PolicyState{LastChange: now.Add(-time.Second)}, w, now)
	if d.Action != "ROLLBACK_PROPOSAL" || d.CanApply {
		t.Fatal(d)
	}
}
