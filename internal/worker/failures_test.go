package worker

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func failureLedgerFixture(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated DB required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := sanaei.UUIDv4()
	schema := "ledger_" + strings.ReplaceAll(id, "-", "")
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
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	if _, err = db.Exec(`CREATE TABLE worker_item_failures(kind text,item_id text,account_id uuid,failures int,last_error text,first_failed_at timestamptz,last_failed_at timestamptz,next_retry_at timestamptz,PRIMARY KEY(kind,item_id))`); err != nil {
		t.Fatal(err)
	}
	return db
}
func TestFailureLedgerAtomicBackoffAndDatabaseReadFailure(t *testing.T) {
	db := failureLedgerFixture(t)
	s := FailureStore{DB: db}
	ctx := context.Background()
	if due, err := s.DueChecked(ctx, "lifecycle", "one"); err != nil || !due {
		t.Fatal(due, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.FailChecked(ctx, "lifecycle", "one", "", errors.New("fixture timeout"))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var failures int
	var next time.Time
	if err := db.QueryRow("SELECT failures,next_retry_at FROM worker_item_failures").Scan(&failures, &next); err != nil || failures != 8 || time.Until(next) < 60*time.Second {
		t.Fatal(failures, next, err)
	}
	if due, err := s.DueChecked(ctx, "lifecycle", "one"); err != nil || due {
		t.Fatal(due, err)
	}
	if _, err := db.Exec(`CREATE FUNCTION reject_delay() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.next_retry_at IS DISTINCT FROM OLD.next_retry_at THEN RAISE EXCEPTION 'fault'; END IF; RETURN NEW; END$$;
 CREATE TRIGGER reject_delay BEFORE UPDATE ON worker_item_failures FOR EACH ROW EXECUTE FUNCTION reject_delay();`); err != nil {
		t.Fatal(err)
	}
	if err := s.FailChecked(ctx, "lifecycle", "one", "", errors.New("fault")); err == nil {
		t.Fatal("failed delay update hidden")
	}
	if err := db.QueryRow("SELECT failures FROM worker_item_failures").Scan(&failures); err != nil || failures != 8 {
		t.Fatal("partial ledger commit", failures, err)
	}
	if _, err := db.Exec("DROP TABLE worker_item_failures"); err != nil {
		t.Fatal(err)
	}
	if due, err := s.DueChecked(ctx, "lifecycle", "one"); err == nil || due {
		t.Fatal("failed read authorized execution", due, err)
	}
	if s.Due(ctx, "lifecycle", "one") {
		t.Fatal("compatibility caller fails open")
	}
}
