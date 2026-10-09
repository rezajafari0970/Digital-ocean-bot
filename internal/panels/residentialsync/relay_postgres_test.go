package residentialsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"testing"
)

func relayDBPanel(t *testing.T, db *sql.DB, provider, host string, trial bool) (account, panel, drop string) {
	account, panel, drop = trialUUID(), trialUUID(), trialUUID()
	profile, dep := trialUUID(), trialUUID()
	sqlMustTrial(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,provider_state)VALUES($1,$1::uuid::text,$2,'fixture','ACTIVE')", account, provider)
	sqlMustTrial(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state,expires_at)VALUES($1,$2,$1::uuid::text,'READY',now()+interval '1 hour')", drop, account)
	sqlMustTrial(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config)VALUES($1,$2,'fixture','{}')", profile, account)
	sqlMustTrial(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,host,profile_snapshot)VALUES($1,$2,$3,$4,'PANEL_COMPLETE','done',$5,$6::jsonb)", dep, account, profile, drop, host, fmt.Sprintf(`{"upcloud_trial_compatible":%t}`, trial))
	sqlMustTrial(t, db, "INSERT INTO panel_instances(id,account_id,droplet_id,driver,base_url,auth_secret_ref)VALUES($1,$2,$3,'sanaei-3x-ui','https://fixture.invalid','fixture')", panel, account, drop)
	return
}
func TestManagedSOCKSSelectionAndPublicationPostgres(t *testing.T) {
	t.Setenv("DOB_RESIDENTIAL_POOL_PANELS", "")
	t.Setenv("DOB_UPCLOUD_RELAY_EXPANDED_PANELS", "all")
	db := trialTestDB(t)
	ctx := context.Background()
	for _, key := range []string{"DOB_UPCLOUD_RELAY_PANELS", "DOB_RESIDENTIAL_ALLOWLIST_PANELS", "DOB_RESIDENTIAL_CLIENT_PATHS_PANELS", "DOB_RESIDENTIAL_ADS_ONLY_PANELS", "DOB_RESIDENTIAL_HARDENING_PANELS", "DOB_ROUTING_STABLE_PLAN_PANELS"} {
		t.Setenv(key, "all")
	}
	sqlMustTrial(t, db, "INSERT INTO global_config_policies(policy_key,ports)VALUES('reality','[443]')")
	sqlMustTrial(t, db, "UPDATE residential_routing_control SET enabled=true,fleet=true")
	_, receiver, _ := relayDBPanel(t, db, "upcloud", "198.51.100.9", true)
	store, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	svc := Service{DB: db, Secrets: store}
	policy, rev, e := svc.policy(ctx, receiver)
	if e != nil {
		t.Fatal(e)
	}
	if !policy.RelayMode || len(policy.Proxies) != 0 {
		t.Fatal("empty trial was not closed")
	}
	proxyID := trialUUID()
	sqlMustTrial(t, db, "INSERT INTO residential_proxies(proxy_id,name,type,host,port,outbound_tag,status,last_success_at)VALUES($1,'fixture','socks5','residential.invalid',10000,$1::uuid::text,'healthy',now()+interval '1 second')", proxyID)
	policy, rev, e = svc.policy(ctx, receiver)
	if e != nil {
		t.Fatal(e)
	}
	donors := []string{}
	for i, provider := range []string{"vultr", "digitalocean", "linode", "vultr"} {
		host := fmt.Sprintf("203.0.113.%d", i+1)
		a, panel, _ := relayDBPanel(t, db, provider, host, false)
		donors = append(donors, panel)
		c, e := newRelayCredential()
		if e != nil {
			t.Fatal(e)
		}
		ref := "fixture-relay"
		raw, _ := json.Marshal(c)
		if e = store.Put(ctx, a, ref, "trial_relay", raw); e != nil {
			t.Fatal(e)
		}
		proxy, e := managedSOCKS(panel, host, ref, 8080, c)
		if e != nil {
			t.Fatal(e)
		}
		sqlMustTrial(t, db, "INSERT INTO panel_routing_state(panel_id,state,revision,plan_hash,pool_enabled,category_digest,verified_at)VALUES($1,'APPLIED',$2,'donor-plan',true,$3,now())", panel, rev, categoryDigest(policy))
		sqlMustTrial(t, db, "INSERT INTO panel_relay_endpoints(panel_id,account_id,host,port,secret_ref,transport_hash,allowed_sources,state,plan_hash,verified_at,valid_until)VALUES($1,$2,$3,8080,$4,$5,ARRAY['198.51.100.9/32'],'APPLIED','donor-plan',now(),now()+interval '1 hour')", panel, a, host, ref, proxy.TransportHash)
	}
	policy, _, e = svc.policy(ctx, receiver)
	if e != nil || len(policy.Proxies) != 4 {
		t.Fatalf("wanted every eligible server: count=%d error=%v", len(policy.Proxies), e)
	}
	// Unknown or short-lived server/config cannot enter, even if native health is fresh.
	for _, tc := range []struct{ name, update, restore string }{
		{"server three minutes", "UPDATE droplets SET expires_at=now()+interval '3 minutes' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)", "UPDATE droplets SET expires_at=now()+interval '1 hour' WHERE id=(SELECT droplet_id FROM panel_instances WHERE id=$1)"},
		{"config three minutes", "UPDATE panel_relay_endpoints SET valid_until=now()+interval '3 minutes' WHERE panel_id=$1", "UPDATE panel_relay_endpoints SET valid_until=now()+interval '1 hour' WHERE panel_id=$1"},
		{"new config seven minutes", "UPDATE panel_relay_endpoints SET valid_until=now()+interval '7 minutes' WHERE panel_id=$1", "UPDATE panel_relay_endpoints SET valid_until=now()+interval '1 hour' WHERE panel_id=$1"},
		{"unknown config expiry", "UPDATE panel_relay_endpoints SET valid_until=NULL WHERE panel_id=$1", "UPDATE panel_relay_endpoints SET valid_until=now()+interval '1 hour' WHERE panel_id=$1"},
	} {
		sqlMustTrial(t, db, tc.update, donors[0])
		q, _, err := svc.policy(ctx, receiver)
		if err != nil || len(q.Proxies) != 3 {
			t.Fatalf("%s: %d %v", tc.name, len(q.Proxies), err)
		}
		sqlMustTrial(t, db, tc.restore, donors[0])
	}
	t.Setenv("DOB_UPCLOUD_RELAY_EXPANDED_PANELS", "none")
	limited, _, err := svc.policy(ctx, receiver)
	if err != nil || len(limited.Proxies) != 3 {
		t.Fatal("canary selector not respected", err)
	}
	t.Setenv("DOB_UPCLOUD_RELAY_EXPANDED_PANELS", "all")
	desired, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"rules": []any{}}}, nil, nil, policy)
	if e != nil {
		t.Fatal(e)
	}
	hash := settingsHash(desired)
	if e = svc.persistPlan(ctx, receiver, rev, hash, policy, nil, desired); e != nil {
		t.Fatal(e)
	}
	sqlMustTrial(t, db, "UPDATE panel_routing_state SET relay_mode=true,state='APPLIED',category_digest=$2,verified_at=now() WHERE panel_id=$1", receiver, categoryDigest(policy))
	healthy := func() bool {
		var x bool
		e := db.QueryRow("SELECT "+RelayOutputHealthySQL+" FROM panel_instances p JOIN panel_routing_state rs ON rs.panel_id=p.id WHERE p.id=$1", receiver).Scan(&x)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	if healthy() {
		t.Fatal("unobserved relay published")
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_assignments SET applied=true,alive=true,observed_at=now(),last_success_at=now() WHERE receiver_panel_id=$1", receiver)
	if !healthy() {
		t.Fatal("fresh verified relays not admitted")
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_endpoints SET valid_until=now()+interval '3 minutes'")
	if healthy() {
		t.Fatal("expiring config published")
	}
	q, _, err := svc.policy(ctx, receiver)
	if err != nil || len(q.Proxies) != 0 {
		t.Fatal("expiring installed config retained", err)
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_endpoints SET valid_until=now()+interval '7 minutes'")
	q, _, err = svc.policy(ctx, receiver)
	if err != nil || len(q.Proxies) != 4 {
		t.Fatal("existing paths lost admission hysteresis", err)
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_endpoints SET valid_until=now()+interval '1 hour'")
	sqlMustTrial(t, db, "UPDATE panel_relay_assignments SET observed_at=now()-interval '30 seconds' WHERE receiver_panel_id=$1", receiver)
	if healthy() {
		t.Fatal("stale relay published")
	}
	lost := policy.Proxies[0].ID
	sqlMustTrial(t, db, "UPDATE panel_relay_assignments SET cooldown_until=now()+interval '2 minutes' WHERE receiver_panel_id=$1 AND donor_panel_id=$2", receiver, lost)
	stable, _, err := svc.policy(ctx, receiver)
	if err != nil || len(stable.Proxies) != 4 {
		t.Fatal("transient failure rewrote installed pool", err)
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_assignments SET selected=false,applied=false WHERE receiver_panel_id=$1 AND donor_panel_id=$2", receiver, lost)
	next, _, e := svc.policy(ctx, receiver)
	if e != nil || len(next.Proxies) != 3 {
		t.Fatalf("replacement: %d %v", len(next.Proxies), e)
	}
	for _, p := range next.Proxies {
		if p.ID == lost {
			t.Fatal("quarantined donor retained")
		}
	}
	sqlMustTrial(t, db, "UPDATE panel_relay_endpoints SET transport_hash='changed'")
	next, _, e = svc.policy(ctx, receiver)
	if e != nil || len(next.Proxies) != 0 {
		t.Fatal("changed credential metadata admitted", e)
	}
	// Panel removal cleans both sides of durable ownership.
	sqlMustTrial(t, db, "DELETE FROM panel_instances WHERE id=$1", donors[0])
	var count int
	if e = db.QueryRow("SELECT count(*) FROM panel_relay_endpoints WHERE panel_id=$1", donors[0]).Scan(&count); e != nil || count != 0 {
		t.Fatal("donor cascade failed", e)
	}
}
