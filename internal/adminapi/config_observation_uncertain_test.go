package adminapi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestConfigObservationDowngradeUncertainSessionDiscard(t *testing.T) {
	t.Setenv("DOB_CONFIG_OBSERVATION_PANELS", "all")
	db := adminTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// This isolated test-schema wrapper grants the real session lock and then
	// returns a nonfatal SQL error. PostgreSQL keeps session advisory locks across
	// statement abort; no backend termination is used to inject this fault.
	sqlMust(t, db, `CREATE FUNCTION pg_advisory_lock_shared(bigint) RETURNS void LANGUAGE plpgsql AS $$
 BEGIN
  PERFORM pg_catalog.pg_advisory_lock_shared($1);
  RAISE EXCEPTION 'synthetic lost downgrade acknowledgement';
 END $$`)
	observer, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	var observerPID, ownerPID int
	if err = observer.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&observerPID); err != nil {
		t.Fatal(err)
	}
	// Baseline cleanup happens only after assertions, so it cannot mask a leak.
	defer func() {
		if ownerPID > 0 {
			_, _ = observer.ExecContext(context.Background(), "SELECT pg_terminate_backend($1)", ownerPID)
		}
	}()
	err = sanaei.WithConfigLock(ctx, db, "uncertain-downgrade", func(c context.Context) error {
		if e := sanaei.WithConfigEvidenceTx(c, db, "uncertain-downgrade", func(tx *sql.Tx) error {
			if e := tx.QueryRowContext(c, "SELECT pg_backend_pid()").Scan(&ownerPID); e != nil {
				return e
			}
			_, e := tx.ExecContext(c, "SELECT set_config('search_path',quote_ident(current_schema())||',pg_catalog',false)")
			return e
		}); e != nil {
			return e
		}
		if e := sanaei.ConfigProofOnly(c, db, "uncertain-downgrade"); e == nil {
			t.Error("synthetic downgrade fault did not fire")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if observerPID == ownerPID {
		t.Fatal("observer did not use distinct physical connection")
	}
	var held int
	if err = observer.QueryRowContext(ctx, "SELECT count(*) FROM pg_locks WHERE pid=$1 AND locktype='advisory'", ownerPID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Errorf("uncertain downgrade leaked %d session advisory locks", held)
	}
	for _, key := range []string{"panel-config:uncertain-downgrade", "panel-config-observe:uncertain-downgrade"} {
		var acquired bool
		if err = observer.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired); err != nil || !acquired {
			t.Errorf("distinct writer blocked after uncertain downgrade: acquired=%t err=%v", acquired, err)
		}
	}
	if _, err = observer.ExecContext(ctx, "SELECT pg_advisory_unlock_all()"); err != nil {
		t.Fatal(err)
	}
}
