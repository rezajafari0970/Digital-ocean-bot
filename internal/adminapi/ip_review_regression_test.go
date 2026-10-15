package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

func TestIPLifecycleReviewEvidenceAndFundingFences(t *testing.T) {
	t.Run("credential-rotation", func(t *testing.T) {
		db := adminTestDB(t)
		ipSeed(t, db)
		s, _, ids := ipApprovedBatch(t, db, ipa, 4)
		sqlMust(t, db, "UPDATE secrets SET updated_at=updated_at+interval '1 second' WHERE account_id=$1", ipa)
		if _, e := s.Dispatch(context.Background(), ids[0], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrEvidence) {
			t.Fatal("old credential evidence admitted", e)
		}
	})
	t.Run("currency-and-funding-revision", func(t *testing.T) {
		db := adminTestDB(t)
		ipSeed(t, db)
		s, _, ids := ipApprovedBatch(t, db, ipa, 4)
		f := ipam.Funding{AccountID: ipa, Currency: "EUR", BudgetMicros: 1000000, FundedUntil: time.Now().Add(48 * time.Hour), MaxConcurrent: 2}
		if _, e := s.SaveFunding(context.Background(), f, 1); !errors.Is(e, ipam.ErrProtected) {
			t.Fatal("mixed currencies", e)
		}
		f.Currency = "USD"
		f.BudgetMicros = 0
		if _, e := s.SaveFunding(context.Background(), f, 1); e != nil {
			t.Fatal("lowering budget should preserve obligations", e)
		}
		if _, e := s.Dispatch(context.Background(), ids[0], ipGeneration(t, db, ipa)); e == nil {
			t.Fatal("expired funding revision")
		}
		var n int
		if e := db.QueryRow("SELECT count(*) FROM ip_funding_reservations WHERE settled_at IS NULL").Scan(&n); e != nil || n != 1 {
			t.Fatal("liability forgotten", n, e)
		}
	})
	t.Run("new-capability-invalidates-plan", func(t *testing.T) {
		db := adminTestDB(t)
		ipSeed(t, db)
		s, _, ids := ipApprovedBatch(t, db, ipa, 4)
		var raw []byte
		if e := db.QueryRow("SELECT body FROM ip_execution_evidence WHERE account_id=$1", ipa).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var v ipam.ExecutionEvidence
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		v.Supported = false
		v.ObservedAt = time.Now().UTC()
		if _, e := s.RecordExecutionEvidence(context.Background(), v); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Dispatch(context.Background(), ids[0], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrEvidence) {
			t.Fatal("superseded evidence reused", e)
		}
	})
}
func TestIPLifecycleReviewInvalidOutcomeAndBoundSelection(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	o, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	for _, status := range []int{0, 409, 503} {
		if e = s.RecordResponse(ctx, o.ID, "bad", ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "ACCEPTED", HTTPStatus: status, ResourceID: "8.8.8.8"}); !errors.Is(e, ipam.ErrEvidence) {
			t.Fatal("invalid success", status, e)
		}
	}
	if e = s.RecordResponse(ctx, o.ID, "conflict", ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "REJECTED", HTTPStatus: 409}); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("conflict treated as noncreation", e)
	}
	if e = s.RecordResponse(ctx, o.ID, "wrong-request", ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: strings.Repeat("0", 64), Outcome: "ACQUIRED", Contract: ipam.AcquisitionContract("digitalocean", "reserved", 4), HTTPStatus: 201, ResourceID: "8.8.8.8"}); !errors.Is(e, ipam.ErrDispatch) {
		t.Fatal("request mismatch", e)
	}
}
func TestIPLifecycleReviewPolicyCannotExpandAcquiredAuthority(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, a := ipOwned(t, db, ipa, 6, "", "2001:db8:10::/64")
	rec, e := s.Policy(ctx, ipa, 6)
	if e != nil {
		t.Fatal(e)
	}
	rec.Policy.DiversityBits = ipPtr(128)
	rec.Policy.LifetimeMaxSeconds = ipPtr(10000)
	if _, e = s.SavePolicy(ctx, ipa, rec.Revision, rec.Policy); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SelectEndpoint(ctx, a, "2001:db8:10::1"); !errors.Is(e, ipam.ErrRevision) {
		t.Fatal("settings widened approval", e)
	}
}
func TestIPLifecycleReviewRouteLeaseConflictAndFootprintLoss(t *testing.T) {
	db := adminTestDB(t)
	s, old, replacement, pub := ipPublicationFixture(t, db)
	ctx := context.Background()
	original := *pub.ClientValidUntil
	if _, e := s.Publish(ctx, pub); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{old.ID, replacement.ID} {
		now := time.Now().UTC()
		e := s.RecordProof(ctx, ipam.HealthProof{EndpointID: id, EndpointRevision: 1, NetworkRevision: "fixture-network", PanelID: pub.PanelID, ConfigRevision: pub.ConfigRevision, Port: 443, RouteClass: "RESIDENTIAL",
			ProviderAttached: true, GuestConfigured: true, ProtocolOK: true, RouteIsolationOK: true, UserNetworkOK: true, FallbackOK: true, ObservedAt: now, ExpiresAt: now.Add(time.Minute), EvidenceDigest: strings.Repeat("d", 64)})
		if e != nil {
			t.Fatal(e)
		}
	}
	pub.RouteClass = "RESIDENTIAL"
	later := original.Add(time.Minute)
	pub.ClientValidUntil = &later
	if out, e := s.Publish(ctx, pub); !errors.Is(e, ipam.ErrReceiptConflict) || len(out) > 0 {
		t.Fatal("lease upsert skipped", out, e)
	}
	var until time.Time
	if e := db.QueryRow("SELECT valid_until FROM ip_publication_leases WHERE endpoint_id=$1", old.ID).Scan(&until); e != nil || until.After(original.Add(time.Microsecond)) {
		t.Fatal("conflict partially committed", until, e)
	}
	sqlMust(t, db, "UPDATE ip_network_footprints SET state='UNKNOWN' WHERE endpoint_id=$1", replacement.ID)
	if e := s.BeginDrain(ctx, old.ID, replacement.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("unknown footprint accepted", e)
	}
	pub.RouteClass = "DIRECT"
	if out, e := s.Publish(ctx, pub); e != nil || len(out) != 1 || out[0].EndpointID != old.ID {
		t.Fatal("invalidated footprint published", out, e)
	}
}
func TestIPLifecycleReviewReplacementMustCoverRoutes(t *testing.T) {
	db := adminTestDB(t)
	s, old, replacement, pub := ipPublicationFixture(t, db)
	ctx := context.Background()
	now := time.Now().UTC()
	e := s.RecordProof(ctx, ipam.HealthProof{EndpointID: old.ID, EndpointRevision: 1, NetworkRevision: "fixture-network", PanelID: pub.PanelID, ConfigRevision: pub.ConfigRevision, Port: 443, RouteClass: "RESIDENTIAL",
		ProviderAttached: true, GuestConfigured: true, ProtocolOK: true, RouteIsolationOK: true, UserNetworkOK: true, FallbackOK: true, ObservedAt: now, ExpiresAt: now.Add(time.Minute), EvidenceDigest: strings.Repeat("e", 64)})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.BeginDrain(ctx, old.ID, replacement.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("DIRECT substituted for residential", e)
	}
	sqlMust(t, db, "UPDATE ip_allocations SET state='LOST' WHERE id=$1", replacement.AllocationID)
	if e = s.BeginDrain(ctx, old.ID, replacement.ID); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("lost replacement used", e)
	}
}
func TestIPLifecycleReviewDownMigrationRetainsNativeSendFence(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	sqlMust(t, db, "INSERT INTO ip_native_delete_fences(account_id,server_id) VALUES($1,'paused-before-send')", ipa)
	raw, e := os.ReadFile("../../migrations/000176_ip_lifecycle_journal.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(raw)); e == nil {
		t.Fatal("rollback erased native send barrier")
	}
	var n int
	if e = db.QueryRow("SELECT count(*) FROM ip_native_delete_fences").Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}

