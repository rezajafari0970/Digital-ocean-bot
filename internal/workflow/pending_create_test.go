package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func pendingFixture(t *testing.T) (*sql.DB, Deployment) {
	t.Helper()
	db := pendingTestDB(t)
	var d Deployment
	if err := db.QueryRow(`INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'kamatera','async-wait-test','fixture') RETURNING id::text`).Scan(&d.AccountID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id=$1", d.AccountID) })
	if err := db.QueryRow(`INSERT INTO deployment_profiles(id,account_id,name,config) VALUES(gen_random_uuid(),$1,'async-wait','{}') RETURNING id::text`, d.AccountID).Scan(&d.ProfileID); err != nil {
		t.Fatal(err)
	}
	var err error
	d, _, err = (SQLStore{DB: db}).Reserve(context.Background(), Request{AccountID: d.AccountID, ProfileID: d.ProfileID})
	if err != nil {
		t.Fatal(err)
	}
	return db, d
}
func pendingOperation(t *testing.T, db *sql.DB, d Deployment, state, key, account, kind, resource string, attempt int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO operations(id,account_id,kind,idempotency_key,state,attempt,resource_id) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,NULLIF($6,''))`, account, kind, key, state, attempt, resource)
	if err != nil {
		t.Fatal(err)
	}
}
func pendingDue(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec("UPDATE deployment_step_attempts SET next_retry_at=now()-interval '1 second' WHERE deployment_id=$1 AND step='create'", id); err != nil {
		t.Fatal(err)
	}
}

func TestSQLPendingCreateWaitsWithoutRetryExhaustionE2E(t *testing.T) {
	db, d := pendingFixture(t)
	ctx := context.Background()
	s := SQLStore{DB: db}
	if n, err := s.BeginStep(ctx, d.ID, "create", 1); err != nil || n != 1 {
		t.Fatalf("initial %d %v", n, err)
	}
	pendingOperation(t, db, d, "unknown", "deploy:"+d.ID+":create:region:US-SC", d.AccountID, "CREATE_DROPLET", "", 1)
	// A prior retry/cancellation budget cannot terminalize an already dispatched intent.
	if _, err := db.Exec("UPDATE deployment_step_attempts SET attempts=7,last_error_class='auth' WHERE deployment_id=$1", d.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		pendingDue(t, db, d.ID)
		if n, err := s.BeginStep(ctx, d.ID, "create", 1); err != nil || n != 7 {
			t.Fatalf("poll %d native attempts=%d error=%v", i, n, err)
		}
		if err := s.FinishStep(ctx, d.ID, "create", errors.New("native confirmation pending"), ErrorPending); err != nil {
			t.Fatal(err)
		}
		if _, err := s.BeginStep(ctx, d.ID, "create", 1); !errors.Is(err, ErrStepRetryDeferred) {
			t.Fatalf("missing poll delay: %v", err)
		}
	}
	var next time.Time
	if err := db.QueryRow("SELECT next_retry_at FROM deployment_step_attempts WHERE deployment_id=$1", d.ID).Scan(&next); err != nil || time.Until(next) < 20*time.Second || time.Until(next) > 31*time.Second {
		t.Fatalf("pending backoff %v %v", next, err)
	}
	// Definitive rejection restores the finite retry policy.
	if _, err := db.Exec("UPDATE operations SET state='failed' WHERE account_id=$1", d.AccountID); err != nil {
		t.Fatal(err)
	}
	pendingDue(t, db, d.ID)
	if _, err := s.BeginStep(ctx, d.ID, "create", 1); !errors.Is(err, ErrStepRetryLimit) {
		t.Fatalf("failed operation bypassed finite cap: %v", err)
	}
}

func TestSQLPendingCreateEvidenceFencingE2E(t *testing.T) {
	cases := []struct {
		name, state, kind, suffix, resource string
		attempt                             int
		foreign, want                       bool
	}{
		{"unknown", "unknown", "CREATE_DROPLET", "US-SC", "", 1, false, true},
		{"running", "running", "CREATE_DROPLET", "US-SC", "", 1, false, true},
		{"verifying", "verifying", "CREATE_DROPLET", "US-SC", "server", 1, false, true},
		{"lost_ack", "succeeded", "CREATE_DROPLET", "US-SC", "server", 1, false, true},
		{"failed", "failed", "CREATE_DROPLET", "US-SC", "", 1, false, false},
		{"planned", "planned", "CREATE_DROPLET", "US-SC", "", 0, false, false},
		{"undispatched", "unknown", "CREATE_DROPLET", "US-SC", "", 0, false, false},
		{"empty_success", "succeeded", "CREATE_DROPLET", "US-SC", "", 1, false, false},
		{"other_kind", "unknown", "DELETE_DROPLET", "US-SC", "", 1, false, false},
		{"empty_region", "unknown", "CREATE_DROPLET", "", "", 1, false, false},
		{"other_deployment", "unknown", "CREATE_DROPLET", "wrong-deployment", "", 1, false, false},
		{"other_account", "unknown", "CREATE_DROPLET", "US-SC", "", 1, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, d := pendingFixture(t)
			account := d.AccountID
			if tc.foreign {
				if err := db.QueryRow(`INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'kamatera','foreign-async','fixture') RETURNING id::text`).Scan(&account); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id=$1", account) })
			}
			key := "deploy:" + d.ID + ":create:region:" + tc.suffix
			if tc.name == "other_deployment" {
				key = "deploy:00000000-0000-0000-0000-000000000000:create:region:US-SC"
			}
			pendingOperation(t, db, d, tc.state, key, account, tc.kind, tc.resource, tc.attempt)
			got, err := (SQLStore{DB: db}).PendingCreate(context.Background(), d.ID)
			if err != nil || got != tc.want {
				t.Fatalf("pending=%v want=%v err=%v", got, tc.want, err)
			}
		})
	}
}

type delayedNativeCreator struct {
	db                  *sql.DB
	posts, observations int
}

func (c *delayedNativeCreator) Create(ctx context.Context, want jobs.Operation, _ droplets.Profile) (jobs.Operation, error) {
	store := jobs.SQLStore{DB: c.db}
	op, fresh, err := store.Reserve(ctx, want)
	if err != nil {
		return op, err
	}
	if fresh {
		c.posts++
		op.Attempt = 1
		op.State = jobs.OperationUnknown
		op.ProviderActionID = "accepted-native-job"
		if err = store.Update(ctx, &op); err != nil {
			return op, err
		}
	}
	c.observations++
	if c.observations <= 10 {
		return op, &providers.Error{Class: providers.ErrorAmbiguousOutcome, Operation: "create_server", Message: "waiting for native server identity"}
	}
	op.State = jobs.OperationSucceeded
	op.ResourceID = "verified-native-server"
	return op, store.Update(ctx, &op)
}

type asyncRuntimeSteps struct {
	stepStub
	create RuntimeSteps
}

func (s *asyncRuntimeSteps) Create(ctx context.Context, d Deployment) (Deployment, error) {
	return s.create.Create(ctx, d)
}

type pendingFailureCounter struct{ calls int }

func (f *pendingFailureCounter) MarkFailed(context.Context, Deployment) error { f.calls++; return nil }
func (f *pendingFailureCounter) MarkFailedIn(context.Context, DBTX, Deployment) error {
	f.calls++
	return nil
}

func TestWorkflowPendingCreateKeepsReservationAndEventuallyResumesE2E(t *testing.T) {
	db, d := pendingFixture(t)
	creator := &delayedNativeCreator{db: db}
	failure := &pendingFailureCounter{}
	steps := &asyncRuntimeSteps{create: RuntimeSteps{Droplets: creator, Profile: droplets.Profile{Name: "owned", Region: "US-SC", Size: "1A-1024-20", Image: "ubuntu"}}}
	req := Request{DeploymentID: d.ID, AccountID: d.AccountID, ProfileID: d.ProfileID}
	for i := 0; i < 11; i++ {
		pendingDue(t, db, d.ID)
		// A new engine/store each time simulates controller restart between polls.
		engine := Engine{Store: SQLStore{DB: db}, Steps: steps, FailureFinalizer: failure}
		got, err := engine.Run(context.Background(), req)
		if i < 10 {
			if err == nil || got.State != Creating || got.CurrentStep != "create" {
				t.Fatalf("poll=%d state=%s step=%s err=%v", i, got.State, got.CurrentStep, err)
			}
			var reservations int
			if err = db.QueryRow("SELECT count(*) FROM deployments WHERE account_id=$1 AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE')", d.AccountID).Scan(&reservations); err != nil || reservations != 1 {
				t.Fatalf("reservation lost: %d %v", reservations, err)
			}
		} else if err != nil || got.State != Ready || got.ProviderID != "verified-native-server" {
			t.Fatalf("native completion did not resume: %+v %v", got, err)
		}
	}
	var attempts int
	if err := db.QueryRow("SELECT attempts FROM deployment_step_attempts WHERE deployment_id=$1 AND step='create'", d.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if creator.posts != 1 || attempts != 1 || failure.calls != 0 {
		t.Fatalf("posts=%d native_attempts=%d cleanup_calls=%d", creator.posts, attempts, failure.calls)
	}
}

type pendingReadFailStore struct{ memStore }

func (*pendingReadFailStore) PendingCreate(context.Context, string) (bool, error) {
	return false, errors.New("evidence database unavailable")
}

type authCreateSteps struct{ stepStub }

func (*authCreateSteps) Create(_ context.Context, d Deployment) (Deployment, error) {
	return d, &providers.Error{Class: providers.ErrorAuthentication, Message: "credential unavailable during reconciliation"}
}
func TestWorkflowPendingEvidenceReadFailureDoesNotFinalize(t *testing.T) {
	store := &pendingReadFailStore{}
	failure := &pendingFailureCounter{}
	d, err := (Engine{Store: store, Steps: &authCreateSteps{}, FailureFinalizer: failure}).Run(context.Background(), Request{AccountID: "account", ProfileID: "profile"})
	if err == nil || d.State == Failed || store.d.State == Failed || failure.calls != 0 {
		t.Fatal(fmt.Sprintf("state=%s persisted=%s cleanup=%d err=%v", d.State, store.d.State, failure.calls, err))
	}
}

// These tests exercise the dispatch fence when evidence changes between
// admission and execution, and recover a result lost after operation success.
type recoveryGate struct{}

func (recoveryGate) AllowMutation() error { return nil }

type recoveryCompute struct{ posts int }

func (p *recoveryCompute) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	p.posts++
	return providers.CreateServerResult{ServerID: "verified-native-server", Outcome: providers.OutcomeAccepted}, nil
}
func (*recoveryCompute) GetServer(context.Context, string) (providers.Server, error) {
	return providers.Server{ID: "verified-native-server"}, nil
}
func (*recoveryCompute) ListServers(context.Context) ([]providers.Server, error) { return nil, nil }
func (*recoveryCompute) DeleteServer(context.Context, string) error {
	return errors.New("unexpected deletion")
}
func (*recoveryCompute) FindServerByIdentity(context.Context, string) ([]providers.Server, error) {
	return nil, nil
}

func TestSQLUnchargedCreateCannotDispatchRegionalFallbackE2E(t *testing.T) {
	db, d := pendingFixture(t)
	ctx := context.Background()
	store := SQLStore{DB: db}
	if n, err := store.BeginStep(ctx, d.ID, "create", 3); err != nil || n != 1 {
		t.Fatalf("initial %d %v", n, err)
	}
	key := "deploy:" + d.ID + ":create:region:A"
	pendingOperation(t, db, d, "unknown", key, d.AccountID, "CREATE_DROPLET", "", 1)
	if err := store.FinishStep(ctx, d.ID, "create", errors.New("pending"), ErrorPending); err != nil {
		t.Fatal(err)
	}
	pendingDue(t, db, d.ID)
	d.State = Creating
	d.CurrentStep = "create"
	if err := store.AdmitStep(ctx, &d, "create", 3); err != nil || !d.CreateObservationOnly {
		t.Fatalf("pending admission %v mode=%v", err, d.CreateObservationOnly)
	}
	// A concurrent observation proves rejection after this admission was free.
	if _, err := db.Exec("UPDATE operations SET state='failed',error_code='region_capacity' WHERE account_id=$1", d.AccountID); err != nil {
		t.Fatal(err)
	}
	provider := &recoveryCompute{}
	steps := RuntimeSteps{Droplets: droplets.Executor{Operations: jobs.SQLStore{DB: db}, Provider: provider, Gate: recoveryGate{}}, Profile: droplets.Profile{Name: "owned", Region: "A", Regions: []string{"A", "B"}, Size: "plan", Image: "image"}}
	got, err := steps.Create(ctx, d)
	if !errors.Is(err, droplets.ErrCreateDispatchDeferred) || provider.posts != 0 || got.ProviderID != "" {
		t.Fatalf("free fallback dispatched: posts=%d result=%+v err=%v", provider.posts, got, err)
	}
	var operations int
	if err := db.QueryRow("SELECT count(*) FROM operations WHERE account_id=$1", d.AccountID).Scan(&operations); err != nil || operations != 1 {
		t.Fatalf("unexpected operation reservation: %d %v", operations, err)
	}
	if err := store.FinishStep(ctx, d.ID, "create", errors.New("dispatch deferred"), ErrorRetryable); err != nil {
		t.Fatal(err)
	}
	pendingDue(t, db, d.ID)
	if err := store.AdmitStep(ctx, &d, "create", 3); err != nil || d.CreateObservationOnly {
		t.Fatalf("normal admission not restored: %v mode=%v", err, d.CreateObservationOnly)
	}
	got, err = steps.Create(ctx, d)
	if err != nil || provider.posts != 1 || got.ProviderID != "verified-native-server" {
		t.Fatalf("budgeted fallback failed: posts=%d got=%+v err=%v", provider.posts, got, err)
	}
	var attempts int
	if err := db.QueryRow("SELECT attempts FROM deployment_step_attempts WHERE deployment_id=$1 AND step='create'", d.ID).Scan(&attempts); err != nil || attempts != 2 {
		t.Fatalf("fallback was not charged once: %d %v", attempts, err)
	}
}
func TestSQLPendingCreateCancellationFenceE2E(t *testing.T) {
	db, d := pendingFixture(t)
	ctx := context.Background()
	store := SQLStore{DB: db}
	pendingOperation(t, db, d, "unknown", "deploy:"+d.ID+":create:region:US-SC", d.AccountID, "CREATE_DROPLET", "", 1)
	d.State = Creating
	d.CurrentStep = "create"
	if err := store.AdmitStep(ctx, &d, "create", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE deployments SET state='FAILED',current_step='done',lock_version=lock_version+1 WHERE id=$1", d.ID); err != nil {
		t.Fatal(err)
	}
	called := false
	err := store.Finalize(ctx, &d, "create", "waiting", func(context.Context, DBTX) error { called = true; return nil })
	if !errors.Is(err, ErrDeploymentVersionConflict) || called {
		t.Fatalf("stale continuation overwrote cancellation: %v callback=%v", err, called)
	}
	var state string
	if err := db.QueryRow("SELECT state FROM deployments WHERE id=$1", d.ID).Scan(&state); err != nil || state != "FAILED" {
		t.Fatalf("cancellation lost: %s %v", state, err)
	}
}
func TestWorkflowSuccessfulCreateLostAcknowledgementE2E(t *testing.T) {
	db, d := pendingFixture(t)
	ctx := context.Background()
	store := SQLStore{DB: db}
	if _, err := store.BeginStep(ctx, d.ID, "create", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE deployment_step_attempts SET attempts=7,last_error_class='auth' WHERE deployment_id=$1", d.ID); err != nil {
		t.Fatal(err)
	}
	pendingOperation(t, db, d, "succeeded", "deploy:"+d.ID+":create:region:US-SC", d.AccountID, "CREATE_DROPLET", "verified-native-server", 1)
	provider := &recoveryCompute{}
	failure := &pendingFailureCounter{}
	steps := &asyncRuntimeSteps{create: RuntimeSteps{Droplets: droplets.Executor{Operations: jobs.SQLStore{DB: db}, Provider: provider, Gate: recoveryGate{}}, Profile: droplets.Profile{Name: "owned", Region: "US-SC", Size: "plan", Image: "image"}}}
	got, err := (Engine{Store: store, Steps: steps, FailureFinalizer: failure}).Run(ctx, Request{DeploymentID: d.ID, AccountID: d.AccountID, ProfileID: d.ProfileID})
	if err != nil || got.State != Ready || got.ProviderID != "verified-native-server" || provider.posts != 0 || failure.calls != 0 {
		t.Fatalf("lost ack recovery: %+v err=%v posts=%d cleanup=%d", got, err, provider.posts, failure.calls)
	}
	var saved bool
	if err := db.QueryRow("SELECT result_snapshot IS NOT NULL AND last_error IS NULL FROM deployment_step_attempts WHERE deployment_id=$1 AND step='create'", d.ID).Scan(&saved); err != nil || !saved {
		t.Fatalf("missing repaired create checkpoint: %v %v", saved, err)
	}
}
