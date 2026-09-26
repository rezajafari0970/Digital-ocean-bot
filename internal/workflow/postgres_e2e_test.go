package workflow

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func e2eDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DOB_E2E_DSN")
	if dsn == "" {
		t.Skip("DOB_E2E_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPostgresRunLeaseExclusiveE2E(t *testing.T) {
	db := e2eDB(t)
	lease := PostgresRunLease{DB: db}
	release, err := lease.Acquire(context.Background(), "e2e-deployment")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err = lease.Acquire(context.Background(), "e2e-deployment"); !errors.Is(err, ErrDeploymentBusy) {
		t.Fatalf("second lease err=%v", err)
	}
}
func TestSQLStepRetrySemanticsE2E(t *testing.T) {
	db := e2eDB(t)
	ctx := context.Background()
	var accountID, profileID, deploymentID string
	if err := db.QueryRowContext(ctx, `INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'digitalocean','e2e','x') RETURNING id::text`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, accountID) })
	if err := db.QueryRowContext(ctx, `INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'e2e','{}') RETURNING id::text`, accountID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create') RETURNING id::text`, accountID, profileID).Scan(&deploymentID); err != nil {
		t.Fatal(err)
	}
	s := SQLStore{DB: db}
	if n, err := s.BeginStep(ctx, deploymentID, "create", 3); err != nil || n != 1 {
		t.Fatalf("begin n=%d err=%v", n, err)
	}
	if err := s.FinishStep(ctx, deploymentID, "create", errors.New("temporary"), ErrorRetryable); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginStep(ctx, deploymentID, "create", 3); !errors.Is(err, ErrStepRetryDeferred) {
		t.Fatalf("want deferred got %v", err)
	}
	_, err := db.ExecContext(ctx, `UPDATE deployment_step_attempts SET next_retry_at=now()-interval '1 second' WHERE deployment_id=$1 AND step='create'`, deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.BeginStep(ctx, deploymentID, "create", 3); err != nil || n != 2 {
		t.Fatalf("retry n=%d err=%v", n, err)
	}
	if err := s.FinishStep(ctx, deploymentID, "create", errors.New("bad auth"), ErrorAuth); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginStep(ctx, deploymentID, "create", 3); !errors.Is(err, ErrStepTerminal) {
		t.Fatalf("want terminal got %v", err)
	}
	var class string
	var next sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT last_error_class,next_retry_at FROM deployment_step_attempts WHERE deployment_id=$1 AND step='create'`, deploymentID).Scan(&class, &next); err != nil {
		t.Fatal(err)
	}
	if class != string(ErrorAuth) || next.Valid {
		t.Fatalf("class=%s next=%v", class, next)
	}
	_ = time.Second
}
