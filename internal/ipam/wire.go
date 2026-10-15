package ipam

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ipwire"
)

// WireRequest binds the versioned encoding, exact method/URL/body and decoding
// contract to both the approved plan and trusted execution evidence.
type WireRequest struct {
	Spec   ipwire.Spec `json:"spec"`
	Digest string      `json:"digest"`
}

func (w WireRequest) prepare() (ipwire.Prepared, error) {
	p, e := ipwire.Prepare(w.Spec)
	if e != nil || w.Digest != p.Digest() {
		return ipwire.Prepared{}, ErrEvidence
	}
	return p, nil
}
func wireScope(s ipwire.Spec, provider string, family int, product, server, region string) bool {
	return s.Action == "ALLOCATE" && s.Provider == provider && s.Family == family && s.Product == product && s.ServerID == server && s.Region == region
}
func validateWireKeys(p Plan) error {
	if len(p.Input.WireRequests) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, a := range p.Assignments {
		seen[a.ServerID] = true
		if _, ok := p.Input.WireRequests[a.ServerID]; !ok {
			return ErrEvidence
		}
	}
	if len(seen) != len(p.Input.WireRequests) {
		return ErrEvidence
	}
	return nil
}
func boundWire(p Plan, a Assignment, v ExecutionEvidence, required bool) (ipwire.Prepared, error) {
	w, ok := p.Input.WireRequests[a.ServerID]
	if !ok && v.WireRequest == nil && !required {
		return ipwire.Prepared{}, nil
	}
	if !ok || v.WireRequest == nil || w != *v.WireRequest || validateWireKeys(p) != nil {
		return ipwire.Prepared{}, ErrEvidence
	}
	prepared, e := w.prepare()
	if e != nil || !wireScope(w.Spec, v.Provider, p.Input.Policy.Family, p.Input.Policy.Product, a.ServerID, a.Region) || prepared.Impact() != a.NetworkImpact {
		return ipwire.Prepared{}, ErrEvidence
	}
	bits := 32
	if w.Spec.Family == 6 {
		bits = w.Spec.PrefixLength
		if bits == 0 {
			bits = 128
		}
	}
	if (w.Spec.Family == 6 && p.Input.Policy.AllocationBits == nil) || (p.Input.Policy.AllocationBits != nil && *p.Input.Policy.AllocationBits != bits) {
		return ipwire.Prepared{}, ErrEvidence
	}
	return prepared, nil
}

// WirePermit is an offline integration boundary, NOT a production account runtime.
// Copies share the same private spent flag. No database API reconstructs a permit.
// A production adapter must still fence credentials and account routing at its
// actual RoundTrip boundary. No production route or worker calls Run in this slice.
type WirePermit struct{ state *wirePermitState }
type wirePermitState struct {
	store      Store
	operation  Operation
	request    ipwire.Prepared
	generation string
	spent      atomic.Bool
}

