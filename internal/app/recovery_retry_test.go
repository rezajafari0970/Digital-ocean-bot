package app

import (
	"context"
	"testing"
	"time"
)

func TestProvisionStepRetryUsesStepTimestampAndPropagatesReadErrors(t *testing.T) {
	db := fenceTestDB(t)
	_, err := db.Exec(`CREATE TABLE provision_runs(id text,account_id text,droplet_id text,next_retry_at timestamptz);
 CREATE TABLE provision_step_attempts(run_id text,step text,next_retry_at timestamptz);
 INSERT INTO provision_runs VALUES('r','a','d',now()-interval '1 hour');
 INSERT INTO provision_step_attempts VALUES('r','install',now()+interval '5 minutes');`)
	if err != nil {
		t.Fatal(err)
	}
	next, err := provisionStepRetryAt(context.Background(), db, "a", "d", "install")
	if err != nil || !next.Valid || time.Until(next.Time) < 4*time.Minute {
		t.Fatal(next, err)
	}
	next, err = provisionStepRetryAt(context.Background(), db, "a", "d", "absent")
	if err != nil || next.Valid {
		t.Fatal(next, err)
	}
	if _, err = db.Exec("ALTER TABLE provision_step_attempts ALTER COLUMN next_retry_at TYPE text USING 'invalid'"); err != nil {
		t.Fatal(err)
	}
	if _, err = provisionStepRetryAt(context.Background(), db, "a", "d", "install"); err == nil {
		t.Fatal("scan failure became permission to retry")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = provisionStepRetryAt(ctx, db, "a", "d", "install"); err == nil {
		t.Fatal("canceled read became permission to retry")
	}
}
