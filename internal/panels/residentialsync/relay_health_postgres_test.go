package residentialsync

import (
	"context"
	"testing"
	"time"
)

// Hold the same first row as apply/persistPlan while the health writer runs.
// Health must wait without taking an assignment row; otherwise the plan writer
// cannot complete and PostgreSQL detects the production lock-order cycle.
func TestRelayHealthDoesNotDeadlockPlanPostgres(t *testing.T) {
	db := trialTestDB(t)
	_, receiver, _ := relayDBPanel(t, db, "upcloud", "198.51.100.8", true)
	account, donor, drop := relayDBPanel(t, db, "vultr", "203.0.113.8", false)
	sqlMustTrial(t, db, "INSERT INTO panel_routing_state(panel_id,state,plan_hash,relay_mode)VALUES($1,'APPLIED','old-plan',true)", receiver)
	tag := "residential-ads-relay-lock-fixture"
	sqlMustTrial(t, db, `INSERT INTO panel_relay_assignments(receiver_panel_id,donor_panel_id,donor_droplet_id,donor_account_id,secret_ref,transport_hash,donor_plan_hash,receiver_plan_hash,outbound_tag,selected,applied)
 VALUES($1,$2,$3,$4,'fixture','fixture','donor-plan','old-plan',$5,true,true)`, receiver, donor, drop, account, tag)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	writer, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	var pid int
	if err = writer.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.ExecContext(ctx, "UPDATE panel_routing_state SET plan_hash='new-plan' WHERE panel_id=$1", receiver); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	now := time.Now().Unix()
	go func() {
		result <- (Service{DB: db}).recordRelayObservations(ctx, receiver, []relayObservation{{Tag: tag, Alive: true, LastTryTime: now, LastSeenTime: now, UpdatedAt: now}})
	}()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-result:
			t.Fatalf("health unexpectedly finished before writer: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("health transaction did not reach the held plan row")
	}
	if _, err = writer.ExecContext(ctx, "SET LOCAL lock_timeout='200ms'"); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.ExecContext(ctx, "UPDATE panel_relay_assignments SET receiver_plan_hash='new-plan' WHERE receiver_panel_id=$1", receiver); err != nil {
		t.Fatalf("health locked assignment before plan state: %v", err)
	}
	if err = writer.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
	var applied bool
	if err = db.QueryRowContext(ctx, `SELECT a.alive AND a.applied AND a.receiver_plan_hash=r.plan_hash AND r.healthy_count=1 FROM panel_relay_assignments a JOIN panel_routing_state r ON r.panel_id=a.receiver_panel_id WHERE a.receiver_panel_id=$1`, receiver).Scan(&applied); err != nil || !applied {
		t.Fatalf("fresh observation not bound to committed plan: %v %v", applied, err)
	}
}