// DispatchWire atomically consumes one send entitlement with dispatch. Commit
// uncertainty returns no permit, even if the database actually committed.
func (s Store) DispatchWire(ctx context.Context, id, generation string) (*WirePermit, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	o, e := s.dispatchTx(ctx, tx, id, generation, true)
	if e != nil {
		return nil, e
	}
	p, a, v, e := loadWirePlan(ctx, tx, o)
	if e != nil {
		return nil, e
	}
	prepared, e := boundWire(p, a, v, true)
	if e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(p.Input.WireRequests[o.ServerID])
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_wire_consumptions(operation_id,dispatch_id,request_digest,wire_digest,generation,request) VALUES($1,$2,$3,$4,$5,$6::jsonb)`, o.ID, o.DispatchID, o.RequestDigest, prepared.Digest(), generation, string(raw))
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return &WirePermit{state: &wirePermitState{store: s, operation: o, request: prepared, generation: generation}}, nil
}
func loadWirePlan(ctx context.Context, q SQLFence, o Operation) (Plan, Assignment, ExecutionEvidence, error) {
	var p Plan
	var a Assignment
	var v ExecutionEvidence
	var raw []byte
	e := q.QueryRowContext(ctx, `SELECT p.body FROM ip_mutation_intents m JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest WHERE m.id=$1 AND m.account_id=$2`, o.ID, o.AccountID).Scan(&raw)
	if e != nil {
		return p, a, v, e
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		return p, a, v, e
	}
	for _, candidate := range p.Assignments {
		if candidate.ServerID == o.ServerID && candidate.Region == o.Region {
			a = candidate
			break
		}
	}
	if a.ServerID == "" {
		return p, a, v, ErrEvidence
	}
	v, e = executionEvidence(ctx, q, p, a)
	return p, a, v, e
}
func (s *wirePermitState) preflight(ctx context.Context) error {
	tx, e := s.store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	o := s.operation
	if e = LockAccount(ctx, tx, o.AccountID); e != nil {
		return e
	}
	if e = currentObservationGeneration(ctx, tx, o.AccountID, s.generation); e != nil {
		return e
	}
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT m.state='DISPATCHED' AND m.dispatch_id=w.dispatch_id AND w.dispatch_id=$2::uuid
 AND w.request_digest=$3 AND w.wire_digest=$4 AND w.generation=$5
 AND g.revoked_at IS NULL AND g.expires_at>clock_timestamp() AND a.enabled AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL
 AND pol.revision=p.policy_revision AND f.revision=g.funding_revision AND f.funded_until>clock_timestamp()
 AND NOT EXISTS(SELECT 1 FROM ip_wire_results WHERE operation_id=m.id)
 AND NOT EXISTS(SELECT 1 FROM ip_mutation_intents other WHERE other.account_id=m.account_id AND other.state IN('UNKNOWN','BLOCKED'))
 AND NOT EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=m.account_id AND server_id=m.server_id)
 AND NOT EXISTS(SELECT 1 FROM ip_mutation_lanes WHERE account_id=m.account_id AND rate_limit_until>clock_timestamp())
 FROM ip_wire_consumptions w JOIN ip_mutation_intents m ON m.id=w.operation_id JOIN ip_approvals g ON g.id=m.approval_id
 JOIN ip_plans p ON p.digest=g.plan_digest JOIN ip_policies pol ON pol.account_id=m.account_id AND pol.family=m.family
 JOIN accounts a ON a.id=m.account_id JOIN ip_account_funding f ON f.id=g.funding_scope
 WHERE m.id=$1`, o.ID, o.DispatchID, o.RequestDigest, s.request.Digest(), s.generation).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrDispatch
	}
	p, a, v, e := loadWirePlan(ctx, tx, o)
	if e != nil {
		return e
	}
	w, e := boundWire(p, a, v, true)
	if e != nil || w.Digest() != s.request.Digest() || time.Since(p.CreatedAt) > 5*time.Minute || !p.Input.Quote.ExpiresAt.After(time.Now()) {
		return ErrEvidence
	}
	for _, c := range v.Capacities {
		n, e := capacityHeadroom(ctx, tx, o.AccountID, c, s.generation)
		if e != nil {
			return e
		}
		if n < 0 {
			return ErrEvidence
		}
	}
	return tx.Commit()
}

// Run burns this handle before preflight, then invokes a trusted bounded adapter
// at most once. The adapter must honor cancellation and provide its own account
// credential/route send fence. Preflight alone is NOT an atomic network fence.
// Network errors, crashes and persistence failures never restore the entitlement.
func (p *WirePermit) Run(ctx context.Context, exchange func(context.Context, Operation, ipwire.Prepared) ipwire.Reply) (ipwire.Result, error) {
	if p == nil || p.state == nil || !p.state.spent.CompareAndSwap(false, true) {
		return ipwire.Result{}, ErrDispatch
	}
	s := p.state
	attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e := s.preflight(attempt)
	reply := ipwire.Reply{}
	if e == nil && exchange != nil {
		reply = exchange(attempt, s.operation, s.request)
	} else if e == nil {
		e = ErrEvidence
	}
	result := s.request.DecodeReply(reply, time.Now().UTC())
	// A cancelled caller cannot suppress durable response/429 evidence.
	persist, cancelPersist := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPersist()
	if err := s.store.saveWireResult(persist, s.operation, s.request.Digest(), result); err != nil {
		return result, err
	}
	if err := s.store.ReplayWireResult(persist, s.operation.AccountID, s.operation.ID); err != nil {
		return result, err
	}
	return result, e
}

type wireResultRecord struct {
	DispatchID    string          `json:"dispatch_id"`
	RequestDigest string          `json:"request_digest"`
	WireDigest    string          `json:"wire_digest"`
	Result        ipwire.Result   `json:"result"`
	Receipt       DispatchReceipt `json:"receipt"`
}

