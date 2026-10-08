package serverprotection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func pollingDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, e := url.Parse(dsn)
	if e != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := sanaei.UUIDv4()
	schema := "protection_" + strings.ReplaceAll(id, "-", "")
	if _, e = admin.Exec("CREATE SCHEMA " + schema); e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := sql.Open("postgres", u.String())
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(10)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	pollingExec(t, db, `CREATE TABLE accounts(id uuid PRIMARY KEY,enabled boolean,provider_state text,deletion_requested_at timestamptz);
 CREATE TABLE droplets(id uuid PRIMARY KEY,state text,expires_at timestamptz);
 CREATE TABLE panel_instances(id uuid PRIMARY KEY,account_id uuid,droplet_id uuid,base_url text,enabled boolean);
 CREATE TABLE deployments(droplet_id uuid,state text,host text,profile_snapshot jsonb);
 CREATE TABLE panel_inventory_syncs(panel_id uuid,finished_at timestamptz,id uuid,state text);
 INSERT INTO accounts VALUES('00000000-0000-4000-8000-000000000001',true,'ACTIVE',NULL);`)
	for _, f := range []string{"../../migrations/000157_server_protection.up.sql", "../../migrations/000159_verified_protection_receipts.up.sql"} {
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		pollingExec(t, db, string(raw))
	}
	pollingExec(t, db, "UPDATE server_protection_control SET enabled=true,scope='fleet'")
	return db
}
func pollingExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := db.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}
func pollingSeed(t *testing.T, db *sql.DB, n int, enabled bool) string {
	t.Helper()
	id := fmt.Sprintf("10000000-0000-4000-8000-%012d", n)
	expiry := time.Now().Add(time.Hour)
	key := "enabled"
	if !enabled {
		expiry = time.Now().Add(-time.Hour)
		key = "cleanup"
	}
	pollingExec(t, db, "INSERT INTO droplets VALUES($1,'READY',$2)", id, expiry)
	pollingExec(t, db, "INSERT INTO panel_instances VALUES($1,'00000000-0000-4000-8000-000000000001',$1,'http://fixture.invalid:2053',true)", id)
	pollingExec(t, db, "INSERT INTO deployments VALUES($1::uuid,'PANEL_COMPLETE',$1::text,jsonb_build_object('ssh_user','root','ssh_key_secret_ref',$2::text))", id, key)
	pollingExec(t, db, "INSERT INTO server_protection_nodes(panel_id,desired_revision,control_revision,desired_enabled,next_check_at) VALUES($1,1,1,$2,now()-interval '1 minute')", id, enabled)
	return id
}

type blockingPollSecrets struct{ enabled chan struct{} }

func (s blockingPollSecrets) Get(ctx context.Context, account, key string) ([]byte, error) {
	if key == "cleanup" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case s.enabled <- struct{}{}:
	default:
	}
	return nil, errors.New("fixture observation; no remote connection")
}
func TestProtectionEnabledLoopProgressesBehindCleanupTimeouts(t *testing.T) {
	db := pollingDB(t)
	for i := 1; i <= 65; i++ {
		pollingSeed(t, db, i, false)
	}
	pollingSeed(t, db, 100, true)
	seen := make(chan struct{}, 1)
	budget := worker.NewWorkBudget(10)
	c := Controller{DB: db, Secrets: blockingPollSecrets{seen}, Admit: budget.Do, SSH: provisioning.SSHClient{}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("controller did not join canceled work")
		}
		if budget.Snapshot()["active"] != 0 {
			t.Error("admission leaked")
		}
	}()
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("enabled reconciliation starved behind disabled cleanup")
	}
}
