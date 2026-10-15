package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"testing"
	"time"
)

func ipWaitBlocked(t *testing.T, db *sql.DB, fragment string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if e := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND cardinality(pg_blocking_pids(pid))>0 AND query LIKE '%'||$1||'%')", fragment).Scan(&blocked); e != nil {
			t.Fatal(e)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected database interleaving did not block", fragment)
}
func TestIPLifecycleR2DispatchSpacing(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	p := ipPolicy(4)
	p.Desired = ipPtr(2)
	p.PerRun = ipPtr(2)
	p.MaxConcurrent = ipPtr(2)
	s, _, ids := ipApprovedBatch(t, db, ipa, 4, p)
	if _, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Dispatch(ctx, ids[1], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("batch bypassed send spacing", e)
	}
	var next time.Time
	if e := db.QueryRow("SELECT next_dispatch_at FROM ip_mutation_lanes WHERE account_id=$1", ipa).Scan(&next); e != nil || time.Until(next) < 29*time.Second {
		t.Fatal(next, e)
	}
	// Same account, opposite-family lane must impose the send gate as well.
	sqlMust(t, db, "UPDATE ip_mutation_lanes SET family=6 WHERE account_id=$1", ipa)
	if _, e := s.Dispatch(ctx, ids[1], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("cross-family send spacing", e)
	}
}
func TestIPLifecycleR2PricedHorizonAndAsyncOwnership(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, approval, ids := ipApprovedBatch(t, db, ipa, 4)
	var digest string
	if e := db.QueryRow("SELECT plan_digest FROM ip_approvals WHERE id=$1", approval).Scan(&digest); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Approve(ctx, ipam.ApprovalRequest{PlanDigest: digest, ApprovedBy: "fixture", ExpiresAt: time.Now().Add(30 * time.Second), FundedUntil: time.Now().Add(47 * time.Hour), MaxCommitmentMicros: 1000000}); !errors.Is(e, ipam.ErrApproval) {
		t.Fatal("unpriced funding extension", e)
	}
	op, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	ack := ipam.DispatchReceipt{DispatchID: op.DispatchID, RequestDigest: op.RequestDigest, Outcome: "ACKNOWLEDGED", HTTPStatus: 202, ResourceID: "unverified"}
	if e = s.RecordResponse(ctx, op.ID, "ack", ack); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ConfirmAcquisition(ctx, op.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("ack became ownership", e)
	}
	ack.Outcome = "ACQUIRED"
	ack.HTTPStatus = 201
	ack.Contract = ipam.AcquisitionContract("vultr", "reserved", 4)
	if e = s.RecordResponse(ctx, op.ID, "wrong-contract", ack); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("cross-provider contract", e)
	}
	ack.Outcome = "REJECTED"
	ack.HTTPStatus = 422
	if e = s.RecordResponse(ctx, op.ID, "generic-4xx", ack); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("unclassified 4xx released obligations", e)
	}
}
func TestIPLifecycleR2KeyRotationRetainsOldCleanupKey(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 42
	old, e := secrets.NewStore(secrets.SQLRepository{DB: db}, key1, 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = old.Put(ctx, ipa, "test", "provider-token", []byte("fixture-original")); e != nil {
		t.Fatal(e)
	}
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	if _, e = s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
		t.Fatal(e)
	}
	rotated, e := secrets.NewStoreWithKeys(secrets.SQLRepository{DB: db}, key2, 2, map[int][]byte{1: key1})
	if e != nil {
		t.Fatal(e)
	}
	if e = rotated.Put(ctx, ipa, "test", "provider-token", []byte("fixture-new")); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "DELETE FROM secrets WHERE account_id=$1", ipa)
	got, e := rotated.GetIPCleanup(ctx, ipa, ids[0])
	if e != nil || string(got) != "fixture-original" {
		t.Fatal("old key/AAD lost", e)
	}
	if _, e = secrets.NewStoreWithKeys(secrets.SQLRepository{DB: db}, key2, 2, nil); e == nil {
		t.Fatal("retired obligated key")
	}
}
func TestIPLifecycleR2CredentialWriterCannotCrossSnapshot(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	vault, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = vault.Put(ctx, ipa, "test", "provider-token", []byte("fixture-admitted")); e != nil {
		t.Fatal(e)
	}
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	generation := ipGeneration(t, db, ipa)
	barrier, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer barrier.Close()
	if _, e = barrier.ExecContext(ctx, "SELECT pg_advisory_lock(191762)"); e != nil {
		t.Fatal(e)
	}
	defer barrier.ExecContext(ctx, "SELECT pg_advisory_unlock(191762)")
	sqlMust(t, db, `CREATE FUNCTION ip_pause_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(191762); RETURN NEW; END$$;
 CREATE TRIGGER ip_pause_snapshot BEFORE INSERT ON ip_cleanup_credentials FOR EACH ROW EXECUTE FUNCTION ip_pause_snapshot()`)
	dispatchDone := make(chan error, 1)
	go func() { _, e := s.Dispatch(ctx, ids[0], generation); dispatchDone <- e }()
	ipWaitBlocked(t, db, "INSERT INTO ip_cleanup_credentials")
	rotateDone := make(chan error, 1)
	go func() { rotateDone <- vault.Put(ctx, ipa, "test", "provider-token", []byte("fixture-rotated")) }()
	ipWaitBlocked(t, db, "INSERT INTO secrets")
	if _, e = barrier.ExecContext(ctx, "SELECT pg_advisory_unlock(191762)"); e != nil {
		t.Fatal(e)
	}
	if e = <-dispatchDone; e != nil {
		t.Fatal(e)
	}
	if e = <-rotateDone; e != nil {
		t.Fatal(e)
	}
	got, e := vault.GetIPCleanup(ctx, ipa, ids[0])
	if e != nil || string(got) != "fixture-admitted" {
		t.Fatal("different generation archived", e)
	}
}
func TestIPLifecycleR2ConfigWriterFencesPublicationAndDrain(t *testing.T) {
	for _, method := range []string{"publish", "drain", "retire"} {
		t.Run(method, func(t *testing.T) {
			db := adminTestDB(t)
			ctx := context.Background()
			s, old, replacement, pub := ipPublicationFixture(t, db)
			if method == "retire" {
				if e := s.BeginDrain(ctx, old.ID, replacement.ID); e != nil {
					t.Fatal(e)
				}
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			writerDone := make(chan error, 1)
			go func() {
				writerDone <- sanaei.WithConfigLock(ctx, db, pub.PanelID, func(context.Context) error {
					close(entered)
					<-release
					_, e := db.Exec("UPDATE panel_routing_state SET state='APPLYING' WHERE panel_id=$1", pub.PanelID)
					return e
				})
			}()
			<-entered
			done := make(chan error, 1)
			go func() {
				var e error
				switch method {
				case "publish":
					var out []ipam.PublishedEndpoint
					out, e = s.Publish(ctx, pub)
					if len(out) > 0 {
						e = errors.New("stale configuration escaped")
					}
				case "drain":
					e = s.BeginDrain(ctx, old.ID, replacement.ID)
				case "retire":
					e = s.Retire(ctx, old.ID)
				}
				done <- e
			}()
			ipWaitBlocked(t, db, "pg_advisory_xact_lock_shared")
			close(release)
			if e := <-writerDone; e != nil {
				t.Fatal(e)
			}
			e := <-done
			if method == "publish" {
				if e != nil {
					t.Fatal(e)
				}
			} else if !errors.Is(e, ipam.ErrProtected) {
				t.Fatal("stale retirement", e)
			}
		})
	}
}
func TestIPLifecycleR2InventoryValidityCannotExtend(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, _ := ipApprovedBatch(t, db, ipa, 4)
	var raw []byte
	if e := db.QueryRow("SELECT body FROM ip_execution_evidence WHERE account_id=$1", ipa).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var ev ipam.ExecutionEvidence
	if e := json.Unmarshal(raw, &ev); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, ipa)
	inv := ipam.InventoryStore{DB: db}
	tick, e := inv.Begin(ctx, ipa)
	if e != nil {
		t.Fatal(e)
	}
	// An observer may complete late; its later assessment cannot refresh the old read.
	tick.StartedAt = time.Now().Add(-299 * time.Second)
	obs := ipObs(ipPtr(10))
	obs.CredentialGeneration = ipGeneration(t, db, ipa)
	obs.ObservedAt = tick.StartedAt
	if e = inv.Finish(ctx, tick, &obs, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	ev.InventoryRequestID = tick.RequestID
	ev.ObservedAt = time.Now()
	ev.ValidUntil = time.Now().Add(4 * time.Minute)
	id, e := s.RecordExecutionEvidence(ctx, ev)
	if e != nil {
		t.Fatal(e)
	}
	var expires time.Time
	if e = db.QueryRow("SELECT valid_until FROM ip_execution_evidence WHERE id=$1", id).Scan(&expires); e != nil {
		t.Fatal(e)
	}
	if expires.After(tick.StartedAt.Add(5*time.Minute + time.Microsecond)) {
		t.Fatal("inventory freshness extended", expires)
	}
	time.Sleep(time.Until(expires) + 10*time.Millisecond)
	ev.ObservedAt = time.Now()
	ev.ValidUntil = time.Now().Add(time.Minute)
	if _, e = s.RecordExecutionEvidence(ctx, ev); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("expired inventory reauthorized", e)
	}
}
func TestIPLifecycleR2EmptyAndStaleGenerationCannotOwn(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	op, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RecordResponse(ctx, op.ID, "created", ipCreatedReceipt(t, s, op, "created", "fixture-new")); e != nil {
		t.Fatal(e)
	}
	for _, g := range []string{"", "stale", ipGeneration(t, db, ipa)} {
		ipAllow(t, db, ipa)
		inv := ipam.InventoryStore{DB: db}
		tick, e := inv.Begin(ctx, ipa)
		if e != nil {
			t.Fatal(e)
		}
		obs := ipObs(ipPtr(10))
		obs.CredentialGeneration = g
		obs.Resources = []providers.IPResource{{ID: "fixture-new", Family: 4, Kind: "reserved", Address: "192.0.2.10", ServerID: "server-1", Region: "region-1", Protected: true}}
		if e = inv.Finish(ctx, tick, &obs, "", 0, 0); e != nil {
			t.Fatal(e)
		}
		_, e = s.ConfirmAcquisition(ctx, op.ID, tick.RequestID)
		if g == "" || g == "stale" {
			if !errors.Is(e, ipam.ErrEvidence) {
				t.Fatal("untrusted generation owned", e)
			}
		} else {
			if e != nil {
				t.Fatal(e)
			}
			var retained bool
			if e = db.QueryRow("SELECT EXISTS(SELECT 1 FROM ip_lifecycle_inventory_receipts WHERE request_id=$1)", tick.RequestID).Scan(&retained); e != nil || !retained {
				t.Fatal("positive receipt unretained", e)
			}
		}
	}
}

