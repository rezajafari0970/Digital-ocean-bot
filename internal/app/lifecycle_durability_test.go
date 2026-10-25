package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
)

var lifecycleReadFailure = errors.New("injected lifecycle result-stream failure")

type lifecycleFaultConnector struct {
	driver.Connector
	after int
}

func (c lifecycleFaultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &lifecycleFaultConn{Conn: conn, after: c.after}, nil
}

type lifecycleFaultConn struct {
	driver.Conn
	after int
}

func (c *lifecycleFaultConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
}
func (c *lifecycleFaultConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}
func (c *lifecycleFaultConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
	if err == nil && strings.HasPrefix(q, "UPDATE deployments SET lock_version=") && strings.Contains(q, "RETURNING id::text") {
		return &lifecycleInterruptedRows{Rows: rows, remaining: c.after}, nil
	}
	return rows, err
}

type lifecycleInterruptedRows struct {
	driver.Rows
	remaining int
}

func (r *lifecycleInterruptedRows) Next(v []driver.Value) error {
	if r.remaining == 0 {
		return lifecycleReadFailure
	}
	err := r.Rows.Next(v)
	if err == nil {
		r.remaining--
	}
	return err
}
func lifecycleFaultDB(t *testing.T, db *sql.DB, after int) *sql.DB {
	t.Helper()
	var schema string
	if err := db.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("BULK_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	connector, err := pq.NewConnector(u.String())
	if err != nil {
		t.Fatal(err)
	}
	fault := sql.OpenDB(lifecycleFaultConnector{Connector: connector, after: after})
	fault.SetMaxOpenConns(2)
	t.Cleanup(func() { fault.Close() })
	return fault
}

func TestLifecycleDeleteConfirmationInterruptedReadRollsBack(t *testing.T) {
	db := installerBootstrapDB(t)
	for _, tc := range []struct {
		name  string
		after int
	}{{"before_first_row", 0}, {"after_first_row", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBootstrapFixture(t, db)
			execBootstrap(t, db, `UPDATE accounts SET enabled=true,provider_checked_at=now() WHERE id=$1`, f.d.AccountID)
			execBootstrap(t, db, `UPDATE droplets SET state='DELETING' WHERE id=$1`, f.d.DropletID)
			execBootstrap(t, db, `UPDATE deployments SET provider_id='fixture-resource' WHERE id=$1`, f.d.ID)
			execBootstrap(t, db, `INSERT INTO deployments(id,account_id,profile_id,droplet_id,provider_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,$3,'fixture-resource','WAITING_INSTALLER','provision')`, f.d.AccountID, f.d.ProfileID, f.d.DropletID)
			execBootstrap(t, db, `INSERT INTO resources(id,account_id,provider,provider_resource_id,type,state,managed) VALUES(gen_random_uuid(),$1,'vultr','fixture-resource','server','active',true)`, f.d.AccountID)
			execBootstrap(t, db, `INSERT INTO schedules(id,account_id,profile_id,interval_seconds,next_run_at) VALUES(gen_random_uuid(),$1,$2,60,now()+interval '1 hour')`, f.d.AccountID, f.d.ProfileID)
			err := (Container{DB: lifecycleFaultDB(t, db, tc.after)}).ConfirmDeleted(context.Background(), f.d.AccountID, "fixture-resource")
			if !errors.Is(err, lifecycleReadFailure) {
				t.Fatalf("interrupted confirmation returned %v; must roll back", err)
			}
			var intact bool
			err = db.QueryRow(`SELECT
   (SELECT state='DELETING' FROM droplets WHERE id=$1) AND
   (SELECT state='active' FROM resources WHERE account_id=$2 AND provider_resource_id='fixture-resource') AND
   (SELECT count(*)=2 AND bool_and(state='WAITING_INSTALLER' AND lock_version=0) FROM deployments WHERE account_id=$2) AND
   NOT EXISTS(SELECT 1 FROM deployment_events e JOIN deployments d ON d.id=e.deployment_id WHERE d.account_id=$2 AND e.message='RESOURCE_DELETED') AND
   (SELECT next_run_at>now()+interval '30 minutes' FROM schedules WHERE account_id=$2) AND
   (SELECT provider_checked_at IS NOT NULL FROM accounts WHERE id=$2)`, f.d.DropletID, f.d.AccountID).Scan(&intact)
			if err != nil || !intact {
				t.Fatalf("partial delete escaped rollback: intact=%v err=%v", intact, err)
			}
			c := Container{DB: db}
			if err = c.ConfirmDeleted(context.Background(), f.d.AccountID, "fixture-resource"); err != nil {
				t.Fatal(err)
			}
			if err = c.ConfirmDeleted(context.Background(), f.d.AccountID, "fixture-resource"); err != nil {
				t.Fatal(err)
			}
			var retired bool
			var events int
			err = db.QueryRow(`SELECT (SELECT state='DELETED' FROM droplets WHERE id=$1),(SELECT count(*) FROM deployment_events e JOIN deployments d ON d.id=e.deployment_id WHERE d.account_id=$2 AND e.message='RESOURCE_DELETED')`, f.d.DropletID, f.d.AccountID).Scan(&retired, &events)
			if err != nil || !retired || events != 2 {
				t.Fatalf("retry/idempotency failed: retired=%v events=%d err=%v", retired, events, err)
			}
		})
	}
}
