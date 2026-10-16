package usercapacity

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestFleetCoverageBeyond64PreservesExecutionAndScopeGuards(t *testing.T) {
	db := admissionTestDB(t)
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`CREATE TABLE global_config_policies(policy_key text PRIMARY KEY,enabled bool,ports jsonb);
 INSERT INTO global_config_policies VALUES('reality',true,'[443]');
 UPDATE bulk_lifecycle_control SET enabled=true,auto_enroll=true,max_active_scopes=0,operation_budget=23,max_batch_size=10;
 UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=NULL,inbound_id=NULL;
 UPDATE droplets SET expires_at=now()+interval '2 hours';`)
	s := Service{DB: db}
	ctx := context.Background()
	for i := 0; i < 96; i++ {
		panel, err := sanaei.UUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		must(`INSERT INTO panel_instances VALUES($1,'11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222',true)`, panel)
		if err = s.admitLifecycleScope(ctx, panel, 1, 443); err != nil {
			t.Fatal(i, err)
		}
	}
	var n, minBudget, maxBudget, concurrency int
	if err := db.QueryRow(`SELECT count(*),min(remaining_operations),max(remaining_operations) FROM bulk_lifecycle_scopes`).Scan(&n, &minBudget, &maxBudget); err != nil || n != 96 || minBudget != 23 || maxBudget != 23 {
		t.Fatal(n, minBudget, maxBudget, err)
	}
	if err := db.QueryRow(`SELECT concurrency FROM client_mutation_execution_gate`).Scan(&concurrency); err != nil || concurrency != 1 {
		t.Fatal("execution widened", concurrency, err)
	}
	for _, change := range []string{"UPDATE client_mutation_execution_gate SET kill_switch=true", "UPDATE client_mutation_execution_gate SET kill_switch=false; UPDATE accounts SET provider_state='LOCKED'", "UPDATE accounts SET provider_state='ACTIVE'; UPDATE droplets SET expires_at=now()+interval '30 seconds'"} {
		must(change)
		if err := s.admitLifecycleScope(ctx, admissionPanel, 1, 443); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM bulk_lifecycle_scopes`).Scan(&n); err != nil || n != 96 {
			t.Fatal("all-eligible bypassed safety gate", n, err)
		}
	}
	must("UPDATE droplets SET expires_at=now()+interval '2 hours'; UPDATE bulk_lifecycle_control SET max_active_scopes=64")
	if err := s.admitLifecycleScope(ctx, admissionPanel, 1, 443); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM bulk_lifecycle_scopes`).Scan(&n); err != nil || n != 96 {
		t.Fatal("custom cap ignored", n, err)
	}
}
