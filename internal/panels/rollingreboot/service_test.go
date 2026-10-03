package rollingreboot

import (
	"strings"
	"testing"
)

func TestMax(t *testing.T) {
	if max(1, 2) != 2 || max(3, 2) != 3 {
		t.Fatal("max")
	}
}

func TestDeferredAndObsoleteAreNeverClaimed(t *testing.T) {
	if !strings.Contains(claimSQL, "WHERE j.state='PENDING'") {
		t.Fatal("claim query must require PENDING")
	}
	for _, forbidden := range []string{"j.state='DEFERRED'", "j.state='OBSOLETE'"} {
		if strings.Contains(claimSQL, forbidden) {
			t.Fatalf("claim query must not admit %s", forbidden)
		}
	}
}

func TestDeferredReadyDoesNotBecomeObsolete(t *testing.T) {
	if strings.Contains(reconcileDeferredSQL, "d.state='READY'") {
		t.Fatal("READY droplets must not be obsoleted")
	}
	if strings.Contains(reconcileDeferredSQL, "reboot_required=false") {
		t.Fatal("guardian signal alone must not obsolete deferred maintenance")
	}
}

func TestDeferredLifecycleStatesBecomeObsolete(t *testing.T) {
	for _, state := range []string{"RETIRING", "DELETING", "DELETED"} {
		if !strings.Contains(reconcileDeferredSQL, "'"+state+"'") {
			t.Fatalf("missing lifecycle obsolete state %s", state)
		}
	}
	if !strings.Contains(reconcileDeferredSQL, "NOT EXISTS (SELECT 1 FROM droplets") {
		t.Fatal("deleted droplet row must obsolete deferred maintenance")
	}
}

func TestReconcileLeavesCompletedAndFailedUntouched(t *testing.T) {
	if !strings.Contains(reconcileDeferredSQL, "WHERE j.state='DEFERRED'") {
		t.Fatal("reconcile must be restricted to DEFERRED jobs")
	}
	for _, terminal := range []string{"COMPLETED", "FAILED"} {
		if strings.Contains(reconcileDeferredSQL, "j.state='"+terminal+"'") {
			t.Fatalf("reconcile must not target terminal state %s", terminal)
		}
	}
}

func TestDeferredCanBeExplicitlyReactivated(t *testing.T) {
	if !strings.Contains(reactivateSQL, "SET state='PENDING'") {
		t.Fatal("reactivation must return an eligible job to PENDING")
	}
	if !strings.Contains(reactivateSQL, "j.state IN ('DEFERRED','OBSOLETE')") {
		t.Fatal("reactivation must be explicit from deferred/obsolete maintenance states")
	}
	if !strings.Contains(reactivateSQL, "d.state='READY'") || !strings.Contains(reactivateSQL, "rs.reboot_required=true") {
		t.Fatal("reactivation must re-check current maintenance eligibility")
	}
}
