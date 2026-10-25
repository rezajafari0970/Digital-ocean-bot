package configplane

import (
	"context"
	"testing"
)

func TestPendingPortReservationIsPanelScoped(t *testing.T) {
	db := cdnProofDB(t)
	cdnExec(t, db, "UPDATE config_control_authority SET unified=true WHERE singleton")
	_, old, blockedPanel := cdnFixture(t, db)
	_, replacement, freePanel := cdnFixture(t, db)
	cdnExec(t, db, "UPDATE config_driver_profiles SET enabled=false,delete_requested_at=now() WHERE id=$1", old)
	svc := Service{DB: db}
	ctx := context.Background()
	for _, state := range []string{"CONFIRMED", "PREPARED", "SENT"} {
		cdnExec(t, db, "UPDATE config_driver_listeners SET state='BLOCKED',remote_id=CASE WHEN $2='CONFIRMED' THEN 123 ELSE NULL END,applied_spec='{\"port\":443}',mutation_spec='{\"port\":443}',mutation_state=$2 WHERE profile_id=$1", old, state)
		if err := svc.checkListenerPortReservation(ctx, replacement, blockedPanel, 443); err == nil || err.Error() != "profile_port_cleanup_pending" {
			t.Fatal("pending reservation lost", state, err)
		}
		if err := svc.checkListenerPortReservation(ctx, replacement, freePanel, 443); err != nil {
			t.Fatal("one panel blocked another", state, err)
		}
		if err := svc.checkListenerPortReservation(ctx, replacement, blockedPanel, 8443); err != nil {
			t.Fatal("one port blocked another", state, err)
		}
		if err := svc.checkListenerPortReservation(ctx, old, blockedPanel, 443); err != nil {
			t.Fatal("owner blocked its own recovery", state, err)
		}
	}
	cdnExec(t, db, "UPDATE config_driver_profiles SET delete_requested_at=NULL WHERE id=$1", old)
	if err := svc.checkListenerPortReservation(ctx, replacement, blockedPanel, 443); err == nil || err.Error() != "profile_port_conflict" {
		t.Fatal("paused owner was treated as deleting", err)
	}
	cdnExec(t, db, "UPDATE config_driver_listeners SET state='RETIRED',remote_id=NULL,mutation_state='CONFIRMED' WHERE profile_id=$1", old)
	if err := svc.checkListenerPortReservation(ctx, replacement, blockedPanel, 443); err != nil {
		t.Fatal("confirmed removal did not release panel", err)
	}
}
