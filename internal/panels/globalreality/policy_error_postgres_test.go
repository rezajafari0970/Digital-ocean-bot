package globalreality

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
)

type unusedPolicySecrets struct{}

func (unusedPolicySecrets) Get(context.Context, string, string) ([]byte, error) {
	return nil, errors.New("unexpected secret read")
}
func (unusedPolicySecrets) Put(context.Context, string, string, string, []byte) error {
	return errors.New("unexpected secret write")
}

func TestGlobalRealityPolicyQueryFailureIsNotDisabledSuccess(t *testing.T) {
	raw := os.Getenv("BULK_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.TrimPrefix(u.Path, "/"), "_test_") {
		t.Fatal("refusing non-test database")
	}
	schema := fmt.Sprintf("global_policy_failure_%d", time.Now().UnixNano())
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE")
	if _, err = db.ExecContext(ctx, "CREATE TABLE reality_transport_apply_jobs(panel_id uuid,state text)"); err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db, Secrets: unusedPolicySecrets{}}
	panel := readyworker.Panel{ID: "4b5875e4-1e37-442e-b9d2-b079d7865786"}
	// A genuine missing-table query failure must remain observable. It must
	// never be interpreted as an explicitly disabled policy.
	err = service.ReconcilePanel(ctx, panel, true)
	var pe *pq.Error
	if !errors.As(err, &pe) || pe.Code != "42P01" {
		t.Errorf("policy SQL failure became %v", err)
	}
	if _, err = db.ExecContext(ctx, "CREATE TABLE global_config_policies(policy_key text,enabled bool,ports jsonb)"); err != nil {
		t.Fatal(err)
	}
	if err = service.ReconcilePanel(ctx, panel, true); err != nil {
		t.Errorf("absent policy: %v", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO global_config_policies VALUES('reality',false,'[443]')`); err != nil {
		t.Fatal(err)
	}
	if err = service.ReconcilePanel(ctx, panel, true); err != nil {
		t.Errorf("explicit disabled policy: %v", err)
	}
}