func wireReceipt(o Operation, r ipwire.Result, now time.Time) DispatchReceipt {
	x := DispatchReceipt{DispatchID: o.DispatchID, RequestDigest: o.RequestDigest, Outcome: r.Outcome, ResourceID: r.ResourceID, Contract: r.Contract, HTTPStatus: r.HTTPStatus}
	switch r.ErrorClass {
	case "RATE_LIMIT":
		x.ErrorClass = "rate_limited"
	case "NETWORK":
		x.ErrorClass = "network"
	case "AUTH":
		x.ErrorClass = "authentication"
	case "PAYMENT":
		x.ErrorClass = "billing"
	case "PROVIDER_UNAVAILABLE":
		x.ErrorClass = "unavailable"
	case "":
	default:
		x.ErrorClass = "unknown"
	}
	if r.RetryAt != nil && r.RetryAt.After(now) {
		x.RetryAfterSeconds = min(int64(31536000), int64(r.RetryAt.Sub(now).Seconds())+1)
	}
	if r.HTTPStatus == 429 {
		x.RetryAfterSeconds = max(x.RetryAfterSeconds, 60)
	}
	return x
}
func (s Store) saveWireResult(ctx context.Context, o Operation, digest string, r ipwire.Result) error {
	record := wireResultRecord{o.DispatchID, o.RequestDigest, digest, r, wireReceipt(o, r, time.Now().UTC())}
	raw, _ := json.Marshal(record)
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, o.AccountID); e != nil {
		return e
	}
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_wire_consumptions w JOIN ip_mutation_intents m ON m.id=w.operation_id WHERE m.id=$1 AND m.account_id=$2 AND w.dispatch_id=$3::uuid AND w.request_digest=$4 AND w.wire_digest=$5)`, o.ID, o.AccountID, o.DispatchID, o.RequestDigest, digest).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrDispatch
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_wire_results(operation_id,body) VALUES($1,$2::jsonb) ON CONFLICT(operation_id) DO NOTHING`, o.ID, string(raw))
	if e != nil {
		return e
	}
	e = tx.QueryRowContext(ctx, `SELECT body=$2::jsonb FROM ip_wire_results WHERE operation_id=$1`, o.ID, string(raw)).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return ErrReceiptConflict
	}
	return tx.Commit()
}

// ReplayWireResult only persists an already durable response. It cannot send HTTP
// or reconstruct a permit, and is explicitly scoped to the owning account.
func (s Store) ReplayWireResult(ctx context.Context, account, id string) error {
	var raw []byte
	e := s.DB.QueryRowContext(ctx, `SELECT r.body FROM ip_wire_results r JOIN ip_wire_consumptions w ON w.operation_id=r.operation_id JOIN ip_mutation_intents m ON m.id=r.operation_id
 WHERE m.account_id=$1 AND m.id=$2 AND r.body->>'dispatch_id'=w.dispatch_id::text AND r.body->>'request_digest'=w.request_digest AND r.body->>'wire_digest'=w.wire_digest`, account, id).Scan(&raw)
	if e != nil {
		return e
	}
	var r wireResultRecord
	if e = json.Unmarshal(raw, &r); e != nil {
		return e
	}
	return s.RecordResponse(ctx, id, "wire:"+r.DispatchID, r.Receipt)
}

// RecoverWireAccount is bounded and independent per account. Expired dispatched
// operations become UNKNOWN first; saved results may then be replayed. Missing
// results never grant absence, ownership, cancellation or a retry entitlement.
func (s Store) RecoverWireAccount(ctx context.Context, account string, olderThan time.Time, limit int) (int, error) {
	if account == "" || limit < 1 || limit > 100 || olderThan.After(time.Now().Add(-time.Minute)) {
		return 0, ErrEvidence
	}
	if e := s.RecoverDispatched(ctx, account, olderThan); e != nil {
		return 0, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT r.operation_id::text FROM ip_wire_results r JOIN ip_wire_consumptions w ON w.operation_id=r.operation_id JOIN ip_mutation_intents m ON m.id=r.operation_id
 WHERE m.account_id=$1 AND NOT EXISTS(SELECT 1 FROM ip_mutation_receipts WHERE operation_id=m.id AND receipt_key='wire:'||w.dispatch_id::text) ORDER BY r.recorded_at,r.operation_id LIMIT $2`, account, limit)
	if e != nil {
		return 0, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	n := 0
	for _, id := range ids {
		if e = s.ReplayWireResult(ctx, account, id); e != nil {
			return n, e
		}
		n++
	}
	return n, nil
}
