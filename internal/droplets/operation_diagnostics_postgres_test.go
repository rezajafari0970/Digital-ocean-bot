package droplets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestOperationDiagnosticsCapacityPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'upcloud','diagnostic-fixture','fixture') RETURNING id::text").Scan(&account); err != nil {
		t.Fatal(err)
	}
	store := jobs.SQLStore{DB: db}
	providerErr := &providers.Error{Class: providers.ErrorCapacity, StatusCode: 403, Code: "SECRET_SENTINEL", Message: "https://secret.example/SECRET_SENTINEL", Cause: nil}
	provider := &computeStub{createErr: providerErr}
	e := Executor{Operations: store, Provider: provider, Gate: gateStub{}}
	p := Profile{Name: "diagnostics", Region: "fixture", Size: "fixture", Image: "fixture"}
	op, err := e.Create(ctx, BuildCreateOperation(account, p), p)
	if err != providerErr || op.State != jobs.OperationFailed || provider.creates != 1 {
		t.Fatalf("unexpected outcome state=%s calls=%d err=%v", op.State, provider.creates, err)
	}
	var code, message string
	if err = db.QueryRow("SELECT coalesce(error_code,''),coalesce(error_message,'') FROM operations WHERE id=$1", op.ID).Scan(&code, &message); err != nil {
		t.Fatal(err)
	}
	if code != "capacity" || !strings.Contains(message, "HTTP 403") || strings.Contains(code+message, "SECRET_SENTINEL") {
		t.Fatalf("missing or unsafe diagnostic code=%q message=%q", code, message)
	}
}

func TestOperationDiagnosticsAtomicPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'upcloud','diagnostic-fixture','fixture') RETURNING id::text").Scan(&account); err != nil {
		t.Fatal(err)
	}
	store := jobs.SQLStore{DB: db}
	op, _, err := store.Reserve(ctx, jobs.Operation{AccountID: account, Kind: "CREATE_DROPLET", IdempotencyKey: "atomic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []jobs.OperationState{jobs.OperationRunning, jobs.OperationVerifying, jobs.OperationSucceeded} {
		op.State = jobs.OperationUnknown
		op.ErrorCode = "transport"
		op.ErrorMessage = "create_server: transport"
		if err = store.Update(ctx, &op); err != nil {
			t.Fatal(err)
		}
		got, e := store.Get(ctx, account, op.IdempotencyKey)
		if e != nil || got.ErrorCode != "transport" || got.ErrorMessage != op.ErrorMessage {
			t.Fatalf("readback=%+v err=%v", got, e)
		}
		// Recovery may persist richer evidence while a caller sends no new details.
		if _, err = db.Exec("UPDATE operations SET error_code='capacity',error_message='known outcome' WHERE id=$1", op.ID); err != nil {
			t.Fatal(err)
		}
		op.ErrorCode = ""
		op.ErrorMessage = ""
		if err = store.Update(ctx, &op); err != nil {
			t.Fatal(err)
		}
		if op.ErrorCode != "capacity" || op.ErrorMessage != "operation: capacity" {
			t.Fatalf("evidence erased or readback stale: %+v", op)
		}
		op.State = next
		if err = store.Update(ctx, &op); err != nil {
			t.Fatal(err)
		}
		got, e = store.Get(ctx, account, op.IdempotencyKey)
		if e != nil || got.ErrorCode != "" || got.ErrorMessage != "" || op.ErrorCode != "" || op.ErrorMessage != "" {
			t.Fatalf("stale diagnostic after progress: %+v err=%v", got, e)
		}
	}
	op.State = jobs.OperationFailed
	op.ErrorCode = "capacity"
	op.ErrorMessage = "create_server: capacity (HTTP 403)"
	stale := op
	if err = store.Update(ctx, &op); err != nil {
		t.Fatal(err)
	}
	stale.ErrorCode = "transport"
	stale.ErrorMessage = "loser"
	if err = store.Update(ctx, &stale); !errors.Is(err, jobs.ErrOperationVersionConflict) {
		t.Fatalf("stale error=%v", err)
	}
	got, e := store.Get(ctx, account, op.IdempotencyKey)
	if e != nil || got.ErrorCode != "capacity" || got.LockVersion != op.LockVersion {
		t.Fatalf("winning evidence lost: %+v err=%v", got, e)
	}
	wrong := op
	wrong.AccountID = "00000000-0000-0000-0000-000000000001"
	if err = store.Update(ctx, &wrong); !errors.Is(err, jobs.ErrOperationNotFound) {
		t.Fatalf("tenant check=%v", err)
	}
	// A cancelled context cannot silently claim the diagnostic was persisted.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	before := op.LockVersion
	if err = store.Update(cancelled, &op); !errors.Is(err, context.Canceled) || op.LockVersion != before {
		t.Fatalf("cancelled update=%v version=%d", err, op.LockVersion)
	}
}

func TestOperationDiagnosticsConcurrentPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'upcloud','diagnostic-race','fixture') RETURNING id::text").Scan(&account); err != nil {
		t.Fatal(err)
	}
	store := jobs.SQLStore{DB: db}
	original, _, err := store.Reserve(ctx, jobs.Operation{AccountID: account, Kind: "CREATE_DROPLET", IdempotencyKey: "race"})
	if err != nil {
		t.Fatal(err)
	}
	const n = 12
	start := make(chan struct{})
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			op := original
			op.State = jobs.OperationFailed
			op.ErrorCode = "capacity"
			op.ErrorMessage = fmt.Sprintf("create_server: capacity (HTTP %d)", 400+i)
			<-start
			done <- store.Update(ctx, &op)
		}(i)
	}
	close(start)
	winners := 0
	for i := 0; i < n; i++ {
		e := <-done
		if e == nil {
			winners++
		} else if !errors.Is(e, jobs.ErrOperationVersionConflict) {
			t.Fatal(e)
		}
	}
	got, err := store.Get(ctx, account, original.IdempotencyKey)
	if err != nil || winners != 1 || got.LockVersion != 1 || got.ErrorCode != "capacity" || !strings.HasPrefix(got.ErrorMessage, "create_server: capacity (HTTP 4") {
		t.Fatalf("winners=%d got=%+v err=%v", winners, got, err)
	}
}

func TestOperationDiagnosticsJournalFaultPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'upcloud','diagnostic-fault','fixture') RETURNING id::text").Scan(&account); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`CREATE FUNCTION reject_diagnostic() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='failed' THEN RAISE EXCEPTION 'injected journal fault'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_diagnostic BEFORE UPDATE ON operations FOR EACH ROW EXECUTE FUNCTION reject_diagnostic()`)
	if err != nil {
		t.Fatal(err)
	}
	store := jobs.SQLStore{DB: db}
	p := &computeStub{createErr: &providers.Error{Class: providers.ErrorRegionCapacity, StatusCode: 503}}
	e := Executor{Operations: store, Provider: p, Gate: gateStub{}}
	profile := Profile{Name: "fixture"}
	op, err := e.Create(ctx, BuildCreateOperation(account, profile), profile)
	if err == nil || providers.IsClass(err, providers.ErrorRegionCapacity) || !strings.Contains(err.Error(), "persist operation outcome") {
		t.Fatalf("journal failure hidden: %v", err)
	}
	got, err := store.Get(ctx, account, op.IdempotencyKey)
	if err != nil || got.State != jobs.OperationRunning || got.ErrorCode != "" || p.creates != 1 {
		t.Fatalf("durable state=%+v calls=%d err=%v", got, p.creates, err)
	}
}

func TestOperationDiagnosticsLegacySecretPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if err := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'upcloud','diagnostic-legacy','fixture') RETURNING id::text").Scan(&account); err != nil {
		t.Fatal(err)
	}
	store := jobs.SQLStore{DB: db}
	op, _, err := store.Reserve(ctx, jobs.Operation{AccountID: account, Kind: "CREATE_DROPLET", IdempotencyKey: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"capacity", "SECRET_SENTINEL"} {
		if _, err = db.Exec("UPDATE operations SET state='unknown',error_code=$2,error_message='provider capacity: https://secret.example/SECRET_SENTINEL' WHERE id=$1", op.ID, code); err != nil {
			t.Fatal(err)
		}
		got, e := store.Get(ctx, account, op.IdempotencyKey)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(got.ErrorCode+got.ErrorMessage, "SECRET_SENTINEL") {
			t.Fatal("legacy secret exposed through Get")
		}
		got.ErrorCode = ""
		got.ErrorMessage = ""
		if e = store.Update(ctx, &got); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(got.ErrorCode+got.ErrorMessage, "SECRET_SENTINEL") {
			t.Fatal("legacy secret exposed through Update RETURNING")
		}
		var raw string
		if e = db.QueryRow("SELECT error_message FROM operations WHERE id=$1", op.ID).Scan(&raw); e != nil || !strings.Contains(raw, "SECRET_SENTINEL") {
			t.Fatal("missing metadata erased historical evidence", e)
		}
	}
}
