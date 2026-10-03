package usercapacity

import (
	"testing"
	"time"
)

func TestPlannedRecoveryDecisionRequiresIndependentObservations(t *testing.T) {
	created := time.Unix(1000, 0)
	if got := plannedRecoveryDecision(created, 0, nil, created.Add(10*time.Second)); got != plannedWait {
		t.Fatalf("young=%v", got)
	}
	t1 := created.Add(15 * time.Second)
	if got := plannedRecoveryDecision(created, 0, nil, t1); got != plannedCheck {
		t.Fatalf("first=%v", got)
	}
	if got := plannedRecoveryDecision(created, 1, &t1, t1.Add(4*time.Second)); got != plannedWait {
		t.Fatalf("spacing=%v", got)
	}
	t2 := t1.Add(5 * time.Second)
	if got := plannedRecoveryDecision(created, 1, &t1, t2); got != plannedCheck {
		t.Fatalf("second=%v", got)
	}
	t3 := t2.Add(5 * time.Second)
	if got := plannedRecoveryDecision(created, 2, &t2, t3); got != plannedAbort {
		t.Fatalf("third=%v", got)
	}
}

func TestPlannedRecoveryClockRollbackWaits(t *testing.T) {
	created := time.Unix(1000, 0)
	last := created.Add(20 * time.Second)
	if got := plannedRecoveryDecision(created, 2, &last, created.Add(19*time.Second)); got != plannedWait {
		t.Fatalf("got=%v", got)
	}
}
