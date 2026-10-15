package adminapi

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ipwire"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ipWireApprovedBatch(t *testing.T, db *sql.DB, account string, family int, custom ...ipam.Policy) (ipam.Store, string, []string) {
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
	server := "123"
	if provider == "vultr" {
		server = "11111111-1111-4111-8111-111111111111"
	}
	spec := ipwire.Spec{Provider: provider, Family: family, Product: "reserved", Action: "ALLOCATE", ServerID: server, Region: "nyc3"}
	if family == 6 && provider == "vultr" {
		spec.PrefixLength = 64
	}
	prepared, err := ipwire.Prepare(spec)
	if err != nil {
		t.Fatal(err)
	}
	wire := ipam.WireRequest{Spec: spec, Digest: prepared.Digest()}
	evidence, e := s.RecordExecutionEvidence(ctx, ipam.ExecutionEvidence{WireRequest: &wire, AccountID: account, Generation: generation, Provider: provider, Family: family, Product: "reserved", Region: "nyc3", ServerID: server, InventoryRequestID: tick.RequestID, Supported: true, Healthy: true, NetworkImpact: "DETACHED_ALLOCATION", ObservedAt: time.Now().UTC(), ValidUntil: now.Add(4 * time.Minute),
		Capacities: []ipam.CapacityEvidence{{Scope: "account:reserved", Family: 0, Product: "reserved", Limit: ipPtr(*p.Desired), Used: ipPtr(0), Source: "test fixture shared quota"}, {Scope: "server:123", Family: 0, ServerID: server, Limit: ipPtr(2), Used: ipPtr(0), Source: "test fixture shared attachment"}}})
	if e != nil {
		t.Fatal(e)
	}
	in := ipam.PlanInput{AccountID: account, PolicyRevision: saved.Revision, Policy: p, Quota: ipPtr(10), InventoryAt: obs.ObservedAt, CredentialGeneration: generation, EvidenceIDs: map[string]string{server: evidence}, WireRequests: map[string]ipam.WireRequest{server: wire}, FundingRevision: funding.Revision,
		Quote:      ipam.Quote{Currency: "USD", HourlyMicros: ipPtr(int64(1000)), DetachedHourlyMicros: ipPtr(int64(10000)), GranularitySeconds: 3600, Source: "test-only quote", ExpiresAt: now.Add(time.Hour)},
		Candidates: []ipam.Candidate{{AccountID: account, ServerID: server, Region: "nyc3", Healthy: true, Supported: true, CapabilityKnown: true, CapabilityAt: now, CredentialGeneration: generation, Family: family, Product: "reserved", Slots: ipPtr(2), NetworkImpact: "DETACHED_ALLOCATION"}}}
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

