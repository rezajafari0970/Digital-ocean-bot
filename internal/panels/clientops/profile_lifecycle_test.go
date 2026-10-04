package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"os"
	"strings"
	"testing"
)

func profileFixture(t *testing.T) (*sql.DB, Journal, *sanaei.PanelRuntime, *lifeServer) {
	db, j, rt, state := lifecycleFixture(t)
	raw, err := os.ReadFile("../../../migrations/000148_independent_reality_profiles.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := strings.Split(string(raw), "-- Preserve")[0]
	_, err = db.Exec(ddl + `ALTER TABLE bulk_user_ownership ADD COLUMN route_class text NOT NULL DEFAULT '';
 CREATE TABLE panel_client_routes(panel_id uuid,client_id text,email text,route_class text);
 UPDATE global_config_policies SET enabled=true,target_users_per_inbound=3;
 UPDATE bulk_lifecycle_scopes SET use_global_policy=true;
 INSERT INTO reality_config_profiles(route_class,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second) VALUES
 ('DIRECT','[443]',1,101,600,2,1),('RESIDENTIAL','[443]',1,202,1200,3,10);`)
	if err != nil {
		t.Fatal(err)
	}
	intervalDDL, err := os.ReadFile("../../../migrations/000149_profile_creation_interval.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(intervalDDL)); err != nil {
		t.Fatal(err)
	}
	return db, j, rt, state
}
func TestProfileLifecycleIndependentPlanUpdateShrinkAndReplacement(t *testing.T) {
	db, j, rt, state := profileFixture(t)
	// First one Direct, then one Residential, plus the untouched manual client.
	for i := 0; i < 2; i++ {
		if !planLife(t, j, rt) {
			t.Fatal("profile plan missing")
		}
		var cls string
		var n int
		if err := db.QueryRow(`SELECT route_class,count(*) FROM bulk_user_ownership WHERE state='PLANNED' GROUP BY route_class`).Scan(&cls, &n); err != nil || cls == "" || n != 1 {
			t.Fatal("class not durable before POST", cls, n, err)
		}
		if len(state.clients) != 1+i {
			t.Fatal("planner posted")
		}
		runLife(t, j, rt)
	}
	if planLife(t, j, rt) {
		t.Fatal("steady state not idempotent")
	}
	var directID, resID, resEmail string
	for email, c := range state.clients {
		if email == "manual" {
			continue
		}
		switch c.TotalGB {
		case 101:
			directID = c.ID
			if c.LimitHWID != 2 {
				t.Fatal(c)
			}
		case 202:
			resID = c.ID
			resEmail = email
			if c.LimitHWID != 3 {
				t.Fatal(c)
			}
		default:
			t.Fatal(c)
		}
	}
	if directID == "" || resID == "" {
		t.Fatal("independent identities missing")
	}
	if _, err := db.Exec(`UPDATE reality_config_profiles SET user_quota_bytes=303,device_limit=4,revision=revision+1 WHERE route_class='RESIDENTIAL'`); err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("update missing")
	}
	job := runLife(t, j, rt)
	if job.Kind != KindUpdate || job.ClientID != resID {
		t.Fatal("cross-class update", job.Kind, job.ClientID)
	}
	if planLife(t, j, rt) {
		t.Fatal("other class drift")
	}
	state.used[resEmail] = 303
	if !planLife(t, j, rt) {
		t.Fatal("quota delete missing")
	}
	job = runLife(t, j, rt)
	if job.Kind != KindBulkDelete {
		t.Fatal(job.Kind)
	}
	if !planLife(t, j, rt) {
		t.Fatal("replacement missing")
	}
	runLife(t, j, rt)
	for email, c := range state.clients {
		if email == "manual" {
			continue
		}
		if c.TotalGB == 101 && c.ID != directID {
			t.Fatal("direct replaced")
		}
		if c.TotalGB == 303 && c.ID == resID {
			t.Fatal("exhausted UUID reused")
		}
	}
	if _, err := db.Exec(`UPDATE reality_config_profiles SET target_users_per_inbound=0,revision=revision+1 WHERE route_class='DIRECT'`); err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("shrink missing")
	}
	job = runLife(t, j, rt)
	var p BulkPayload
	json.Unmarshal(job.Payload, &p)
	if len(p.Clients) != 1 || p.Clients[0].ID != directID {
		t.Fatal("cross-class shrink")
	}
	if _, ok := state.clients["manual"]; !ok {
		t.Fatal("manual client removed")
	}
	if len(state.clients) != 2 || planLife(t, j, rt) {
		t.Fatal("shrink failed to settle")
	}
}
func TestProfileChangedDuringQueuedCreateIsSuperseded(t *testing.T) {
	db, j, rt, state := profileFixture(t)
	if !planLife(t, j, rt) {
		t.Fatal("plan missing")
	}
	db.Exec(`UPDATE reality_config_profiles SET user_quota_bytes=999,revision=revision+1 WHERE route_class='DIRECT'`)
	job, ok, err := j.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); !errors.Is(err, ErrLifecycleSuperseded) {
		t.Fatal(err)
	}
	if state.postCreates != 0 {
		t.Fatal("obsolete profile posted")
	}
	if err = j.supersedeLifecycle(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("replacement plan missing")
	}
	runLife(t, j, rt)
}
func TestProfileRetryRecoversOnlyMissingIdentities(t *testing.T) {
	db, j, rt, state := profileFixture(t)
	db.Exec(`UPDATE reality_config_profiles SET target_users_per_inbound=3 WHERE route_class='DIRECT'`)
	if !planLife(t, j, rt) {
		t.Fatal("plan missing")
	}
	state.partialCreate = true
	job, ok, err := j.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job)
	if err == nil {
		t.Fatal("expected lost response")
	}
	// Recovery of the same immutable job observes committed clients first.
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	if len(state.clients) != 4 {
		t.Fatal("duplicate or missing client", len(state.clients))
	}
	if len(state.createSizes) != 2 || state.createSizes[0] != 3 || state.createSizes[1] != 2 {
		t.Fatal("blind batch retry", state.createSizes)
	}
}

func TestOtherProfileEditDoesNotInvalidateDurablePlan(t *testing.T) {
	db, j, rt, state := profileFixture(t)
	if !planLife(t, j, rt) {
		t.Fatal("plan missing")
	}
	if _, err := db.Exec(`UPDATE reality_config_profiles SET user_quota_bytes=999,revision=revision+1 WHERE route_class='RESIDENTIAL'`); err != nil {
		t.Fatal(err)
	}
	job := runLife(t, j, rt)
	var p BulkPayload
	json.Unmarshal(job.Payload, &p)
	if p.Policy.Class != "DIRECT" || len(state.clients) != 2 {
		t.Fatal("other profile invalidated plan")
	}
}
