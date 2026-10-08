package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"sync"
	"testing"
	"time"
)

const authAccountA = "00000000-0000-4000-8000-000000000001"
const authAccountB = "00000000-0000-4000-8000-000000000002"

func authRetryFixture(t *testing.T) *sql.DB {
	db := failureLedgerFixture(t)
	_, err := db.Exec(`CREATE TABLE proxy_economy_policy(singleton boolean,enabled boolean,canary_account_id uuid);
INSERT INTO proxy_economy_policy VALUES(true,true,NULL);
CREATE TABLE accounts(id uuid PRIMARY KEY,secret_ref text);
CREATE TABLE secrets(account_id uuid,id text,updated_at timestamptz);
CREATE TABLE worker_recovery_checkpoints(kind text,item_id text,account_id uuid,created_at timestamptz DEFAULT now());
CREATE TABLE operations(id text,state text);
INSERT INTO operations VALUES('retained','unknown');
INSERT INTO accounts VALUES('` + authAccountA + `','key'),('` + authAccountB + `','key');
INSERT INTO secrets SELECT id,secret_ref,now()-interval '1 hour' FROM accounts;`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func auth401() error {
	return &providers.Error{Class: providers.ErrorAuthentication, StatusCode: 401, Message: "provider rejected credential"}
}

type authHint struct{ error }

func (authHint) RetryDelay() time.Duration { return 9 * time.Second }
func seedAuthAttempt(t *testing.T, db *sql.DB, kind, item, account string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO worker_recovery_checkpoints(kind,item_id,account_id) VALUES($1,$2,$3)`, kind, item, account); err != nil {
		t.Fatal(err)
	}
}
func authDelay(t *testing.T, db *sql.DB, item string) (int, string, int) {
	t.Helper()
	var delay, failures int
	var msg string
	if err := db.QueryRow(`SELECT round(extract(epoch FROM next_retry_at-last_failed_at))::int,last_error,failures FROM worker_item_failures WHERE item_id=$1`, item).Scan(&delay, &msg, &failures); err != nil {
		t.Fatal(err)
	}
	return delay, msg, failures
}
func TestProviderAuthRetryExactScopeAndPrecedencePostgres(t *testing.T) {
	db := authRetryFixture(t)
	ctx := context.Background()
	s := FailureStore{DB: db}
	cases := []struct {
		name, kind string
		cause      error
		want       int
		marker     bool
	}{
		{"operation", "operation", auth401(), 300, true}, {"lifecycle", "lifecycle", fmt.Errorf("wrapped: %w", auth401()), 300, true},
		{"deployment", "deployment", auth401(), 2, false}, {"untyped", "operation", errors.New("HTTP401"), 2, false},
		{"proxy407", "operation", &providers.Error{Class: providers.ErrorAuthentication, StatusCode: 407}, 2, false},
		{"transport", "operation", &providers.Error{Class: providers.ErrorTransport, StatusCode: 401}, 2, false},
		{"ambiguous", "operation", &providers.Error{Class: providers.ErrorAmbiguousOutcome, Cause: auth401()}, 2, false},
		{"hint", "operation", authHint{auth401()}, 9, false},
	}
	for _, v := range cases {
		seedAuthAttempt(t, db, v.kind, v.name, authAccountA)
		if err := s.FailChecked(ctx, v.kind, v.name, authAccountA, v.cause); err != nil {
			t.Fatal(err)
		}
		delay, msg, _ := authDelay(t, db, v.name)
		if delay != v.want || (msg == ProviderAuthRetryMarker) != v.marker {
			t.Fatalf("%s delay=%d marker=%s", v.name, delay, msg)
		}
	}
	for _, v := range []struct{ name, sql string }{{"off", `UPDATE proxy_economy_policy SET enabled=false`}, {"other_canary", `UPDATE proxy_economy_policy SET enabled=true,canary_account_id='` + authAccountB + `'`}} {
		if _, err := db.Exec(v.sql); err != nil {
			t.Fatal(err)
		}
		seedAuthAttempt(t, db, "operation", v.name, authAccountA)
		if err := s.FailChecked(ctx, "operation", v.name, authAccountA, auth401()); err != nil {
			t.Fatal(err)
		}
		delay, _, _ := authDelay(t, db, v.name)
		if delay != 2 {
			t.Fatal(v.name, delay)
		}
	}
	if _, err := db.Exec(`UPDATE proxy_economy_policy SET canary_account_id='` + authAccountA + `'`); err != nil {
		t.Fatal(err)
	}
	seedAuthAttempt(t, db, "operation", "own_canary", authAccountA)
	if err := s.FailChecked(ctx, "operation", "own_canary", authAccountA, auth401()); err != nil {
		t.Fatal(err)
	}
	delay, _, _ := authDelay(t, db, "own_canary")
	if delay != 300 {
		t.Fatal(delay)
	}
	if err := s.FailChecked(ctx, "operation", "missing_checkpoint", authAccountA, auth401()); err != nil {
		t.Fatal(err)
	}
	delay, _, _ = authDelay(t, db, "missing_checkpoint")
	if delay != 2 {
		t.Fatal("missing attempt evidence", delay)
	}
}
func TestProviderAuthRetryConcurrentRearmAndRollbackPostgres(t *testing.T) {
	db := authRetryFixture(t)
	ctx := context.Background()
	s := FailureStore{DB: db}
	seedAuthAttempt(t, db, "operation", "one", authAccountA)
	seedAuthAttempt(t, db, "lifecycle", "other", authAccountB)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.FailChecked(ctx, "operation", "one", authAccountA, auth401()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.FailChecked(ctx, "lifecycle", "other", authAccountB, auth401()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO worker_item_failures(kind,item_id,account_id,failures,last_error,next_retry_at) VALUES('operation','similar',$1,17,'PROVIDER_AUTH_RETRY_extra',now()+interval '5 minutes')`, authAccountA); err != nil {
		t.Fatal(err)
	}
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = RearmProviderAuthRetriesTx(ctx, tx, authAccountA); err != nil {
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		if due, err := s.DueChecked(ctx, "operation", "one"); err != nil || due != commit {
			t.Fatal(due, err)
		}
	}
	_, _, count := authDelay(t, db, "one")
	if count != 8 {
		t.Fatal("failure counter reset", count)
	}
	for _, v := range []struct{ kind, item string }{{"lifecycle", "other"}, {"operation", "similar"}} {
		if s.Due(ctx, v.kind, v.item) {
			t.Fatal("unrelated retry rearmed", v)
		}
	}
	// Replacement commits while an old attempt is finishing: its late 401
	// cannot put the new credential behind another five-minute wait.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET secret_ref=secret_ref WHERE id=$1`, authAccountA); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE secrets SET updated_at=clock_timestamp() WHERE account_id=$1`, authAccountA); err != nil {
		t.Fatal(err)
	}
	if err = RearmProviderAuthRetriesTx(ctx, tx, authAccountA); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.FailChecked(ctx, "operation", "one", authAccountA, auth401()) }()
	select {
	case err := <-done:
		t.Fatal("failure bypassed credential transaction", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	delay, msg, count := authDelay(t, db, "one")
	if delay != 2 || msg == ProviderAuthRetryMarker || count != 9 {
		t.Fatal("old credential backoff", delay, msg, count)
	}
	// Failure commits first: subsequent validated replacement makes it due.
	if _, err = db.Exec(`UPDATE secrets SET updated_at=now()-interval '1 hour' WHERE account_id=$1`, authAccountA); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = failTx(ctx, tx, "operation", "one", authAccountA, auth401()); err != nil {
		t.Fatal(err)
	}
	go func() {
		replacement, e := db.BeginTx(ctx, nil)
		if e == nil {
			e = RearmProviderAuthRetriesTx(ctx, replacement, authAccountA)
			if e == nil {
				e = replacement.Commit()
			} else {
				replacement.Rollback()
			}
		}
		done <- e
	}()
	select {
	case err := <-done:
		t.Fatal("replacement bypassed pending failure", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if !s.Due(ctx, "operation", "one") {
		t.Fatal("replacement did not rearm")
	}
	var state string
	var checkpoints int
	if err = db.QueryRow(`SELECT state FROM operations WHERE id='retained'`).Scan(&state); err != nil || state != "unknown" {
		t.Fatal("unknown outcome changed", state, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM worker_recovery_checkpoints`).Scan(&checkpoints); err != nil || checkpoints != 2 {
		t.Fatal("checkpoints changed", checkpoints, err)
	}
}
