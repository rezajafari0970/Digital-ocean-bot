package adminapi

import (
	"context"
	"fmt"
	"testing"
)

func TestTrialRelayStatusFreshnessLifetimeAndPerPanel(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	ctx := context.Background()
	account := seedBuildAccount(t, db, "upcloud")
	makePanel := func(account string) string {
		panel, drop := perfUUID(), perfUUID()
		sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at)VALUES($1,$2,$1::uuid::text,'READY',now()+interval '1 hour')", drop, account)
		sqlMust(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref)VALUES($1,$2,$3,'sanaei-3x-ui','https://fixture.invalid','fixture')", panel, account, drop)
		sqlMust(t, db, "INSERT INTO panel_routing_state(panel_id,state,relay_mode,plan_hash,category_digest,verified_at)VALUES($1,'APPLIED',false,$1::uuid::text,'same',now())", panel)
		return panel
	}
	receivers := []string{makePanel(account), makePanel(account)}
	sqlMust(t, db, "UPDATE panel_routing_state SET relay_mode=true")
	for i := 0; i < 3; i++ {
		da := seedBuildAccount(t, db, "vultr")
		donor := makePanel(da)
		sqlMust(t, db, "UPDATE accounts SET provider_state='ACTIVE' WHERE id=$1", da)
		sqlMust(t, db, "INSERT INTO panel_relay_endpoints(panel_id,account_id,host,port,secret_ref,transport_hash,state,plan_hash,verified_at,valid_until)VALUES($1,$2,$3,8080,'fixture','transport','APPLIED',$1::uuid::text,now(),now()+interval '1 hour')", donor, da, fmt.Sprintf("203.0.113.%d", i+1))
		for _, receiver := range receivers {
			sqlMust(t, db, "INSERT INTO panel_relay_assignments(receiver_panel_id,donor_panel_id,donor_droplet_id,donor_account_id,secret_ref,transport_hash,donor_plan_hash,receiver_plan_hash,outbound_tag,applied,alive,observed_at,last_success_at)SELECT $1,id,droplet_id,account_id,'fixture','transport',id::text,$1::uuid::text,'fixture',true,true,now(),now() FROM panel_instances WHERE id=$2", receiver, donor)
		}
	}
	check := func(want string, n int) {
		t.Helper()
		got, e := s.trialRelayStatus(ctx, account)
		if e != nil || got == nil || got.Status != want || got.Healthy != n || len(got.Panels) != 2 {
			t.Fatalf("%+v %v want %s %d", got, e, want, n)
		}
	}
	check("healthy", 6)
	sqlMust(t, db, "UPDATE panel_relay_assignments SET observed_at=now()-interval '30 seconds' WHERE receiver_panel_id=$1", receivers[0])
	check("unavailable", 3)
	sqlMust(t, db, "UPDATE panel_relay_assignments SET observed_at=now()")
	sqlMust(t, db, "UPDATE panel_relay_endpoints SET valid_until=now()+interval '3 minutes' WHERE host='203.0.113.1'")
	check("degraded", 4)
	sqlMust(t, db, "UPDATE droplets SET expires_at=now()+interval '3 minutes' WHERE id IN(SELECT p.droplet_id FROM panel_instances p JOIN panel_relay_endpoints ep ON ep.panel_id=p.id WHERE ep.host='203.0.113.2')")
	check("degraded", 2)
	sqlMust(t, db, "UPDATE panel_relay_endpoints SET transport_hash='changed' WHERE host='203.0.113.3'")
	check("unavailable", 0)
}
