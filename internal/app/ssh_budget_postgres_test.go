package app

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"strings"
	"testing"
)

func TestInitialSSHBudgetPostgres(t *testing.T) {
	db := installerBootstrapDB(t)
	ctx := context.Background()
	h := RecoveryHandler{Container: Container{DB: db}}
	fixture := func() bootstrapFixture {
		f := newBootstrapFixture(t, db)
		execBootstrap(t, db, `UPDATE deployments SET state='PROVISIONING' WHERE id=$1`, f.d.ID)
		execBootstrap(t, db, `UPDATE provision_runs SET state='WAITING_SSH',current_step='ssh',created_at=now()-interval '16 minutes',last_error='publickey denied' WHERE id=$1`, f.run)
		execBootstrap(t, db, `INSERT INTO provision_step_attempts(run_id,step,attempts,last_started_at,last_finished_at,last_error,next_retry_at) VALUES($1,'ssh',3,now()-interval '2 minutes',now()-interval '1 minute','publickey denied',now()+interval '10 minutes')`, f.run)
		return f
	}
	t.Run("atomic-terminal-and-retirement-preserve-root-cause", func(t *testing.T) {
		f := fixture()
		stopped, err := h.expireInitialSSH(ctx, f.d)
		if err != nil || !stopped {
			t.Fatalf("stopped=%v err=%v", stopped, err)
		}
		var ds, vs, ps, msg string
		if err = db.QueryRow(`SELECT d.state,v.state,p.state,d.last_error FROM deployments d JOIN droplets v ON v.id=d.droplet_id JOIN provision_runs p ON p.droplet_id=v.id WHERE d.id=$1`, f.d.ID).Scan(&ds, &vs, &ps, &msg); err != nil {
			t.Fatal(err)
		}
		if ds != "FAILED" || vs != "RETIRING" || ps != "FAILED" || !strings.Contains(msg, "publickey denied") {
			t.Fatalf("%s %s %s %s", ds, vs, ps, msg)
		}
		_, err = h.expireInitialSSH(ctx, f.d)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		db.QueryRow(`SELECT count(*) FROM lifecycle_events WHERE resource_id=$1`, f.d.DropletID).Scan(&count)
		if count != 1 {
			t.Fatalf("duplicate retirement: %d", count)
		}
	})
	t.Run("active-owner-fenced", func(t *testing.T) {
		f := fixture()
		release, err := (workflow.PostgresRunLease{DB: db}).Acquire(ctx, f.d.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		_, err = h.expireInitialSSH(ctx, f.d)
		if err != nil {
			t.Fatal(err)
		}
		var s string
		db.QueryRow(`SELECT state FROM droplets WHERE id=$1`, f.d.DropletID).Scan(&s)
		if s != "PROVISIONING" {
			t.Fatal(s)
		}
	})
	for _, tc := range []struct{ name, query string }{
		{"installer-never-expired", `UPDATE provision_runs SET current_step='bootstrap',state='RUNNING_SCRIPT' WHERE id=$1`},
		{"fresh-boot", `UPDATE provision_runs SET created_at=now() WHERE id=$1`},
		{"not-enough-probes", `UPDATE provision_step_attempts SET attempts=1 WHERE run_id=$1`},
		{"interrupted-probe-reconcile-first", `UPDATE provision_step_attempts SET last_finished_at=NULL WHERE run_id=$1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture()
			execBootstrap(t, db, tc.query, f.run)
			stopped, err := h.expireInitialSSH(ctx, f.d)
			if err != nil || stopped {
				t.Fatalf("stopped=%v err=%v", stopped, err)
			}
		})
	}
}
