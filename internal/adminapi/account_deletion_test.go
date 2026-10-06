package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountDeletionPurgesDataAndIsolatesOtherAccount(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	const a = "22222222-2222-4222-8222-222222222222"
	const b = "33333333-3333-4333-8333-333333333333"
	for _, id := range []string{a, b} {
		sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'test','digitalocean','test')", id)
		sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", id)
		sqlMust(t, db, "INSERT INTO account_billing_snapshots(account_id,data) VALUES($1,'{\"balance\":\"3.00\"}')", id)
	}
	s := Server{DB: db}
	req := httptest.NewRequest("DELETE", "/api/v1/accounts/"+a, nil)
	req.SetPathValue("id", a)
	req = req.WithContext(context.WithValue(ctx, principalKey{}, auth.Principal{Role: auth.Admin}))
	w := httptest.NewRecorder()
	s.deleteAccount(w, req)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM account_deletion_jobs WHERE account_id=$1", a).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	c := app.Container{DB: db}
	if err := c.ProcessAccountDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "network_profiles", "account_billing_snapshots", "account_deletion_jobs"} {
		col := "account_id"
		if table == "accounts" {
			col = "id"
		}
		if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE "+col+"=$1", a).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	if err := db.QueryRow("SELECT count(*) FROM account_billing_snapshots WHERE account_id=$1", b).Scan(&n); err != nil || n != 1 {
		t.Fatal("other account damaged", n, err)
	}
}
func TestAccountDeletionRetainsCredentialsWhileResourcesPending(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	const a = "22222222-2222-4222-8222-222222222222"
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,enabled,deletion_requested_at) VALUES($1,'test','digitalocean','test',false,now())", a)
	sqlMust(t, db, "INSERT INTO account_deletion_jobs(account_id) VALUES($1)", a)
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'test-server','RETIRING')", a)
	if err := (app.Container{DB: db}).ProcessAccountDeletions(ctx); err == nil {
		t.Fatal("must remain pending")
	}
	var state string
	if err := db.QueryRow("SELECT runtime_status FROM accounts WHERE id=$1", a).Scan(&state); err != nil || state != "DELETE_PENDING" {
		t.Fatal(state, err)
	}
}

type deletionFixtureDriver struct {
	providers.Driver
	providers.ComputeDriver
	providers.SSHKeyDriver
	provider string
	servers  []providers.Server
	timeout  bool
	lists    int
}

