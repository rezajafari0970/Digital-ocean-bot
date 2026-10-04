package clientops

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestProfileIntervalDurableIndependentNoCatchupBurst(t *testing.T) {
	db, j, rt, state := profileFixture(t)
	if _, err := db.Exec("UPDATE reality_config_profiles SET creation_interval_seconds=360,target_users_per_inbound=3,user_lifetime_seconds=0"); err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("first plan missing")
	}
	runLife(t, j, rt)
	if !planLife(t, j, rt) {
		t.Fatal("other class was blocked")
	}
	runLife(t, j, rt)
	if len(state.createSizes) != 2 || state.createSizes[0] != 1 || state.createSizes[1] != 1 {
		t.Fatal("interval created a batch", state.createSizes)
	}
	// A new journal/worker must use the same persisted clock.
	restarted := Journal{DB: db}
	if planLife(t, restarted, rt) {
		t.Fatal("restart bypassed interval")
	}
	// Downtime does not accumulate scheduled creates.
	if _, err := db.Exec("UPDATE config_creation_schedule SET last_planned_at=now()-interval '1 day' WHERE route_class='RESIDENTIAL'"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := restarted.PlanLifecycle(context.Background(), rt, 1, func(_ context.Context, _, n int) (int, error) { return n, nil })
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM client_mutation_jobs WHERE state='PENDING'").Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	runLife(t, restarted, rt)
	if state.createSizes[len(state.createSizes)-1] != 1 || planLife(t, restarted, rt) {
		t.Fatal("catch-up burst")
	}
	if _, ok := state.clients["manual"]; !ok {
		t.Fatal("manual client changed")
	}
	// Expired/quota cleanup is not delayed by the creation clock, but replacement is.
	for email, c := range state.clients {
		if c.TotalGB == 202 {
			state.used[email] = 202
		}
	}
	if !planLife(t, restarted, rt) {
		t.Fatal("cleanup delayed")
	}
	job := runLife(t, restarted, rt)
	if job.Kind != KindBulkDelete {
		t.Fatal(job.Kind)
	}
	if planLife(t, restarted, rt) {
		t.Fatal("early replacement")
	}
}

func TestProfileIntervalClockRollsBackWithPlan(t *testing.T) {
	db, j, rt, _ := profileFixture(t)
	if _, err := db.Exec(`UPDATE reality_config_profiles SET creation_interval_seconds=360;
 CREATE FUNCTION fail_interval_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture ownership failure'; END $$;
 CREATE TRIGGER fail_interval_fixture BEFORE INSERT ON bulk_user_ownership FOR EACH ROW EXECUTE FUNCTION fail_interval_fixture();`); err != nil {
		t.Fatal(err)
	}
	_, err := j.PlanLifecycle(context.Background(), rt, 1, func(_ context.Context, _, n int) (int, error) { return n, nil })
	if err == nil {
		t.Fatal("expected rollback")
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM config_creation_schedule").Scan(&n); err != nil || n != 0 {
		t.Fatal("clock escaped failed plan", n, err)
	}
	if err = db.QueryRow("SELECT count(*) FROM client_mutation_jobs").Scan(&n); err != nil || n != 0 {
		t.Fatal("job escaped failed plan", n, err)
	}
	if _, err = db.Exec("DROP TRIGGER fail_interval_fixture ON bulk_user_ownership"); err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("retry delayed by rolled-back clock")
	}
	// Simulate a lost POST response with one durable identity, recover the same job.
	job, ok, err := j.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	var last time.Time
	if err = db.QueryRow("SELECT last_planned_at FROM config_creation_schedule WHERE route_class='DIRECT'").Scan(&last); err != nil {
		t.Fatal(err)
	}
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	db.QueryRow("SELECT last_planned_at FROM config_creation_schedule WHERE route_class='DIRECT'").Scan(&after)
	if !after.Equal(last) {
		t.Fatal("recovery reset interval")
	}
}