func TestIPLifecycleRecoverySerializesWithAdmission(t *testing.T) {
	for _, recoveryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "dispatch-first", true: "recovery-first"}[recoveryFirst], func(t *testing.T) {
			db := adminTestDB(t)
			ipSeed(t, db)
			ctx := context.Background()
			p := ipPolicy(4)
			p.Desired = ipPtr(2)
			p.PerRun = ipPtr(2)
			p.MaxConcurrent = ipPtr(2)
			s, _, ids := ipApprovedBatch(t, db, ipa, 4, p)
			if _, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
				t.Fatal(e)
			}
			sqlMust(t, db, "UPDATE ip_mutation_lanes SET next_dispatch_at=NULL")
			cutoff := time.Now()
			if recoveryFirst {
				if e := s.RecoverDispatched(ctx, ipa, cutoff); e != nil {
					t.Fatal(e)
				}
				if _, e := s.Dispatch(ctx, ids[1], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrProtected) {
					t.Fatal("UNKNOWN barrier bypassed", e)
				}
				return
			}
			gate, e := db.Conn(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer gate.Close()
			if _, e = gate.ExecContext(ctx, "SELECT pg_advisory_lock(191763)"); e != nil {
				t.Fatal(e)
			}
			defer gate.ExecContext(ctx, "SELECT pg_advisory_unlock(191763)")
			sqlMust(t, db, `CREATE FUNCTION ip_pause_second() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(191763); RETURN NEW; END$$;
 CREATE TRIGGER ip_pause_second BEFORE INSERT ON ip_cleanup_credentials FOR EACH ROW EXECUTE FUNCTION ip_pause_second()`)
			dispatchDone := make(chan error, 1)
			go func() { _, e := s.Dispatch(ctx, ids[1], ipGeneration(t, db, ipa)); dispatchDone <- e }()
			ipWaitBlocked(t, db, "INSERT INTO ip_cleanup_credentials")
			recoveryDone := make(chan error, 1)
			go func() { recoveryDone <- s.RecoverDispatched(ctx, ipa, cutoff) }()
			ipWaitBlocked(t, db, "pg_advisory_xact_lock")
			var before string
			if e = db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ids[0]).Scan(&before); e != nil || before != "DISPATCHED" {
				t.Fatal("recovery crossed dispatch fence", before, e)
			}
			if _, e = gate.ExecContext(ctx, "SELECT pg_advisory_unlock(191763)"); e != nil {
				t.Fatal(e)
			}
			if e = <-dispatchDone; e != nil {
				t.Fatal(e)
			}
			if e = <-recoveryDone; e != nil {
				t.Fatal(e)
			}
			var after string
			if e = db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ids[0]).Scan(&after); e != nil || after != "UNKNOWN" {
				t.Fatal(after, e)
			}
		})
	}
}