func (d *deletionFixtureDriver) Name() string { return d.provider }
func (d *deletionFixtureDriver) Capabilities() providers.Capabilities {
	return providers.Capabilities{Compute: true, InlineSSHKeys: d.provider == "upcloud"}
}
func (d *deletionFixtureDriver) ListServers(ctx context.Context) ([]providers.Server, error) {
	d.lists++
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 45*time.Second {
		return nil, errors.New("provider cleanup has no bounded deadline")
	}
	if d.timeout {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return d.servers, nil
}
func (d *deletionFixtureDriver) ListSSHKeys(context.Context) ([]providers.SSHKey, error) {
	return nil, nil
}

type deletionFixtureFactory struct{ d *deletionFixtureDriver }

func (f deletionFixtureFactory) Name() string                 { return f.d.provider }
func (f deletionFixtureFactory) Metadata() providers.Metadata { return providers.Metadata{} }
func (f deletionFixtureFactory) Open(context.Context, providers.OpenRequest) (providers.Driver, error) {
	return f.d, nil
}

func requestDeletion(s *Server, id string, role auth.Role) *httptest.ResponseRecorder {
	r := httptest.NewRequest("DELETE", "/api/v1/accounts/"+id, nil)
	r.SetPathValue("id", id)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, auth.Principal{Role: role}))
	w := httptest.NewRecorder()
	s.deleteAccount(w, r)
	return w
}
func TestAccountDeletionProviderVerificationAndTimeoutFault(t *testing.T) {
	for _, provider := range []string{"digitalocean", "vultr", "upcloud"} {
		t.Run(provider, func(t *testing.T) {
			db := adminTestDB(t)
			ctx := context.Background()
			a, d, _ := seedPanel(t, db, "http://fixture.invalid")
			sqlMust(t, db, "UPDATE accounts SET provider=$2 WHERE id=$1", a, provider)
			sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct')", a)
			sqlMust(t, db, "UPDATE droplets SET state='DELETED' WHERE id=$1", d)
			sqlMust(t, db, "INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('deletion-secret',$1,'api_token','test','test',1)", a)
			s := Server{DB: db}
			if w := requestDeletion(&s, a, auth.Admin); w.Code != 202 {
				t.Fatal(w.Code, w.Body.String())
			}
			fake := &deletionFixtureDriver{provider: provider}
			reg := providers.NewRegistry()
			if e := reg.Register(deletionFixtureFactory{fake}); e != nil {
				t.Fatal(e)
			}
			c := app.Container{DB: db, Accounts: app.Repository{DB: db}, Providers: reg}
			if e := c.ProcessAccountDeletions(ctx); e == nil {
				t.Fatal("safety settle was skipped")
			}
			if fake.lists != 0 {
				t.Fatal("provider checked before settling")
			}
			p := s.accountDeletionProgress(ctx, a).(map[string]any)
			if p["phase"] != "SETTLING" || p["remaining_servers"] != 0 {
				t.Fatal(p)
			}
			var until, request time.Time
			if e := db.QueryRow("SELECT requested_at,next_attempt_at FROM account_deletion_jobs WHERE account_id=$1", a).Scan(&request, &until); e != nil || !until.Equal(request.Add(2*time.Minute)) {
				t.Fatal("retry must be exact settle boundary", request, until, e)
			}
			// Duplicate retry keeps the original safety clock.
			if w := requestDeletion(&s, a, auth.Admin); w.Code != 202 {
				t.Fatal(w.Code)
			}
			var again time.Time
			db.QueryRow("SELECT requested_at FROM account_deletion_jobs WHERE account_id=$1", a).Scan(&again)
			if !again.Equal(request) {
				t.Fatal("retry resets settle")
			}
			sqlMust(t, db, "UPDATE account_deletion_jobs SET requested_at=now()-interval '3 minutes',next_attempt_at=now() WHERE account_id=$1", a)
			sqlMust(t, db, "UPDATE accounts SET deletion_requested_at=now()-interval '3 minutes' WHERE id=$1", a)
			fake.timeout = true
			timeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			e := c.ProcessAccountDeletions(timeout)
			cancel()
			if !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("timeout fixture did not reach provider", e)
			}
			var detail string
			var delayed bool
			if e := db.QueryRow("SELECT last_error,next_attempt_at>now() FROM account_deletion_jobs WHERE account_id=$1", a).Scan(&detail, &delayed); e != nil || !strings.Contains(detail, "timed out") || !delayed {
				t.Fatal(detail, delayed, e)
			}
			var n int
			db.QueryRow("SELECT count(*) FROM secrets WHERE account_id=$1", a).Scan(&n)
			if n != 1 {
				t.Fatal("timeout erased credential")
			}
			fake.timeout = false
			var pid string
			db.QueryRow("SELECT provider_resource_id FROM droplets WHERE id=$1", d).Scan(&pid)
			fake.servers = []providers.Server{{ID: pid}}
			sqlMust(t, db, "UPDATE account_deletion_jobs SET next_attempt_at=now() WHERE account_id=$1", a)
			if e := c.ProcessAccountDeletions(ctx); e == nil {
				t.Fatal("local DELETED must not replace provider absence")
			}
			fake.servers = nil
			sqlMust(t, db, "UPDATE account_deletion_jobs SET next_attempt_at=now() WHERE account_id=$1", a)
			if e := c.ProcessAccountDeletions(ctx); e != nil {
				t.Fatal(e)
			}
			db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", a).Scan(&n)
			if n != 0 {
				t.Fatal("verified account remains")
			}
			if w := requestDeletion(&s, a, auth.Admin); w.Code != 200 || !strings.Contains(w.Body.String(), "ABSENT") {
				t.Fatal("lost-response retry", w.Code, w.Body.String())
			}
		})
	}
}
func TestAccountDeletionSkipsLockedHeadAndKeepsCleanupNetwork(t *testing.T) {
	db := adminTestDB(t)
	ctx := context.Background()
	s := Server{DB: db}
	var ids []string
	for _, provider := range []string{"digitalocean", "vultr", "upcloud"} {
		id := perfUUID()
		ids = append(ids, id)
		sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'fixture',$2,'test')", id, provider)
		sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'proxy_required')", id)
		if w := requestDeletion(&s, id, auth.Admin); w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	// First account is locked by another worker; third is deliberately not due.
	sqlMust(t, db, "UPDATE account_deletion_jobs SET next_attempt_at=now()-interval '1 minute' WHERE account_id=$1", ids[0])
	sqlMust(t, db, "UPDATE account_deletion_jobs SET next_attempt_at=now()+interval '1 hour' WHERE account_id=$1", ids[2])
	c := app.Container{DB: db}
	maintained, e := c.AccountNetworkMaintenanceIDs(ctx)
	if e != nil || len(maintained) != 3 {
		t.Fatal("zero-server cleanup lost network", maintained, e)
	}
	conn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,713))", ids[0]); e != nil {
		t.Fatal(e)
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,713))", ids[0])
	if e = c.ProcessAccountDeletions(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", ids[1]).Scan(&n)
	if n != 0 {
		t.Fatal("locked head starved next provider")
	}
	db.QueryRow("SELECT attempts FROM account_deletion_jobs WHERE account_id=$1", ids[2]).Scan(&n)
	if n != 0 {
		t.Fatal("not-due job claimed")
	}
	// Legacy archive has no cleanup intent/job and must not be reactivated.
	sqlMust(t, db, "DELETE FROM account_deletion_jobs WHERE account_id=$1", ids[2])
	sqlMust(t, db, "UPDATE accounts SET deleted_at=now(),runtime_status='DELETED' WHERE id=$1", ids[2])
	maintained, e = c.AccountNetworkMaintenanceIDs(ctx)
	if e != nil || len(maintained) != 1 || maintained[0] != ids[0] {
		t.Fatal(maintained, e)
	}
}
func TestAccountDeletionConcurrentRetryAndRequestFault(t *testing.T) {
	db := adminTestDB(t)
	id := perfUUID()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'fixture','upcloud','test')", id)
	s := Server{DB: db}
	if w := requestDeletion(&s, id, auth.Viewer); w.Code != 403 {
		t.Fatal("non-admin deletion", w.Code)
	}
	sqlMust(t, db, "CREATE FUNCTION reject_deletion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$")
	sqlMust(t, db, "CREATE TRIGGER deletion_fault BEFORE INSERT ON account_deletion_jobs FOR EACH ROW EXECUTE FUNCTION reject_deletion()")
	if w := requestDeletion(&s, id, auth.Admin); w.Code != 500 {
		t.Fatal(w.Code)
	}
	var active bool
	db.QueryRow("SELECT enabled AND deletion_requested_at IS NULL FROM accounts WHERE id=$1", id).Scan(&active)
	if !active {
		t.Fatal("request partially committed")
	}
	sqlMust(t, db, "DROP TRIGGER deletion_fault ON account_deletion_jobs")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := requestDeletion(&s, id, auth.Admin)
			if w.Code != 202 {
				t.Errorf("duplicate %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	var n int
	db.QueryRow("SELECT count(*) FROM account_deletion_jobs WHERE account_id=$1", id).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := (app.Container{DB: db}).ProcessAccountDeletions(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", id).Scan(&n)
	if n != 0 {
		t.Fatal("concurrent worker did not finish")
	}
}

func TestAccountDeletionPendingOperationAndBoundedAdmission(t *testing.T) {
	db := adminTestDB(t)
	id := perfUUID()
	s := Server{DB: db}
	ctx := context.Background()
	sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref) VALUES($1,'fixture','digitalocean','test')", id)
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec("SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))", "account-mutation:"+id); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	w := requestDeletion(&s, id, auth.Admin)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" || time.Since(start) > 4*time.Second {
		t.Fatal("unbounded or hidden contention", w.Code, time.Since(start), w.Body.String())
	}
	var untouched bool
	if e = db.QueryRow("SELECT enabled AND deletion_requested_at IS NULL FROM accounts WHERE id=$1", id).Scan(&untouched); e != nil || !untouched {
		t.Fatal("busy request partially applied", untouched, e)
	}
	tx.Rollback()
	sqlMust(t, db, "INSERT INTO operations(id,account_id,kind,idempotency_key,state) VALUES(gen_random_uuid(),$1,'CREATE_DROPLET','unknown-fixture','unknown')", id)
	if w = requestDeletion(&s, id, auth.Admin); w.Code != 202 {
		t.Fatal(w.Code)
	}
	if e = (app.Container{DB: db}).ProcessAccountDeletions(ctx); e == nil {
		t.Fatal("unknown create outcome erased")
	}
	p := s.accountDeletionProgress(ctx, id).(map[string]any)
	if p["phase"] != "CLEANUP" || p["pending_operations"] != 1 || p["remaining_servers"] != 0 {
		t.Fatal("zero servers hides unknown operation", p)
	}
}

