package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func trialEnable(s *Server, id string, version int64, role auth.Role, accept bool) int {
	r := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"version":%d,"restricted_egress_accepted":%t}`, version, accept)))
	r.SetPathValue("id", id)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Username: "trial-test", Role: role}))
	w := httptest.NewRecorder()
	s.enableUpCloudTrial(w, r)
	return w.Code
}
func trialFixture(t *testing.T) (*sql.DB, *Server, string, int64) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	id := seedBuildAccount(t, db, "upcloud")
	sqlMust(t, db, "INSERT INTO global_config_policies(policy_key,ports) VALUES('reality','[443]')")
	putBuildSnapshot(t, db, id, "trial_restricted", true, 2, 0)
	if err := capacity.RecordCreateBlock(context.Background(), db, id, trialDenied()); err != nil {
		t.Fatal(err)
	}
	b, err := capacity.ReadCreateBlock(context.Background(), db, id)
	if err != nil {
		t.Fatal(err)
	}
	return db, s, id, b.Version
}
func TestUpCloudTrialAtomicEnableAndNewDenial(t *testing.T) {
	db, s, id, v := trialFixture(t)
	if trialEnable(s, id, v, auth.Operator, true) != 403 || trialEnable(s, id, v, auth.Admin, false) != 400 || trialEnable(s, id, v+1, auth.Admin, true) != 409 {
		t.Fatal("authorization/version gate")
	}
	if got := trialEnable(s, id, v, auth.Admin, true); got != 200 {
		t.Fatal("enable", got)
	}
	st, e := s.accountBuildState(context.Background(), id)
	if e != nil || !st.TrialCompatible || !st.ProviderCanCreate || st.Buildable == nil || *st.Buildable != 2 || st.TrialProxyCandidates != 0 {
		t.Fatalf("%+v %v", st, e)
	}
	if _, e = capacity.Read(context.Background(), db, id, time.Minute); e != nil {
		t.Fatal(e)
	}
	// Both list and detail expose the restricted mode, not unrestricted permission.
	w := httptest.NewRecorder()
	s.accounts(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"upcloud_trial_compatible":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	var n int
	db.QueryRow("SELECT count(*) FROM audit_events WHERE account_id=$1 AND action='upcloud_trial_compatible_enabled'", id).Scan(&n)
	if n != 1 || trialEnable(s, id, v, auth.Admin, true) != 409 {
		t.Fatal("duplicate mode transition", n)
	}
	if e = capacity.RecordCreateBlock(context.Background(), db, id, trialDenied()); e != nil {
		t.Fatal(e)
	}
	if _, e = capacity.Read(context.Background(), db, id, time.Minute); !errors.Is(e, capacity.ErrCreateBlocked) {
		t.Fatal("new denial bypassed", e)
	}
	b, _ := capacity.ReadCreateBlock(context.Background(), db, id)
	if trialEnable(s, id, b.Version, auth.Admin, true) != 409 {
		t.Fatal("mode activation cleared a new denial")
	}
	(app.Container{DB: db}).RecordProviderObservation(context.Background(), id, app.ProviderStateActive, nil, "read succeeded")
	b, _ = capacity.ReadCreateBlock(context.Background(), db, id)
	if b == nil {
		t.Fatal("refresh removed denial")
	}
}
func TestUpCloudTrialRejectsIncompatibleTransitions(t *testing.T) {
	for _, kind := range []string{"provider", "disabled", "deleting", "other_code", "operation", "deployment", "ports"} {
		t.Run(kind, func(t *testing.T) {
			db, s, id, v := trialFixture(t)
			switch kind {
			case "provider":
				sqlMust(t, db, "UPDATE accounts SET provider='vultr' WHERE id=$1", id)
			case "disabled":
				sqlMust(t, db, "UPDATE accounts SET enabled=false WHERE id=$1", id)
			case "deleting":
				sqlMust(t, db, "UPDATE accounts SET deletion_requested_at=now() WHERE id=$1", id)
			case "other_code":
				sqlMust(t, db, "UPDATE account_create_blocks SET code='OTHER_PERMISSION' WHERE account_id=$1", id)
			case "operation":
				sqlMust(t, db, "INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','trial-operation','unknown')", id)
			case "deployment":
				profile := perfUUID()
				sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'trial','{}')", profile, id)
				sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create')", id, profile)
			case "ports":
				sqlMust(t, db, "UPDATE global_config_policies SET ports='[8443]'")
			}
			if got := trialEnable(s, id, v, auth.Admin, true); got != 409 {
				t.Fatal(kind, got)
			}
			var enabled bool
			db.QueryRow("SELECT upcloud_trial_compatible FROM accounts WHERE id=$1", id).Scan(&enabled)
			b, _ := capacity.ReadCreateBlock(context.Background(), db, id)
			if enabled || b == nil || b.Version != v {
				t.Fatal("rejection mutated state")
			}
		})
	}
}
func TestUpCloudTrialAuditFaultAndConcurrentEnable(t *testing.T) {
	db, s, id, v := trialFixture(t)
	sqlMust(t, db, "CREATE FUNCTION fail_trial_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'trial audit fault'; END $$")
	sqlMust(t, db, "CREATE TRIGGER trial_audit_fault BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_trial_audit()")
	if got := trialEnable(s, id, v, auth.Admin, true); got != 500 {
		t.Fatal(got)
	}
	var enabled bool
	db.QueryRow("SELECT upcloud_trial_compatible FROM accounts WHERE id=$1", id).Scan(&enabled)
	b, _ := capacity.ReadCreateBlock(context.Background(), db, id)
	if enabled || b == nil || b.Version != v {
		t.Fatal("audit failure not atomic")
	}
	sqlMust(t, db, "DROP TRIGGER trial_audit_fault ON audit_events")
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- trialEnable(s, id, v, auth.Admin, true) }()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for c := range results {
		counts[c]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
}

type trialPanelIO struct{ portSeen bool }

func (x *trialPanelIO) Put(context.Context, string, string, string, []byte) error { return nil }
func (x *trialPanelIO) Get(context.Context, string, string) ([]byte, error) {
	return []byte("fixture-only"), nil
}
func (x *trialPanelIO) Run(context.Context, provisioning.Target, []byte, string) (string, error) {
	return "", nil
}
func (x *trialPanelIO) Upload(_ context.Context, _ provisioning.Target, _ []byte, local, _ string, _ os.FileMode) error {
	b, e := os.ReadFile(local)
	if e == nil {
		x.portSeen = strings.Contains(string(b), "XUI_PORT=3389\n")
	}
	return e
}
func TestUpCloudTrialSnapshotPinsCreateAndPanelPort(t *testing.T) {
	db, s, id, v := trialFixture(t)
	if trialEnable(s, id, v, auth.Admin, true) != 200 {
		t.Fatal("enable")
	}
	profile, dep, droplet := perfUUID(), perfUUID(), perfUUID()
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'trial','{}')", profile, id)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES($1,$2,'trial-id','READY')", droplet, id)
	sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step) VALUES($1,$2,$3,$4,'READY','done')", dep, id, profile, droplet)
	store := workflow.ProfileStore{DB: db}
	if err := store.AttachSnapshot(context.Background(), dep, workflow.ProfileSnapshot{Name: "immutable"}); err != nil {
		t.Fatal(err)
	}
	// Changing account preferences cannot reinterpret a previously admitted create.
	sqlMust(t, db, "UPDATE accounts SET upcloud_trial_compatible=false WHERE id=$1", id)
	if err := store.AttachSnapshot(context.Background(), dep, workflow.ProfileSnapshot{Name: "overwritten"}); err != nil {
		t.Fatal(err)
	}
	snap, e := store.SnapshotForDeployment(context.Background(), dep)
	if e != nil || !snap.UpCloudTrialCompatible || snap.Name != "immutable" {
		t.Fatal(snap, e)
	}
	cfg, _, e := (app.Container{DB: db}).DeploymentConfigFromSnapshot(context.Background(), dep)
	if e != nil || !cfg.Profile.UpCloudTrialCompatible {
		t.Fatal("create lost immutable mode", e)
	}
	io := &trialPanelIO{}
	p := sanaei.PanelConfigurer{DB: db, Secrets: io, Runner: io, Uploader: io}
	if e = p.Configure(context.Background(), id, droplet, provisioning.Target{Host: "192.0.2.1"}); e != nil {
		t.Fatal(e)
	}
	var port int
	if e = db.QueryRow("SELECT port FROM xui_panel_deployments WHERE droplet_id=$1", droplet).Scan(&port); e != nil || port != 3389 || !io.portSeen {
		t.Fatal(port, e)
	}
	// A pre-existing incompatible ledger must never overwrite the pinned port.
	sqlMust(t, db, "UPDATE xui_panel_deployments SET port=2053 WHERE droplet_id=$1", droplet)
	if e = p.Configure(context.Background(), id, droplet, provisioning.Target{Host: "192.0.2.1"}); e == nil {
		t.Fatal("trial accepted incompatible existing panel ledger")
	}
	if e = p.RepairCompleted(context.Background(), id, droplet, provisioning.Target{Host: "192.0.2.1"}); e == nil {
		t.Fatal("repair accepted incompatible trial port")
	}
	// User profile JSON cannot opt a standard account into the mode.
	dep2 := perfUUID()
	sqlMust(t, db, "INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES($1,$2,$3,'FAILED','done')", dep2, id, profile)
	if e = store.AttachSnapshot(context.Background(), dep2, workflow.ProfileSnapshot{UpCloudTrialCompatible: true}); e != nil {
		t.Fatal(e)
	}
	snap, e = store.SnapshotForDeployment(context.Background(), dep2)
	if e != nil || snap.UpCloudTrialCompatible {
		t.Fatal("untrusted profile opt-in", e)
	}
	raw, _ := json.Marshal(snap)
	if len(raw) == 0 {
		t.Fatal("snapshot marshal")
	}
}

func TestUpCloudTrialDowngradeProtectsQueuedSnapshot(t *testing.T) {
	db, _, id, _ := trialFixture(t)
	profile, dep := perfUUID(), perfUUID()
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'trial','{}')", profile, id)
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,state,current_step,profile_snapshot) VALUES($1,$2,$3,'PLANNED','create','{"upcloud_trial_compatible":true}')`, dep, id, profile)
	down, e := os.ReadFile("../../migrations/000160_upcloud_trial_compatible.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("downgrade lost an in-flight pinned mode")
	}
	sqlMust(t, db, "UPDATE deployments SET state='FAILED' WHERE id=$1", dep)
	sqlMust(t, db, string(down))
}