func TestIPLifecycleRecoverySerializesWithFreshQueue(t *testing.T) {
	for _, recoveryFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "queue-first", true: "recovery-first"}[recoveryFirst], func(t *testing.T) {
			db := adminTestDB(t)
			ipSeed(t, db)
			ctx := context.Background()
			p := ipPolicy(4)
			p.Desired = ipPtr(2)
			p.PerRun = ipPtr(1)
			p.MaxConcurrent = ipPtr(2)
			s, first, ids := ipApprovedBatch(t, db, ipa, 4, p)
			if _, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
				t.Fatal(e)
			}
			sqlMust(t, db, "UPDATE ip_mutation_lanes SET next_attempt_at=NULL")
			var raw []byte
			if e := db.QueryRow("SELECT p.body FROM ip_plans p JOIN ip_approvals g ON g.plan_digest=p.digest WHERE g.id=$1", first).Scan(&raw); e != nil {
				t.Fatal(e)
			}
			var original ipam.Plan
			if e := json.Unmarshal(raw, &original); e != nil {
				t.Fatal(e)
			}
			in := original.Input
			in.Reserved = 1
			if e := db.QueryRow("SELECT sum(commitment_micros) FROM ip_funding_reservations").Scan(&in.ExistingCommitmentMicros); e != nil {
				t.Fatal(e)
			}
			now := time.Now()
			plan := ipam.Preview(in, now)
			if e := s.SavePlan(ctx, plan); e != nil {
				t.Fatal(e, plan.Blockers)
			}
			approval, e := s.Approve(ctx, ipam.ApprovalRequest{PlanDigest: plan.Digest, ApprovedBy: "fresh second queue fixture", ExpiresAt: now.Add(time.Minute), FundedUntil: now.Add(24 * time.Hour), MaxCommitmentMicros: 1000000})
			if e != nil {
				t.Fatal(e)
			}
			cutoff := time.Now()
			if recoveryFirst {
				if e = s.RecoverDispatched(ctx, ipa, cutoff); e != nil {
					t.Fatal(e)
				}
				if got, e := s.QueueApproval(ctx, approval); !errors.Is(e, ipam.ErrLaneBusy) || len(got) > 0 {
					t.Fatal("UNKNOWN allowed fresh batch", got, e)
				}
				return
			}
			gate, e := db.Conn(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer gate.Close()
			if _, e = gate.ExecContext(ctx, "SELECT pg_advisory_lock(191764)"); e != nil {
				t.Fatal(e)
			}
			defer gate.ExecContext(ctx, "SELECT pg_advisory_unlock(191764)")
			sqlMust(t, db, `CREATE FUNCTION ip_pause_queue() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN PERFORM pg_advisory_xact_lock(191764); RETURN NEW; END$$;
 CREATE TRIGGER ip_pause_queue BEFORE INSERT ON ip_mutation_intents FOR EACH ROW EXECUTE FUNCTION ip_pause_queue()`)
			queued := make(chan error, 1)
			go func() {
				got, e := s.QueueApproval(ctx, approval)
				if e == nil && len(got) != 1 {
					e = errors.New("wrong fresh batch")
				}
				queued <- e
			}()
			ipWaitBlocked(t, db, "INSERT INTO ip_mutation_intents")
			recovered := make(chan error, 1)
			go func() { recovered <- s.RecoverDispatched(ctx, ipa, cutoff) }()
			ipWaitBlocked(t, db, "pg_advisory_xact_lock")
			var state string
			if e = db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ids[0]).Scan(&state); e != nil || state != "DISPATCHED" {
				t.Fatal("recovery crossed queue", state, e)
			}
			if _, e = gate.ExecContext(ctx, "SELECT pg_advisory_unlock(191764)"); e != nil {
				t.Fatal(e)
			}
			if e = <-queued; e != nil {
				t.Fatal(e)
			}
			if e = <-recovered; e != nil {
				t.Fatal(e)
			}
			var n int
			if e = db.QueryRow("SELECT count(*) FROM ip_mutation_intents").Scan(&n); e != nil || n != 2 {
				t.Fatal(n, e)
			}
		})
	}
}
func TestIPLifecycleReadbackRequiresMatchingAcknowledgement(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	op, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	good := ipCreatedReceipt(t, s, op, "response", "192.0.2.10")
	if e = s.RecordResponse(ctx, op.ID, "response", good); e != nil {
		t.Fatal(e)
	}
	bad := good
	bad.ResourceID = "192.0.2.11"
	if e = s.RecordResponse(ctx, op.ID, "response", bad); !errors.Is(e, ipam.ErrReceiptConflict) {
		t.Fatal("immutable key conflict", e)
	}
	if e = s.RecordResponse(ctx, op.ID, "new-key", bad); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("unmatched acknowledgement accepted", e)
	}
}