func TestAccountDeletionSkipsWholeLockedPage(t *testing.T) {
	db := adminTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	var ids []string
	defer func() {
		for _, id := range ids {
			conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1,713))", id)
		}
	}()
	for i := 0; i < 34; i++ {
		id := perfUUID()
		sqlMust(t, db, "INSERT INTO accounts(id,name,provider,secret_ref,enabled,deletion_requested_at) VALUES($1,'fixture','upcloud','test',false,now())", id)
		sqlMust(t, db, "INSERT INTO account_deletion_jobs(account_id,next_attempt_at) VALUES($1,now()-interval '1 minute')", id)
		if i < 33 {
			if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,713))", id); e != nil {
				t.Fatal(e)
			}
			ids = append(ids, id)
		} else {
			if e = (app.Container{DB: db}).ProcessAccountDeletions(ctx); e != nil {
				t.Fatal(e)
			}
			var n int
			db.QueryRow("SELECT count(*) FROM accounts WHERE id=$1", id).Scan(&n)
			if n != 0 {
				t.Fatal("locked page starved later account")
			}
		}
	}
}

func TestAccountDeletionReportsDistinctResources(t *testing.T) {
	db := adminTestDB(t)
	a, _, _ := seedPanel(t, db, "http://fixture.invalid")
	var pid string
	if e := db.QueryRow("SELECT provider_resource_id FROM droplets WHERE account_id=$1", a).Scan(&pid); e != nil {
		t.Fatal(e)
	}
	for _, resource := range []string{pid, "another-resource"} {
		sqlMust(t, db, "INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'digitalocean',$2,'server','active',true)", a, resource)
	}
	sqlMust(t, db, "INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'third-resource','READY')", a)
	s := Server{DB: db}
	w := requestDeletion(&s, a, auth.Admin)
	var result struct {
		Remaining int `json:"remaining_resources"`
		Progress  struct {
			Remaining int `json:"remaining_servers"`
		} `json:"deletion"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || w.Code != 202 || result.Remaining != 3 || result.Progress.Remaining != 3 {
		t.Fatal(w.Code, w.Body.String(), e)
	}
}
