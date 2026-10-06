package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func installerBootstrapDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var suffix string
	if err = admin.QueryRow("SELECT replace(gen_random_uuid()::text,'-','')").Scan(&suffix); err != nil {
		t.Fatal(err)
	}
	schema := "installer_bootstrap_" + suffix
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	dir := t.TempDir()
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		raw, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		raw = []byte(strings.ReplaceAll(string(raw), "tc.table_schema='public'", "tc.table_schema=current_schema()"))
		if e = os.WriteFile(filepath.Join(dir, filepath.Base(f)), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err = (migrate.Runner{DB: db, Dir: dir}).Up(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

type bootstrapFixture struct {
	d    workflow.Deployment
	run  string
	plan provisioning.Plan
}

func newBootstrapFixture(t *testing.T, db *sql.DB) bootstrapFixture {
	t.Helper()
	var f bootstrapFixture
	f.plan = provisioning.Plan{Scripts: []provisioning.ScriptStep{
		{Name: "first", Category: "bootstrap", Execute: "prepare-first", Precheck: "test-first", MaxAttempts: 3},
		{Name: "bootstrap-kernel-memory-guard", Category: "bootstrap", Execute: "prepare-guard", Precheck: "test-guard", MaxAttempts: 3},
		{Name: "panel", Category: "install", Execute: "true"},
	}}
	must := func(q string, args []any, out *string) {
		t.Helper()
		if err := db.QueryRow(q, args...).Scan(out); err != nil {
			t.Fatal(err)
		}
	}
	must("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'vultr','bootstrap-fixture','unused') RETURNING id::text", nil, &f.d.AccountID)
	must("INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'bootstrap','{}') RETURNING id::text", []any{f.d.AccountID}, &f.d.ProfileID)
	must("INSERT INTO droplets(id,account_id,provider_resource_id,state) VALUES(gen_random_uuid(),$1,'fixture-resource','PROVISIONING') RETURNING id::text", []any{f.d.AccountID}, &f.d.DropletID)
	raw, err := json.Marshal(workflow.ProfileSnapshot{InstallSteps: f.plan.Scripts, InstallerRef: &provisioning.InstallerRef{Name: "fixture", Version: 1}})
	if err != nil {
		t.Fatal(err)
	}
	must("INSERT INTO deployments(id,account_id,profile_id,droplet_id,state,current_step,profile_snapshot) VALUES(gen_random_uuid(),$1,$2,$3,'WAITING_INSTALLER','provision',$4) RETURNING id::text", []any{f.d.AccountID, f.d.ProfileID, f.d.DropletID, string(raw)}, &f.d.ID)
	f.d, err = (workflow.SQLStore{DB: db}).Get(context.Background(), f.d.ID, f.d.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	must("INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step,attempt) VALUES(gen_random_uuid(),$1,$2,'WAITING_INSTALLER','panel',4) RETURNING id::text", []any{f.d.AccountID, f.d.DropletID}, &f.run)
	return f
}
func bootstrapComplete(t *testing.T, db *sql.DB, f bootstrapFixture, steps ...string) {
	t.Helper()
	s := provisioning.SQLStore{DB: db}
	for _, step := range steps {
		if _, err := s.BeginStep(context.Background(), f.run, step, 10); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishStep(context.Background(), f.run, step, nil, false); err != nil {
			t.Fatal(err)
		}
	}
}
func execBootstrap(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
func TestInstallerBootstrapRecoveryPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	t.Run("real-activation-gate-before-network", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		bootstrapComplete(t, db, f, "ssh", "readiness")
		if err := (RecoveryHandler{Container: c}).RecoverDeployment(ctx, worker.RecoveryItem{ID: f.d.ID, AccountID: f.d.AccountID}); err != nil {
			t.Fatal(err)
		}
		var state, current string
		var attempt int
		if err := db.QueryRow("SELECT state,current_step,attempt FROM provision_runs WHERE id=$1", f.run).Scan(&state, &current, &attempt); err != nil {
			t.Fatal(err)
		}
		if state != "PENDING" || current != "first" || attempt != 4 {
			t.Fatalf("state=%s step=%s attempt=%d", state, current, attempt)
		}
		d, err := (workflow.SQLStore{DB: db}).Get(ctx, f.d.ID, f.d.AccountID)
		if err != nil || d.State != workflow.Provisioning || d.LockVersion != f.d.LockVersion+1 {
			t.Fatalf("deployment=%+v err=%v", d, err)
		}
	})
	t.Run("backoff-and-terminal-evidence-retained", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		bootstrapComplete(t, db, f, "ssh", "readiness")
		s := provisioning.SQLStore{DB: db}
		if _, err := s.BeginStep(ctx, f.run, "first", 3); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishStep(ctx, f.run, "first", errors.New("temporary"), false); err != nil {
			t.Fatal(err)
		}
		ok, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if _, err = s.BeginStep(ctx, f.run, "first", 3); !errors.Is(err, provisioning.ErrStepRetryDeferred) {
			t.Fatalf("backoff lost: %v", err)
		}
		execBootstrap(t, db, "UPDATE provision_step_attempts SET terminal=true WHERE run_id=$1 AND step='first'", f.run)
		if _, err = s.BeginStep(ctx, f.run, "first", 3); !errors.Is(err, provisioning.ErrStepTerminal) {
			t.Fatalf("terminal lost: %v", err)
		}
	})
	t.Run("all-presteps-required", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		bootstrapComplete(t, db, f, "ssh", "readiness", "first", "bootstrap-kernel-memory-guard")
		ok, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
	t.Run("failed-run-never-revived", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		execBootstrap(t, db, "UPDATE provision_runs SET state='FAILED' WHERE id=$1", f.run)
		if _, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan); !errors.Is(err, provisioning.ErrStepTerminal) {
			t.Fatalf("err=%v", err)
		}
	})
	for _, resource := range []string{"RETIRING", "DELETING", "DELETED", "READY"} {
		t.Run(resource, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, "UPDATE droplets SET state=$2 WHERE id=$1", f.d.DropletID, resource)
			if _, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan); !errors.Is(err, workflow.ErrDeploymentVersionConflict) {
				t.Fatal(err)
			}
		})
	}
	t.Run("deleting-account", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		execBootstrap(t, db, "UPDATE accounts SET deletion_requested_at=now() WHERE id=$1", f.d.AccountID)
		if _, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan); !errors.Is(err, workflow.ErrDeploymentVersionConflict) {
			t.Fatal(err)
		}
	})
	t.Run("transaction-rollback-on-event-fault", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		execBootstrap(t, db, "CREATE FUNCTION reject_bootstrap_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected-event-fault'; END; $$")
		execBootstrap(t, db, "CREATE TRIGGER reject_bootstrap_event BEFORE INSERT ON deployment_events FOR EACH ROW EXECUTE FUNCTION reject_bootstrap_event()")
		_, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
		execBootstrap(t, db, "DROP TRIGGER reject_bootstrap_event ON deployment_events")
		if err == nil {
			t.Fatal("fault not reached")
		}
		var state, step string
		if err = db.QueryRow("SELECT state,current_step FROM provision_runs WHERE id=$1", f.run).Scan(&state, &step); err != nil {
			t.Fatal(err)
		}
		if state != "WAITING_INSTALLER" || step != "panel" {
			t.Fatal("partial repair escaped rollback")
		}
		ok, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
		if err != nil || ok {
			t.Fatal(err)
		}
	})
	t.Run("concurrent-repair-single-CAS", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		var wg sync.WaitGroup
		var wins atomic.Int32
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
				if err == nil && !ok {
					wins.Add(1)
				} else if !errors.Is(err, workflow.ErrDeploymentVersionConflict) {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("unexpected %v", err)
		}
		if wins.Load() != 1 {
			t.Fatalf("wins=%d", wins.Load())
		}
	})
	t.Run("activation-shares-provisioning-lease", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		release, err := (workflow.PostgresRunLease{DB: db}).Acquire(ctx, f.d.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err = c.activateInstaller(ctx, f.d, workflow.ProfileSnapshot{}); !errors.Is(err, workflow.ErrDeploymentBusy) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("one-repair-reboot-only-after-guard-proof", func(t *testing.T) {
		f := newBootstrapFixture(t, db)
		bootstrapComplete(t, db, f, "ssh", "readiness", "first")
		execBootstrap(t, db, "INSERT INTO installer_reboot_remediations(deployment_id,generation,state,scheduled_at) VALUES($1,1,'SCHEDULED',now()-interval '10 minutes')", f.d.ID)
		ok, err := c.reconcileInstallerBootstrap(ctx, f.d, f.plan)
		if err != nil || ok {
			t.Fatal(err)
		}
		if ok, err = c.claimBootstrapRepairReboot(ctx, f.d, 1, "11111111-1111-4111-8111-111111111111"); err != nil || ok {
			t.Fatal("reboot before guard", err)
		}
		bootstrapComplete(t, db, f, "bootstrap-kernel-memory-guard")
		execBootstrap(t, db, "UPDATE deployments SET state='WAITING_INSTALLER' WHERE id=$1", f.d.ID)
		var wins atomic.Int32
		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := c.claimBootstrapRepairReboot(ctx, f.d, 1, "11111111-1111-4111-8111-111111111111")
				if err != nil {
					errs <- err
				}
				if ok {
					wins.Add(1)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
		if wins.Load() != 1 {
			t.Fatalf("claims=%d", wins.Load())
		}
		var marker string
		if err = db.QueryRow("SELECT last_error FROM installer_reboot_remediations WHERE deployment_id=$1", f.d.ID).Scan(&marker); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(marker, bootstrapRepairReboot+";") || !strings.Contains(marker, "previous_scheduled_at=") {
			t.Fatal("lost old reboot evidence")
		}
	})
}

func TestInstallerBootstrapRebootDispatchFaultPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	c := Container{DB: db}
	f := newBootstrapFixture(t, db)
	ssh := &bootstrapRebootSSH{fail: true}
	err := c.remediateInstallerReboot(ctx, f.d, 1, provisioning.Target{}, nil, ssh)
	if err == nil || errors.Is(err, ErrInstallerRebootScheduled) {
		t.Fatal("lost response not surfaced")
	}
	var marker string
	var scheduled time.Time
	if err = db.QueryRow("SELECT last_error,scheduled_at FROM installer_reboot_remediations WHERE deployment_id=$1", f.d.ID).Scan(&marker, &scheduled); err != nil {
		t.Fatal(err)
	}
	if installerRebootBootID(marker) != fixtureBootID {
		t.Fatal("lost durable boot proof")
	}
	ssh.fail = false
	if err = c.remediateInstallerReboot(ctx, f.d, 1, provisioning.Target{}, nil, ssh); !errors.Is(err, ErrInstallerRebootScheduled) {
		t.Fatal(err)
	}
	if ssh.reads != 1 || ssh.sends != 2 || ssh.commands[1] != ssh.commands[2] {
		t.Fatal("retry did not reconcile original boot")
	}
	var after time.Time
	if err = db.QueryRow("SELECT scheduled_at FROM installer_reboot_remediations WHERE deployment_id=$1", f.d.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !scheduled.Equal(after) {
		t.Fatal("retry reset budget")
	}
	// Crash after a durable claim but before sending any SSH command.
	g := newBootstrapFixture(t, db)
	execBootstrap(t, db, "INSERT INTO installer_reboot_remediations(deployment_id,generation,state,last_error) VALUES($1,1,'SCHEDULED',$2)", g.d.ID, "INSTALLER_REBOOT;boot_id="+fixtureBootID)
	n := ssh.sends
	if err = c.remediateInstallerReboot(ctx, g.d, 1, provisioning.Target{}, nil, ssh); !errors.Is(err, ErrInstallerRebootScheduled) || ssh.sends != n+1 {
		t.Fatal("undispatched intent stranded", err)
	}
	execBootstrap(t, db, "UPDATE installer_reboot_remediations SET scheduled_at=now()-interval '4 minutes' WHERE deployment_id=$1", g.d.ID)
	n = ssh.sends
	if err = c.remediateInstallerReboot(ctx, g.d, 1, provisioning.Target{}, nil, ssh); !errors.Is(err, ErrInstallerRebootExhausted) || ssh.sends != n {
		t.Fatal("unbounded retries", err)
	}
	// Old rows lack boot evidence, so their command must never be replayed.
	h := newBootstrapFixture(t, db)
	execBootstrap(t, db, "INSERT INTO installer_reboot_remediations(deployment_id,generation,state) VALUES($1,1,'SCHEDULED')", h.d.ID)
	if err = c.remediateInstallerReboot(ctx, h.d, 1, provisioning.Target{}, nil, ssh); !errors.Is(err, ErrInstallerRebootScheduled) || ssh.sends != n {
		t.Fatal("legacy reboot replay", err)
	}
}
