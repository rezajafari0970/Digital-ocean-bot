package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func seedBuildAccount(t *testing.T, db *sql.DB, provider string) string {
	id := perfUUID()
	sqlMust(t, db, `INSERT INTO accounts(id,name,provider,secret_ref,enabled,runtime_status,provider_state,provider_checked_at,desired_server_count) VALUES($1,'capacity fixture',$2,'fixture',true,'READY','ACTIVE',now(),5)`, id, provider)
	sqlMust(t, db, `INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')`, id)
	return id
}
func putBuildSnapshot(t *testing.T, db *sql.DB, id, status string, known bool, limit, inuse int) {
	raw, _ := json.Marshal(providers.Observation{Account: providers.Account{Status: status}, Capacity: providers.Capacity{LimitKnown: known, ComputeLimit: limit, ComputeInUse: inuse, PlanAvailable: map[string]int{"small": 5, "large": 2}}})
	sqlMust(t, db, "DELETE FROM provider_snapshots WHERE account_id=$1", id)
	sqlMust(t, db, "INSERT INTO provider_snapshots(id,account_id,provider,version,data,canonical) SELECT gen_random_uuid(),id,provider,2,'{}',$2 FROM accounts WHERE id=$1", id, raw)
}
func TestAccountBuildParityAndReservationsPostgres(t *testing.T) {
	db := adminTestDB(t)
	s := Server{DB: db}
	ctx := context.Background()
	for _, provider := range []string{"upcloud", "digitalocean", "vultr"} {
		id := seedBuildAccount(t, db, provider)
		putBuildSnapshot(t, db, id, "active", true, 3, 1)
		check := func(want *int, providerOK bool) {
			t.Helper()
			v, e := s.accountBuildState(ctx, id)
			if e != nil {
				t.Fatal(e)
			}
			if (v.Buildable == nil) != (want == nil) || (want != nil && *v.Buildable != *want) || v.ProviderCanCreate != providerOK {
				t.Fatalf("%s state=%+v build=%v want=%v", provider, v, v.Buildable, want)
			}
			list := httptest.NewRecorder()
			s.accounts(list, httptest.NewRequest("GET", "/", nil))
			if list.Code != 200 {
				t.Fatal(list.Code, list.Body.String())
			}
			var rows []map[string]any
			json.Unmarshal(list.Body.Bytes(), &rows)
			var row map[string]any
			for _, a := range rows {
				if a["id"] == id {
					row = a
				}
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.SetPathValue("id", id)
			s.accountDashboard(rr, req)
			if rr.Code != 200 {
				t.Fatal(rr.Code, rr.Body.String())
			}
			var d accountDashboard
			json.Unmarshal(rr.Body.Bytes(), &d)
			if row["buildable_now"] != d.Capacity["buildable_now"] || row["provider_can_create"] != d.Account["provider_can_create"] || row["scheduler_can_build"] != d.Account["scheduler_can_build"] || row["provider_reason"] != d.Account["provider_reason"] {
				t.Fatal("list/detail disagree", row, d)
			}
			if d.Capacity["available"] != row["droplet_available"] {
				t.Fatal("inventory capacity mismatch", d.Capacity, row)
			}
		}
		two, one, zero := 2, 1, 0
		check(&two, true)
		sqlMust(t, db, "INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET',$1::uuid::text,'running')", id)
		check(&one, true)
		sqlMust(t, db, "UPDATE operations SET state='failed' WHERE account_id=$1", id)
		check(&two, true)
		sqlMust(t, db, "UPDATE accounts SET desired_server_count=1 WHERE id=$1", id)
		check(&zero, true)
		sqlMust(t, db, "UPDATE accounts SET desired_server_count=5,enabled=false WHERE id=$1", id)
		check(&zero, false)
		sqlMust(t, db, "UPDATE accounts SET enabled=true WHERE id=$1", id)
		sqlMust(t, db, "UPDATE provider_snapshots SET created_at=now()-interval '6 minutes' WHERE account_id=$1", id)
		check(nil, false)
		putBuildSnapshot(t, db, id, "active", true, 0, 0)
		check(&zero, false)
		putBuildSnapshot(t, db, id, "active", false, 0, 0)
		check(nil, provider != "upcloud")
		putBuildSnapshot(t, db, id, "active", true, 3, 1)
	}
}
func trialDenied() error {
	return &providers.Error{Class: providers.ErrorPermissionDenied, Operation: "create_server", StatusCode: 403, Code: "TRIAL_FIREWALL"}
}
func TestCreateBlockRetainsRefreshAndVersionedRecoveryPostgres(t *testing.T) {
	db := adminTestDB(t)
	s := Server{DB: db}
	ctx := context.Background()
	id := seedBuildAccount(t, db, "upcloud")
	putBuildSnapshot(t, db, id, "active", true, 2, 0)
	c := app.Container{DB: db}
	if e := capacity.RecordCreateBlock(ctx, db, id, trialDenied()); e != nil {
		t.Fatal(e)
	}
	old, e := capacity.ReadCreateBlock(ctx, db, id)
	if e != nil || old == nil {
		t.Fatal(old, e)
	}
	c.RecordProviderObservation(ctx, id, app.ProviderStateActive, nil, `{"can_create":true}`)
	if _, e = capacity.Read(ctx, db, id, 2*time.Minute); !errors.Is(e, capacity.ErrCreateBlocked) {
		t.Fatal("refresh cleared block", e)
	}
	v, e := s.accountBuildState(ctx, id)
	if e != nil || v.Buildable == nil || *v.Buildable != 0 || v.ProviderCanCreate || v.Available == nil || *v.Available != 2 || !strings.Contains(v.Reason, "TRIAL_FIREWALL") {
		t.Fatal(v, e)
	}
	release := func(role auth.Role, version int64, resolved bool) int {
		rr := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"version":%d,"restriction_resolved":%t}`, version, resolved)))
		r.SetPathValue("id", id)
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Username: "test-admin", Role: role}))
		s.releaseAccountCreateBlock(rr, r)
		return rr.Code
	}
	if release(auth.Operator, old.Version, true) != 403 || release(auth.Admin, old.Version, false) != 400 {
		t.Fatal("authorization/confirmation missing")
	}
	if e = capacity.RecordCreateBlock(ctx, db, id, trialDenied()); e != nil {
		t.Fatal(e)
	}
	latest, _ := capacity.ReadCreateBlock(ctx, db, id)
	if latest.Version <= old.Version || release(auth.Admin, old.Version, true) != 409 {
		t.Fatal("stale release accepted")
	}
	putBuildSnapshot(t, db, id, "trial_restricted", true, 2, 0)
	if release(auth.Admin, latest.Version, true) != 409 {
		t.Fatal("known trial released")
	}
	putBuildSnapshot(t, db, id, "active", true, 2, 0)
	if release(auth.Admin, latest.Version, true) != 200 {
		t.Fatal("valid release failed")
	}
	if b, e := capacity.ReadCreateBlock(ctx, db, id); e != nil || b != nil {
		t.Fatal(b, e)
	}
	if _, e := capacity.Read(ctx, db, id, 2*time.Minute); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM audit_events WHERE account_id=$1 AND action='create_permission_retry_enabled' AND actor='test-admin'", id).Scan(&n)
	if n != 1 {
		t.Fatal("missing recovery audit", n)
	}
	// A fresh trial observation blocks first creation even with no historical 403.
	putBuildSnapshot(t, db, id, "trial_restricted", true, 2, 0)
	if _, e := capacity.Read(ctx, db, id, 2*time.Minute); !errors.Is(e, capacity.ErrCreateBlocked) {
		t.Fatal("new trial was admitted")
	}
	v, e = s.accountBuildState(ctx, id)
	if e != nil || v.ProviderCanCreate || v.Buildable == nil || *v.Buildable != 0 {
		t.Fatal(v, e)
	}
	// Non-create/non-UpCloud/temporary errors cannot establish a sticky permission block.
	for _, err := range []error{&providers.Error{Class: providers.ErrorPermissionDenied, Operation: "account", StatusCode: 403, Code: "TRIAL_FIREWALL"}, &providers.Error{Class: providers.ErrorRateLimited, Operation: "create_server", StatusCode: 429, Code: "RATE_LIMITED"}} {
		if e := capacity.RecordCreateBlock(ctx, db, id, err); e != nil {
			t.Fatal(e)
		}
	}
	if b, _ := capacity.ReadCreateBlock(ctx, db, id); b != nil {
		t.Fatal(b)
	}
}
func TestUpCloudCreateBlockMigrationBackfillPostgres(t *testing.T) {
	db := adminTestDBThrough(t, 153)
	id := seedBuildAccount(t, db, "upcloud")
	profile := perfUUID()
	sqlMust(t, db, "INSERT INTO deployment_profiles(id,account_id,name,config) VALUES($1,$2,'test','{}')", profile, id)
	sqlMust(t, db, `INSERT INTO deployments(id,account_id,profile_id,state,current_step,last_error) VALUES(gen_random_uuid(),$1,$2,'FAILED','CREATE_SERVER','provider permission_denied: UpCloud HTTP 403 (TRIAL_FIREWALL)')`, id, profile)
	up, e := os.ReadFile("../../migrations/000154_upcloud_create_block.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, string(up))
	sqlMust(t, db, string(up))
	b, e := capacity.ReadCreateBlock(context.Background(), db, id)
	if e != nil || b == nil || b.Code != "TRIAL_FIREWALL" {
		t.Fatal(b, e)
	}
	down, e := os.ReadFile("../../migrations/000154_upcloud_create_block.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(down)); e == nil {
		t.Fatal("unsafe downgrade allowed")
	}
	sqlMust(t, db, "DELETE FROM account_create_blocks")
	sqlMust(t, db, string(down))
}

func TestCreateBlockAuditFaultFailsClosedPostgres(t *testing.T) {
	db := adminTestDB(t)
	id := seedBuildAccount(t, db, "upcloud")
	putBuildSnapshot(t, db, id, "active", true, 2, 0)
	sqlMust(t, db, "CREATE FUNCTION deny_create_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fault'; END $$")
	sqlMust(t, db, "CREATE TRIGGER create_audit_fault BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION deny_create_audit()")
	ctx := context.Background()
	if err := capacity.RecordCreateBlock(ctx, db, id, trialDenied()); err == nil {
		t.Fatal("audit fault not reported")
	}
	b, err := capacity.ReadCreateBlock(ctx, db, id)
	if err != nil || b == nil {
		t.Fatal("audit fault undid denial", b, err)
	}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"version":%d,"restriction_resolved":true}`, b.Version)))
	r.SetPathValue("id", id)
	r = r.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Username: "test-admin", Role: auth.Admin}))
	s := Server{DB: db}
	s.releaseAccountCreateBlock(rr, r)
	if rr.Code != 500 {
		t.Fatal(rr.Code)
	}
	kept, err := capacity.ReadCreateBlock(ctx, db, id)
	if err != nil || kept == nil || kept.Version != b.Version {
		t.Fatal("unaudited release succeeded", kept, err)
	}
	if _, err := capacity.Read(ctx, db, id, time.Minute); !errors.Is(err, capacity.ErrCreateBlocked) {
		t.Fatal(err)
	}
}