func TestIPLifecycleReviewSharedFamilyCapacityBeforeEitherDispatch(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	// First batch is only queued, so a separate-family batch must still reserve its shared slot.
	sqlMust(t, db, "UPDATE ip_mutation_lanes SET next_attempt_at=NULL WHERE account_id=$1", ipa)
	var raw []byte
	if e := db.QueryRow("SELECT body FROM ip_execution_evidence WHERE account_id=$1 AND family=4", ipa).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var evidence ipam.ExecutionEvidence
	if e := json.Unmarshal(raw, &evidence); e != nil {
		t.Fatal(e)
	}
	evidence.Family = 6
	evidence.ObservedAt = time.Now().UTC()
	id, e := s.RecordExecutionEvidence(ctx, evidence)
	if e != nil {
		t.Fatal(e)
	}
	p := ipPolicy(6)
	saved, e := s.SavePolicy(ctx, ipa, 0, p)
	if e != nil {
		t.Fatal(e)
	}
	var original []byte
	if e = db.QueryRow("SELECT p.body FROM ip_plans p JOIN ip_approvals a ON a.plan_digest=p.digest JOIN ip_mutation_intents m ON m.approval_id=a.id WHERE m.id=$1", ids[0]).Scan(&original); e != nil {
		t.Fatal(e)
	}
	var plan ipam.Plan
	if e = json.Unmarshal(original, &plan); e != nil {
		t.Fatal(e)
	}
	input := plan.Input
	input.Policy = p
	input.PolicyRevision = saved.Revision
	input.EvidenceIDs = map[string]string{"server-1": id}
	input.Candidates[0].Family = 6
	if e = db.QueryRow("SELECT sum(commitment_micros) FROM ip_funding_reservations").Scan(&input.ExistingCommitmentMicros); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	newPlan := ipam.Preview(input, now)
	if e = s.SavePlan(ctx, newPlan); e != nil {
		t.Fatal(e, newPlan.Blockers)
	}
	approval, e := s.Approve(ctx, ipam.ApprovalRequest{PlanDigest: newPlan.Digest, ApprovedBy: "test second family", ExpiresAt: now.Add(time.Minute), FundedUntil: now.Add(24 * time.Hour), MaxCommitmentMicros: 1000000})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.QueueApproval(ctx, approval); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("shared family quota overbooked", e)
	}
	if _, e = s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
		t.Fatal("valid first reservation lost", e)
	}
}
func TestIPLifecycleReviewCleanupArchivePreservesAADAndAccount(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	vault, e := secrets.NewStore(secrets.SQLRepository{DB: db}, make([]byte, 32), 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = vault.Put(ctx, ipa, "test", "provider-token", []byte("fixture-old-token")); e != nil {
		t.Fatal(e)
	}
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	if _, e = s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); e != nil {
		t.Fatal(e)
	}
	if e = vault.Put(ctx, ipa, "test", "provider-token", []byte("fixture-new-token")); e != nil {
		t.Fatal(e)
	}
	archived, e := vault.GetIPCleanup(ctx, ipa, ids[0])
	if e != nil || string(archived) != "fixture-old-token" {
		t.Fatal("archived credential failed authenticated decryption", e)
	}
	current, e := vault.Get(ctx, ipa, "test")
	if e != nil || string(current) != "fixture-new-token" {
		t.Fatal("current credential overwritten", e)
	}
	if _, e = vault.GetIPCleanup(ctx, ipb, ids[0]); e == nil {
		t.Fatal("cross-account credential read")
	}
	if _, e = db.Exec("DELETE FROM ip_cleanup_credentials WHERE operation_id=$1", ids[0]); e == nil {
		t.Fatal("unresolved cleanup credential erased")
	}
	sqlMust(t, db, "DELETE FROM secrets WHERE account_id=$1", ipa)
	if _, e = vault.GetIPCleanup(ctx, ipa, ids[0]); e != nil {
		t.Fatal("current-secret removal lost cleanup", e)
	}
}