func wireReply() ipwire.Reply {
	return ipwire.Reply{Status: 202, Complete: true, Body: []byte(`{"reserved_ip":{"ip":"8.8.4.4","region":{"slug":"nyc3"},"droplet":null}}`)}
}
func TestIPWireConcurrentConsumeAndHandleCopies(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipWireApprovedBatch(t, db, ipa, 4)
	generation := ipGeneration(t, db, ipa)
	if _, e := s.Dispatch(ctx, ids[0], generation); !errors.Is(e, ipam.ErrEvidence) {
		t.Fatal("legacy path consumed wire plan", e)
	}
	wins := make(chan *ipam.WirePermit, 12)
	fail := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := s.DispatchWire(ctx, ids[0], generation)
			if e == nil {
				wins <- p
			} else if !errors.Is(e, ipam.ErrDispatch) {
				fail <- e
			}
		}()
	}
	wg.Wait()
	close(wins)
	close(fail)
	for e := range fail {
		t.Fatal(e)
	}
	if len(wins) != 1 {
		t.Fatal("duplicate consumption", len(wins))
	}
	permit := <-wins
	copied := *permit
	var calls atomic.Int32
	callback := func(ctx context.Context, o ipam.Operation, p ipwire.Prepared) ipwire.Reply {
		calls.Add(1)
		if o.AccountID != ipa || p.Spec().ServerID != "123" {
			t.Error("scope mismatch")
		}
		var durable int
		if e := db.QueryRow("SELECT count(*) FROM ip_wire_consumptions WHERE operation_id=$1", o.ID).Scan(&durable); e != nil || durable != 1 {
			t.Error("send before durable consume", e)
		}
		return wireReply()
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := copied.Run(ctx, callback)
			if e != nil && !errors.Is(e, ipam.ErrDispatch) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("replayed callback", calls.Load())
	}
	for _, table := range []string{"ip_wire_consumptions", "ip_wire_results"} {
		if _, e := db.Exec("UPDATE " + table + " SET operation_id=operation_id"); e == nil {
			t.Fatal("mutable ledger", table)
		}
	}
	if e := s.ReplayWireResult(ctx, ipb, ids[0]); e == nil {
		t.Fatal("cross-account replay")
	}
	if e := s.ReplayWireResult(ctx, ipa, ids[0]); e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow("SELECT count(*) FROM ip_mutation_receipts WHERE operation_id=$1", ids[0]).Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
}
func TestIPWireLostPermitAndIndependentAccount(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipWireApprovedBatch(t, db, ipa, 4)
	_, e := s.DispatchWire(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	// Simulate elapsed time after crash. Identity immutability forbids rewriting dispatched_at.
	if e = s.RecoverDispatched(ctx, ipa, time.Now().Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DispatchWire(ctx, ids[0], ipGeneration(t, db, ipa)); !errors.Is(e, ipam.ErrDispatch) {
		t.Fatal("lost permit reconstructed", e)
	}
	n, e := s.RecoverWireAccount(ctx, ipa, time.Now().Add(-time.Minute), 10)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	other, _, otherIDs := ipWireApprovedBatch(t, db, ipb, 4)
	p, e := other.DispatchWire(ctx, otherIDs[0], ipGeneration(t, db, ipb))
	if e != nil {
		t.Fatal("other account blocked", e)
	}
	if _, e = p.Run(ctx, func(context.Context, ipam.Operation, ipwire.Prepared) ipwire.Reply { return ipwire.Reply{} }); e != nil {
		t.Fatal(e)
	}
	var reserved int
	db.QueryRow("SELECT count(*) FROM ip_funding_reservations WHERE operation_id=$1 AND settled_at IS NULL", ids[0]).Scan(&reserved)
	if reserved != 1 {
		t.Fatal("obligation lost")
	}
}
func TestIPWireStaleAndCancelledPermitsBurnWithoutSend(t *testing.T) {
	for _, mode := range []string{"generation", "cancelled", "revoked", "recovered"} {
		t.Run(mode, func(t *testing.T) {
			db := adminTestDB(t)
			ipSeed(t, db)
			ctx := context.Background()
			s, approval, ids := ipWireApprovedBatch(t, db, ipa, 4)
			p, e := s.DispatchWire(ctx, ids[0], ipGeneration(t, db, ipa))
			if e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "generation":
				sqlMust(t, db, "UPDATE network_profiles SET updated_at=clock_timestamp() WHERE account_id=$1", ipa)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "revoked":
				sqlMust(t, db, "UPDATE ip_approvals SET revoked_at=clock_timestamp() WHERE id=$1", approval)
			case "recovered":
				if e = s.RecoverDispatched(ctx, ipa, time.Now().Add(time.Second)); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			fn := func(context.Context, ipam.Operation, ipwire.Prepared) ipwire.Reply { calls++; return wireReply() }
			if _, e = p.Run(ctx, fn); e == nil {
				t.Fatal("stale preflight accepted")
			}
			if calls != 0 {
				t.Fatal("sent stale request")
			}
			if _, e = p.Run(context.Background(), fn); !errors.Is(e, ipam.ErrDispatch) {
				t.Fatal("restored burned permit", e)
			}
			var state string
			db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ids[0]).Scan(&state)
			if state != "UNKNOWN" {
				t.Fatal(state)
			}
		})
	}
}
func TestIPWireDurableResultReplaysAfterReceiptFailure(t *testing.T) {
	db := adminTestDB(t)
	ipSeed(t, db)
	ctx := context.Background()
	s, _, ids := ipWireApprovedBatch(t, db, ipa, 4)
	p, e := s.DispatchWire(ctx, ids[0], ipGeneration(t, db, ipa))
	if e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, `CREATE FUNCTION fixture_wire_reject() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$; CREATE TRIGGER fixture_wire_reject BEFORE INSERT ON ip_mutation_receipts FOR EACH ROW EXECUTE FUNCTION fixture_wire_reject()`)
	calls := 0
	result, e := p.Run(ctx, func(context.Context, ipam.Operation, ipwire.Prepared) ipwire.Reply {
		calls++
		return ipwire.Reply{Status: 429, Complete: true, Header: http.Header{"Retry-After": []string{"7200"}}, Body: []byte(`{"error":"never persist this secret"}`)}
	})
	if e == nil || result.Outcome != "UNKNOWN" || calls != 1 {
		t.Fatal(result, e, calls)
	}
	sqlMust(t, db, "DROP TRIGGER fixture_wire_reject ON ip_mutation_receipts")
	restart := ipam.Store{DB: db}
	n, e := restart.RecoverWireAccount(ctx, ipa, time.Now().Add(-time.Minute), 10)
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	n, e = restart.RecoverWireAccount(ctx, ipa, time.Now().Add(-time.Minute), 10)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	var blocked bool
	db.QueryRow("SELECT rate_limit_until>clock_timestamp()+interval '119 minutes' FROM ip_mutation_lanes WHERE account_id=$1 AND family=4", ipa).Scan(&blocked)
	if !blocked {
		t.Fatal("cooldown lost")
	}
	if _, e = p.Run(ctx, nil); !errors.Is(e, ipam.ErrDispatch) {
		t.Fatal(e)
	}
	var leaked bool
	db.QueryRow("SELECT body::text LIKE '%never persist%' FROM ip_wire_results WHERE operation_id=$1", ids[0]).Scan(&leaked)
	if leaked {
		t.Fatal("raw provider body persisted")
	}
}
func TestIPWireLegacyPlanAndCommitFailureNoPermit(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		db := adminTestDB(t)
		ipSeed(t, db)
		s, _, ids := ipApprovedBatch(t, db, ipa, 4)
		if p, e := s.DispatchWire(context.Background(), ids[0], ipGeneration(t, db, ipa)); e == nil || p != nil {
			t.Fatal("legacy plan sendable", e)
		}
	})
	t.Run("rollback", func(t *testing.T) {
		db := adminTestDB(t)
		ipSeed(t, db)
		s, _, ids := ipWireApprovedBatch(t, db, ipa, 4)
		sqlMust(t, db, `CREATE FUNCTION fixture_consume_reject() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$; CREATE CONSTRAINT TRIGGER fixture_consume_reject AFTER INSERT ON ip_wire_consumptions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fixture_consume_reject()`)
		if p, e := s.DispatchWire(context.Background(), ids[0], ipGeneration(t, db, ipa)); e == nil || p != nil {
			t.Fatal("uncommitted permit", e)
		}
		var state string
		db.QueryRow("SELECT state FROM ip_mutation_intents WHERE id=$1", ids[0]).Scan(&state)
		if state != "PLANNED" {
			t.Fatal("partial transaction", state)
		}
	})
}
