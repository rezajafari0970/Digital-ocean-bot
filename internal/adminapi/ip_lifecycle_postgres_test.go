package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func ipPtr[T any](v T) *T { return &v }
func ipPolicy(family int) ipam.Policy {
	p := ipam.Policy{Enabled: true, Family: family, Product: "reserved", Desired: ipPtr(1), PerRun: ipPtr(1), MaxConcurrent: ipPtr(1), SpacingMinSeconds: ipPtr(30), SpacingMaxSeconds: ipPtr(60), LifetimeMinSeconds: ipPtr(3600), LifetimeMaxSeconds: ipPtr(7200), ActiveScope: "GLOBAL", HistoryScope: "GLOBAL", HistorySeconds: ipPtr(86400), Currency: "USD", BudgetMicros: ipPtr(int64(1000000)), FundedHorizonSeconds: ipPtr(86400)}
	if family == 6 {
		p.DiversityBits = ipPtr(64)
		p.AllocationBits = ipPtr(64)
	}
	return p
}
func ipApprovedBatch(t *testing.T, db *sql.DB, account string, family int, custom ...ipam.Policy) (ipam.Store, string, []string) {
	t.Helper()
	ctx := context.Background()
	s := ipam.Store{DB: db}
	p := ipPolicy(family)
	if len(custom) > 0 {
		p = custom[0]
	}
	saved, e := s.SavePolicy(ctx, account, 0, p)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	funding, e := s.SaveFunding(ctx, ipam.Funding{AccountID: account, Currency: "USD", BudgetMicros: 1000000, FundedUntil: now.Add(48 * time.Hour), MaxConcurrent: 2}, 0)
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES('test',$1,'fixture',decode('00','hex'),decode('00','hex'),1) ON CONFLICT DO NOTHING", account)
	sqlMust(t, db, "INSERT INTO network_profiles(id,account_id,mode) VALUES(gen_random_uuid(),$1,'direct') ON CONFLICT DO NOTHING", account)
	generation, e := ipam.CredentialGeneration(ctx, db, account)
	if e != nil {
		t.Fatal(e)
	}
	inv := ipam.InventoryStore{DB: db}
	tick, e := inv.Begin(ctx, account)
	if e != nil {
		t.Fatal(e)
	}
	obs := ipObs(ipPtr(10))
	obs.CredentialGeneration = generation
	obs.Quotas[0].Family = family
	if e = inv.Finish(ctx, tick, &obs, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	var provider string
	if e = db.QueryRow("SELECT provider FROM accounts WHERE id=$1", account).Scan(&provider); e != nil {
		t.Fatal(e)
	}
	evidence, e := s.RecordExecutionEvidence(ctx, ipam.ExecutionEvidence{AccountID: account, Generation: generation, Provider: provider, Family: family, Product: "reserved", Region: "region-1", ServerID: "server-1", InventoryRequestID: tick.RequestID, Supported: true, Healthy: true, NetworkImpact: "test-only; no network", ObservedAt: time.Now().UTC(), ValidUntil: now.Add(4 * time.Minute),
		Capacities: []ipam.CapacityEvidence{{Scope: "account:reserved", Family: 0, Product: "reserved", Limit: ipPtr(*p.Desired), Used: ipPtr(0), Source: "test fixture shared quota"}, {Scope: "server:server-1", Family: 0, ServerID: "server-1", Limit: ipPtr(2), Used: ipPtr(0), Source: "test fixture shared attachment"}}})
	if e != nil {
		t.Fatal(e)
	}
	in := ipam.PlanInput{AccountID: account, PolicyRevision: saved.Revision, Policy: p, Quota: ipPtr(10), InventoryAt: obs.ObservedAt, CredentialGeneration: generation, EvidenceIDs: map[string]string{"server-1": evidence}, FundingRevision: funding.Revision,
		Quote:      ipam.Quote{Currency: "USD", HourlyMicros: ipPtr(int64(1000)), DetachedHourlyMicros: ipPtr(int64(10000)), GranularitySeconds: 3600, Source: "test-only quote", ExpiresAt: now.Add(time.Hour)},
		Candidates: []ipam.Candidate{{AccountID: account, ServerID: "server-1", Region: "region-1", Healthy: true, Supported: true, CapabilityKnown: true, CapabilityAt: now, CredentialGeneration: generation, Family: family, Product: "reserved", Slots: ipPtr(2), NetworkImpact: "test-only; no network"}}}
	now = time.Now().UTC()
	plan := ipam.Preview(in, now)
	if e = s.SavePlan(ctx, plan); e != nil {
		t.Fatal(e, plan.Blockers)
	}
	approval, e := s.Approve(ctx, ipam.ApprovalRequest{PlanDigest: plan.Digest, ApprovedBy: "isolated test fixture", ExpiresAt: now.Add(time.Minute), FundedUntil: now.Add(24 * time.Hour), MaxCommitmentMicros: 1000000})
	if e != nil {
		t.Fatal(e)
	}
	ids, e := s.QueueApproval(ctx, approval)
	if e != nil {
		t.Fatal(e)
	}
	return s, approval, ids
}
func ipCreatedReceipt(t *testing.T, s ipam.Store, o ipam.Operation, key, resource string) ipam.DispatchReceipt {
	t.Helper()
	status := 201
	if o.Provider == "linode" {
		status = 200
	}
	r := ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "ACQUIRED", Contract: ipam.AcquisitionContract(o.Provider, o.Product, o.Family), HTTPStatus: status, ResourceID: resource}
	if o.Provider == "digitalocean" && o.Family == 4 {
		ack := ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "ACKNOWLEDGED", HTTPStatus: 202, ResourceID: resource}
		if e := s.RecordResponse(context.Background(), o.ID, key+"-ack", ack); e != nil {
			t.Fatal(e)
		}
		r.HTTPStatus = 200
		r.AcknowledgementKey = key + "-ack"
	}
	return r
}
func ipGeneration(t *testing.T, db *sql.DB, account string) string {
	t.Helper()
	g, e := ipam.CredentialGeneration(context.Background(), db, account)
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func TestIPLifecyclePolicyCASAndNoDefaults(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	s := ipam.Store{DB: db}
	ctx := context.Background()
	p, e := s.Policy(ctx, ipa, 6)
	if e != nil || p.Revision != 0 || p.Policy.Enabled || p.Policy.Desired != nil || p.Policy.DiversityBits != nil {
		t.Fatal(p, e)
	}
	saved, e := s.SavePolicy(ctx, ipa, 0, ipPolicy(6))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SavePolicy(ctx, ipa, 0, ipPolicy(6)); !errors.Is(e, ipam.ErrRevision) {
		t.Fatal("lost update", e)
	}
	disabled := ipam.Policy{Family: 6}
	if _, e = s.SavePolicy(ctx, ipa, saved.Revision, disabled); e != nil {
		t.Fatal(e)
	}
	untouched, e := s.Policy(ctx, ipa, 4)
	if e != nil || untouched.Revision != 0 {
		t.Fatal("family coupling", e)
	}
	var events int
	if e = db.QueryRow("SELECT count(*) FROM ip_policy_events").Scan(&events); e != nil || events != 2 {
		t.Fatal(events, e)
	}
}
func TestIPLifecycleOneShotDispatchLateResponseAndRestart(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, approval, ids := ipApprovedBatch(t, db, ipa, 4)
	replay, e := s.QueueApproval(ctx, approval)
	if e != nil || len(replay) != 1 || replay[0] != ids[0] {
		t.Fatal(replay, e)
	}
	var wg sync.WaitGroup
	wins := make(chan ipam.Operation, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
			if e == nil {
				wins <- o
			} else if !errors.Is(e, ipam.ErrDispatch) {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(wins)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if len(wins) != 1 {
		t.Fatal("duplicate dispatch", len(wins))
	}
	ticket := <-wins
	// Worker paused after authorization but before send; restart/takeover must not send again.
	restart := ipam.Store{DB: db}
	if e = restart.RecoverDispatched(ctx, ipa, time.Now().Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	if _, e = restart.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrDispatch) {
		t.Fatal("restart replay", e)
	}
	// Independent account keeps making progress.
	other, _, otherIDs := ipApprovedBatch(t, db, ipb, 4)
	if _, e = other.Dispatch(ctx, otherIDs[0], ipGeneration(t, db, ipb)); e != nil {
		t.Fatal("cross-account blocking", e)
	}
	unknown := ipam.DispatchReceipt{DispatchID: ticket.DispatchID, RequestDigest: ticket.RequestDigest, Outcome: "UNKNOWN", ErrorClass: "transport"}
	if e = s.RecordResponse(ctx, ticket.ID, "timeout", unknown); e != nil {
		t.Fatal(e)
	}
	late := ipCreatedReceipt(t, s, ticket, "response", "8.8.4.4")
	if e = restart.RecordResponse(ctx, ticket.ID, "response", late); e != nil {
		t.Fatal(e)
	}
	if e = restart.RecordResponse(ctx, ticket.ID, "response", late); e != nil {
		t.Fatal("identical receipt", e)
	}
	bad := late
	bad.ResourceID = "8.8.8.8"
	if e = restart.RecordResponse(ctx, ticket.ID, "response", bad); !errors.Is(e, ipam.ErrReceiptConflict) {
		t.Fatal("rewritten receipt", e)
	}
	var state string
	var n int
	if e = db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ticket.ID).Scan(&state); e != nil || state != "ACCEPTED" {
		t.Fatal(state, e)
	}
	if e = db.QueryRow("SELECT count(*) FROM ip_mutation_receipts WHERE operation_id=$1", ticket.ID).Scan(&n); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	if _, e = db.Exec("UPDATE ip_mutation_receipts SET body='{}' WHERE operation_id=$1", ticket.ID); e == nil {
		t.Fatal("ledger mutable")
	}
}
func TestIPLifecycleMissingQuotaAndPausedDeleteFence(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	sqlMust(t, db, "INSERT INTO ip_native_delete_fences(account_id,server_id) VALUES($1,'server-1')", ipa)
	if _, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("native delete crossed", e)
	}
	sqlMust(t, db, "DELETE FROM ip_native_delete_fences")
	if _, e := s.Dispatch(ctx, ids[0], "wrong-generation"); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("generation drift", e)
	}
	if _, e := db.Exec("DELETE FROM accounts WHERE id=$1", ipa); e == nil {
		t.Fatal("pending obligation lost on purge")
	}
}
func TestIPLifecycleRateLimitAndReceiptCommitFailure(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	o, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, `CREATE FUNCTION fixture_reject_ip_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$;
 CREATE TRIGGER fixture_receipt BEFORE INSERT ON ip_mutation_receipts FOR EACH ROW EXECUTE FUNCTION fixture_reject_ip_receipt()`)
	r := ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "UNKNOWN", HTTPStatus: 429, ErrorClass: "rate_limited", RetryAfterSeconds: 7200}
	if e = s.RecordResponse(ctx, o.ID, "response", r); e == nil {
		t.Fatal("uncommitted success")
	}
	var state string
	if e = db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", o.ID).Scan(&state); e != nil || state != "DISPATCHED" {
		t.Fatal(state, e)
	}
	sqlMust(t, db, "DROP TRIGGER fixture_receipt ON ip_mutation_receipts")
	if e = s.RecordResponse(ctx, o.ID, "response", r); e != nil {
		t.Fatal(e)
	}
	var first, second time.Time
	if e = db.QueryRow("SELECT rate_limit_until FROM ip_mutation_lanes WHERE account_id=$1 AND family=4", ipa).Scan(&first); e != nil {
		t.Fatal(e)
	}
	if e = s.RecordResponse(ctx, o.ID, "response", r); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow("SELECT rate_limit_until FROM ip_mutation_lanes WHERE account_id=$1 AND family=4", ipa).Scan(&second); e != nil || !first.Equal(second) || time.Until(first) < 119*time.Minute {
		t.Fatal(first, second, e)
	}
	r.HTTPStatus = 503
	r.Outcome = "REJECTED"
	if e = s.RecordResponse(ctx, o.ID, "unsafe-reject", r); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("503 definitive failure", e)
	}
}
func ipOwned(t *testing.T, db *sql.DB, account string, family int, address, prefix string) (ipam.Store, string) {
	t.Helper()
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, account, family)
	op, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, account))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RecordResponse(ctx, op.ID, "response", ipCreatedReceipt(t, s, op, "response", "resource-1")); e != nil {
		t.Fatal(e)
	}
	ipAllow(t, db, account)
	inv := ipam.InventoryStore{DB: db}
	tick, e := inv.Begin(ctx, account)
	if e != nil {
		t.Fatal(e)
	}
	o := providers.IPObservation{CredentialGeneration: ipGeneration(t, db, account), Complete: true, ObservedAt: time.Now().UTC(), Resources: []providers.IPResource{{ID: "resource-1", Family: family, Kind: "reserved", Address: address, Prefix: prefix, ServerID: "server-1", Region: "region-1", Protected: true}}, Quotas: []providers.IPQuota{}}
	if e = inv.Finish(ctx, tick, &o, "", 0, 0); e != nil {
		t.Fatal(e)
	}
	id, e := s.ConfirmAcquisition(ctx, op.ID, tick.RequestID)
	if e != nil {
		t.Fatal(e)
	}
	return s, id
}
func TestIPLifecycleOwnershipAndSubnetDiversity(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, allocation := ipOwned(t, db, ipa, 6, "", "2001:db8:10::/64")
	first, e := s.SelectEndpoint(ctx, allocation, "2001:0db8:10:0::100")
	if e != nil {
		t.Fatal(e)
	}
	if first.Address != "2001:db8:10::100" || first.DiversityPrefix != "2001:db8:10::/64" || first.ExpiresAt.Sub(first.AcquiredAt) < time.Hour {
		t.Fatal(first)
	}
	if _, e = s.SelectEndpoint(ctx, allocation, "2001:db8:10::101"); !errors.Is(e, ipam.ErrDiversity) {
		t.Fatal("last octet mistaken as diversity", e)
	}
	if _, e = s.SelectEndpoint(ctx, allocation, "2001:db8:11::1"); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("outside owned prefix", e)
	}
	var state string
	var n int
	if e = db.QueryRow("SELECT state FROM ip_allocations WHERE id=$1", allocation).Scan(&state); e != nil || state != "OWNED" {
		t.Fatal("parent discarded", state, e)
	}
	if e = db.QueryRow("SELECT count(*) FROM ip_endpoint_history WHERE allocation_id=$1", allocation).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	if _, e = db.Exec("DELETE FROM accounts WHERE id=$1", ipa); e == nil {
		t.Fatal("owned resource purged")
	}
}
func TestIPLifecycleUnknownAcquisitionCannotAdoptInventory(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipApprovedBatch(t, db, ipa, 4)
	o, e := s.Dispatch(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RecordResponse(ctx, o.ID, "timeout", ipam.DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: "UNKNOWN", ErrorClass: "transport"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ConfirmAcquisition(ctx, o.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(e, ipam.ErrProtected) {
		t.Fatal("inventory inferred ownership", e)
	}
}
