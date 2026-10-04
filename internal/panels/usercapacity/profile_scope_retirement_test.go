package usercapacity

import (
	"context"
	"testing"
)

func TestRemovedPolicyPortClosesOnlySettledScopeAndPreservesBudget(t *testing.T) {
	db := admissionTestDB(t)
	exec := func(q string) {
		t.Helper()
		if _, e := db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	exec(`CREATE TABLE global_config_policies(policy_key text PRIMARY KEY,enabled bool,ports jsonb);INSERT INTO global_config_policies VALUES('reality',true,'[8443]');
 UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false;
 INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,remaining_operations,expires_at) VALUES('55555555-5555-4555-8555-555555555555',1,'44444444-4444-4444-8444-444444444444',true,true,true,23,now()+interval '1 hour');
 INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES('11111111-1111-4111-8111-111111111111','55555555-5555-4555-8555-555555555555',1,'pending','CREATE','retirement-test','{}');`)
	s := Service{DB: db}
	ctx := context.Background()
	if err := s.retirePolicyScopes(ctx, admissionPanel); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	var budget int
	var reason string
	check := func(want bool) {
		t.Helper()
		if err := db.QueryRow(`SELECT enabled,remaining_operations,last_error FROM bulk_lifecycle_scopes`).Scan(&enabled, &budget, &reason); err != nil || enabled != want || budget != 23 {
			t.Fatal(enabled, budget, reason, err)
		}
	}
	check(true)
	exec(`UPDATE client_mutation_jobs SET state='OBSOLETE'`)
	if err := s.retirePolicyScopes(ctx, admissionPanel); err != nil {
		t.Fatal(err)
	}
	check(false)
	if reason != "policy_port_removed" {
		t.Fatal(reason)
	}
	exec(`UPDATE global_config_policies SET ports='[443]'`)
	if err := s.retirePolicyScopes(ctx, admissionPanel); err != nil {
		t.Fatal(err)
	}
	check(false)
}
