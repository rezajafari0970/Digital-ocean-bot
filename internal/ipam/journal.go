package ipam

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrApproval = errors.New("IP plan approval missing, expired or mismatched")
var ErrDispatch = errors.New("IP operation already dispatched; reconcile without replay")
var ErrEvidence = errors.New("IP capability, quota or generation evidence stale")

type ApprovalRequest struct {
	PlanDigest          string
	ApprovedBy          string
	ExpiresAt           time.Time
	FundedUntil         time.Time
	MaxCommitmentMicros int64
}
type Operation struct {
	RequestDigest string     `json:"request_digest"`
	ID            string     `json:"id"`
	AccountID     string     `json:"account_id"`
	Provider      string     `json:"provider"`
	Family        int        `json:"family"`
	Product       string     `json:"product"`
	ServerID      string     `json:"server_id"`
	Region        string     `json:"region"`
	Action        string     `json:"action"`
	ResourceID    string     `json:"resource_id"`
	DispatchID    string     `json:"dispatch_id"`
	State         string     `json:"state"`
	ApprovalID    string     `json:"approval_id"`
	DispatchedAt  *time.Time `json:"dispatched_at"`
}
type DispatchReceipt struct {
	AcknowledgementKey string `json:"acknowledgement_key,omitempty"`
	Contract           string `json:"contract,omitempty"`
	RequestDigest      string `json:"request_digest"`
	DispatchID         string `json:"dispatch_id"`
	Outcome            string `json:"outcome"` // ACCEPTED is not yet verified ownership or reachability.
	ResourceID         string `json:"resource_id"`
	ActionID           string `json:"action_id"`
	ErrorClass         string `json:"error_class"`
	HTTPStatus         int    `json:"http_status"`
	RetryAfterSeconds  int64  `json:"retry_after_seconds"`
}

func digestBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func (s Store) SavePlan(ctx context.Context, p Plan) error {
	if e := validateWireKeys(p); e != nil {
		return e
	}
	canonical := Preview(p.Input, p.CreatedAt)
	raw, _ := json.Marshal(p)
	want, _ := json.Marshal(canonical)
	if string(raw) != string(want) || p.State != "AWAITING_APPROVAL" || p.CreatedAt.After(time.Now()) || time.Since(p.CreatedAt) > 2*time.Minute {
		return ErrEvidence
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, p.Input.AccountID); e != nil {
		return e
	}
	var same bool
	policy, _ := json.Marshal(p.Input.Policy)
	e = tx.QueryRowContext(ctx, "SELECT revision=$3 AND policy=$4::jsonb FROM ip_policies WHERE account_id=$1 AND family=$2", p.Input.AccountID, p.Input.Policy.Family, p.Input.PolicyRevision, string(policy)).Scan(&same)
	if e != nil {
		return e
	}
	if !same {
		return ErrRevision
	}
	var fundingOK bool
	e = tx.QueryRowContext(ctx, "SELECT revision=$2 AND currency=$3 AND funded_until>clock_timestamp() FROM ip_account_funding WHERE account_id=$1", p.Input.AccountID, p.Input.FundingRevision, p.Currency).Scan(&fundingOK)
	if e != nil || !fundingOK {
		return ErrEvidence
	}
	for _, a := range p.Assignments {
		if _, e = executionEvidence(ctx, tx, p, a); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_plans(digest,account_id,family,policy_revision,body,created_at) VALUES($1,$2,$3,$4,$5::jsonb,$6) ON CONFLICT(digest) DO NOTHING`, p.Digest, p.Input.AccountID, p.Input.Policy.Family, p.Input.PolicyRevision, string(raw), p.CreatedAt)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// Approve records the explicit external decision. No HTTP/API route calls it in this release.
func (s Store) Approve(ctx context.Context, r ApprovalRequest) (string, error) {
	if r.ApprovedBy == "" || len(r.ApprovedBy) > 200 || r.MaxCommitmentMicros < 0 {
		return "", ErrApproval
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var raw []byte
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text,body FROM ip_plans WHERE digest=$1", r.PlanDigest).Scan(&account, &raw); e != nil {
		return "", e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return "", e
	}
	var p Plan
	if e = json.Unmarshal(raw, &p); e != nil {
		return "", e
	}
	var now time.Time
	var revision int64
	var enabled bool
	e = tx.QueryRowContext(ctx, `SELECT clock_timestamp(),p.revision,a.enabled AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL
 FROM ip_policies p JOIN accounts a ON a.id=p.account_id WHERE p.account_id=$1 AND p.family=$2`, account, p.Input.Policy.Family).Scan(&now, &revision, &enabled)
	if e != nil {
		return "", e
	}
	if !enabled || revision != p.Input.PolicyRevision || p.State != "AWAITING_APPROVAL" || len(p.Assignments) == 0 || p.EstimatedCommitmentMicros == nil || p.Input.Policy.BudgetMicros == nil || p.Input.Policy.FundedHorizonSeconds == nil {
		return "", ErrApproval
	}
	// Approvals cannot outlive the bound quote/evidence. Cleanup gets separate authority.
	if !r.ExpiresAt.After(now) || r.ExpiresAt.After(p.Input.Quote.ExpiresAt) || r.ExpiresAt.After(p.CreatedAt.Add(5*time.Minute)) || !r.FundedUntil.After(r.ExpiresAt) || r.FundedUntil.After(p.CreatedAt.Add(time.Duration(*p.Input.Policy.FundedHorizonSeconds)*time.Second)) || r.MaxCommitmentMicros < *p.EstimatedCommitmentMicros || r.MaxCommitmentMicros > *p.Input.Policy.BudgetMicros {
		return "", ErrApproval
	}
	var fundingID string
	var fundingRevision int64
	var fundingCurrency string
	var fundingCap int64
	var fundingUntil time.Time
	e = tx.QueryRowContext(ctx, "SELECT id::text,revision,currency,budget_micros,funded_until FROM ip_account_funding WHERE account_id=$1", account).Scan(&fundingID, &fundingRevision, &fundingCurrency, &fundingCap, &fundingUntil)
	if e != nil {
		return "", ErrEvidence
	}
	if fundingRevision != p.Input.FundingRevision || fundingCurrency != p.Currency || r.MaxCommitmentMicros > fundingCap || r.FundedUntil.After(fundingUntil) {
		return "", ErrApproval
	}
	var id string
	e = tx.QueryRowContext(ctx, `INSERT INTO ip_approvals(plan_digest,account_id,family,approved_by,expires_at,currency,max_commitment_micros,funded_until,max_allocations,funding_scope,funding_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, r.PlanDigest, account, p.Input.Policy.Family, r.ApprovedBy, r.ExpiresAt, p.Currency, r.MaxCommitmentMicros, r.FundedUntil, len(p.Assignments), fundingID, fundingRevision).Scan(&id)
	if e != nil {
		return "", e
	}
	return id, tx.Commit()
}

// QueueApproval atomically reserves the entire exact approved batch. No external IO occurs.
func (s Store) QueueApproval(ctx context.Context, approval string) ([]string, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_approvals WHERE id=$1", approval).Scan(&account); e != nil {
		return nil, e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return nil, e
	}
	// An identical queue request returns the durable batch, even if acquisition authority expired later.
	rows, e := tx.QueryContext(ctx, "SELECT id::text FROM ip_mutation_intents WHERE approval_id=$1 ORDER BY idempotency_key", approval)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if len(ids) > 0 {
		return ids, tx.Commit()
	}
	var raw []byte
	var provider string
	var allowed bool
	var cap int64
	e = tx.QueryRowContext(ctx, `SELECT p.body,a.provider,
 g.revoked_at IS NULL AND g.expires_at>clock_timestamp() AND a.enabled AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL AND pol.revision=p.policy_revision,
 g.max_commitment_micros FROM ip_approvals g JOIN ip_plans p ON p.digest=g.plan_digest JOIN accounts a ON a.id=g.account_id JOIN ip_policies pol ON pol.account_id=a.id AND pol.family=g.family WHERE g.id=$1`, approval).Scan(&raw, &provider, &allowed, &cap)
	if e != nil {
		return nil, e
	}
	if !allowed {
		return nil, ErrApproval
	}
	var p Plan
	if e = json.Unmarshal(raw, &p); e != nil {
		return nil, e
	}

	var blocked bool
	var fundingID, currency string
	var fundingRevision int64
	var fundingCap, committed int64
	var fundingUntil time.Time
	var accountConcurrency int
	e = tx.QueryRowContext(ctx, "SELECT id::text,revision,currency,budget_micros,funded_until,max_concurrent FROM ip_account_funding WHERE account_id=$1", account).Scan(&fundingID, &fundingRevision, &currency, &fundingCap, &fundingUntil, &accountConcurrency)
	if e != nil {
		return nil, ErrEvidence
	}
	var approvalScope string
	var approvalRevision int64
	var approvedUntil time.Time
	e = tx.QueryRowContext(ctx, "SELECT funding_scope::text,funding_revision,funded_until FROM ip_approvals WHERE id=$1", approval).Scan(&approvalScope, &approvalRevision, &approvedUntil)
	if e != nil {
		return nil, e
	}
	if approvalScope != fundingID || approvalRevision != fundingRevision || currency != p.Currency || !fundingUntil.After(time.Now()) || approvedUntil.After(fundingUntil) {
		return nil, ErrApproval
	}
	e = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(fr.commitment_micros),0) FROM ip_funding_reservations fr JOIN ip_mutation_intents m ON m.id=fr.operation_id
 WHERE fr.funding_scope=$1 AND fr.settled_at IS NULL AND m.state NOT IN('REJECTED','CANCELLED')`, fundingID).Scan(&committed)
	if e != nil {
		return nil, e
	}
	if p.Input.ExistingCommitmentMicros != committed {
		return nil, ErrEvidence
	}
	incremental := *p.EstimatedCommitmentMicros - p.Input.ExistingCommitmentMicros
	unit := (incremental + int64(len(p.Assignments)) - 1) / int64(len(p.Assignments))
	incremental = unit * int64(len(p.Assignments))
	if incremental < 0 || committed > min(cap, fundingCap) || incremental > min(cap, fundingCap)-committed {
		return nil, ErrApproval
	}
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_mutation_intents WHERE account_id=$1 AND state IN('UNKNOWN','BLOCKED'))
 OR EXISTS(SELECT 1 FROM ip_mutation_lanes WHERE account_id=$1 AND (next_attempt_at>clock_timestamp() OR rate_limit_until>clock_timestamp()))`, account).Scan(&blocked)
	if e != nil {
		return nil, e
	}
	if blocked {
		return nil, ErrLaneBusy
	}
	var managed, queued int
	e = tx.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM ip_allocations WHERE account_id=$1 AND family=$2 AND state<>'RELEASED')+
 (SELECT count(*) FROM ip_mutation_intents WHERE account_id=$1 AND family=$2 AND action='ALLOCATE' AND state NOT IN('REJECTED','CANCELLED','CONFIRMED')),
 (SELECT count(*) FROM ip_mutation_intents WHERE account_id=$1 AND state IN('PLANNED','DISPATCHED','ACCEPTED','UNKNOWN','BLOCKED'))`, account, p.Input.Policy.Family).Scan(&managed, &queued)
	if e != nil {
		return nil, e
	}
	if managed+len(p.Assignments) > *p.Input.Policy.Desired || queued+len(p.Assignments) > accountConcurrency {
		return nil, ErrProtected
	}
	scopeNeed := map[string]int{}
	scopes := map[string]CapacityEvidence{}
	bundles := map[string]ExecutionEvidence{}
	for _, a := range p.Assignments {
		v, err := executionEvidence(ctx, tx, p, a)
		if err != nil {
			return nil, err
		}
		bundles[a.ServerID] = v
		for _, c := range v.Capacities {
			scopeNeed[c.Scope]++
			scopes[c.Scope] = c
		}
	}
	for key, need := range scopeNeed {
		available, err := capacityHeadroom(ctx, tx, account, scopes[key], p.Input.CredentialGeneration)
		if err != nil {
			return nil, err
		}
		if available < need {
			return nil, ErrEvidence
		}
	}
	for i, a := range p.Assignments {
		if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=$1 AND server_id=$2)", account, a.ServerID).Scan(&blocked); e != nil {
			return nil, e
		}
		if blocked {
			return nil, ErrProtected
		}
		request, _ := json.Marshal(struct {
			PlanDigest string     `json:"plan_digest"`
			Assignment Assignment `json:"assignment"`
			Index      int        `json:"index"`
		}{p.Digest, a, i})
		var id string
		e = tx.QueryRowContext(ctx, `INSERT INTO ip_mutation_intents(account_id,provider,family,product,server_id,region,idempotency_key,action,approval_id,request,commitment_micros)
 VALUES($1,$2,$3,$4,$5,$6,$7,'ALLOCATE',$8,$9::jsonb,$10) RETURNING id::text`, account, provider, p.Input.Policy.Family, p.Input.Policy.Product, a.ServerID, a.Region, fmt.Sprintf("%s:%04d", approval, i), approval, string(request), unit).Scan(&id)
		if e != nil {
			return nil, e
		}
		ids = append(ids, id)
		for _, c := range bundles[a.ServerID].Capacities {
			if _, e = tx.ExecContext(ctx, "INSERT INTO ip_operation_reservations(operation_id,scope_key) VALUES($1,$2)", id, c.Scope); e != nil {
				return nil, e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO ip_funding_reservations(operation_id,funding_scope,currency,commitment_micros,funded_until) VALUES($1,$2,$3,$4,$5)", id, fundingID, currency, unit, approvedUntil); e != nil {
			return nil, e
		}
	}
	spacing, e := ChooseSeconds(*p.Input.Policy.SpacingMinSeconds, *p.Input.Policy.SpacingMaxSeconds)
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_mutation_lanes(account_id,family,next_attempt_at) VALUES($1,$2,clock_timestamp()+$3*interval '1 second') ON CONFLICT(account_id,family) DO UPDATE SET next_attempt_at=excluded.next_attempt_at`, account, p.Input.Policy.Family, spacing)
	if e != nil {
		return nil, e
	}
	return ids, tx.Commit()
}

// Dispatch is a one-shot durable authorization. Losing its return value is UNKNOWN,
// not permission to call it again. Callers must never reconstruct/replay this ticket.
func (s Store) Dispatch(ctx context.Context, id, currentGeneration string) (Operation, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Operation{}, e
	}
	defer tx.Rollback()
	o, e := s.dispatchTx(ctx, tx, id, currentGeneration, false)
	if e != nil {
		return o, e
	}
	if e = tx.Commit(); e != nil {
		return Operation{}, e
	}
	return o, nil
}
func (s Store) dispatchTx(ctx context.Context, tx *sql.Tx, id, currentGeneration string, wire bool) (Operation, error) {
	var o Operation
	var e error
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_mutation_intents WHERE id=$1", id).Scan(&o.AccountID); e != nil {
		return o, e
	}
	if e = LockAccount(ctx, tx, o.AccountID); e != nil {
		return o, e
	}
	var raw []byte
	var allowed bool
	var dispatched sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT m.id::text,m.provider,m.family,m.product,m.server_id,m.region,m.action,m.resource_id,m.state,m.approval_id::text,m.dispatch_id::text,p.body,
 g.revoked_at IS NULL AND g.expires_at>clock_timestamp() AND a.enabled AND a.deleted_at IS NULL AND a.deletion_requested_at IS NULL AND pol.revision=p.policy_revision
 FROM ip_mutation_intents m JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest JOIN accounts a ON a.id=m.account_id JOIN ip_policies pol ON pol.account_id=a.id AND pol.family=m.family WHERE m.id=$1 FOR UPDATE OF m`, id).Scan(&o.ID, &o.Provider, &o.Family, &o.Product, &o.ServerID, &o.Region, &o.Action, &o.ResourceID, &o.State, &o.ApprovalID, &dispatched, &raw, &allowed)
	if e != nil {
		return o, e
	}
	if dispatched.Valid || o.State != "PLANNED" {
		return o, ErrDispatch
	}
	if !allowed {
		return o, ErrApproval
	}
	var p Plan
	if e = json.Unmarshal(raw, &p); e != nil {
		return o, e
	}
	if currentGeneration == "" || currentGeneration != p.Input.CredentialGeneration || time.Since(p.CreatedAt) > 5*time.Minute || !p.Input.Quote.ExpiresAt.After(time.Now()) {
		return o, ErrEvidence
	}

	if !wire && len(p.Input.WireRequests) != 0 {
		return o, ErrEvidence
	}
	var assignment Assignment
	found := false
	for _, a := range p.Assignments {
		if a.ServerID == o.ServerID && a.Region == o.Region {
			assignment = a
			found = true
			break
		}
	}
	if !found {
		return o, ErrEvidence
	}
	if e = lockGeneration(ctx, tx, o.AccountID); e != nil {
		return o, e
	}
	evidence, e := executionEvidence(ctx, tx, p, assignment)
	if e != nil {
		return o, e
	}
	if wire {
		if _, e = boundWire(p, assignment, evidence, true); e != nil {
			return o, e
		}
	}
	for _, c := range evidence.Capacities {
		headroom, err := capacityHeadroom(ctx, tx, o.AccountID, c, currentGeneration)
		if err != nil {
			return o, err
		}
		if headroom < 0 {
			return o, ErrEvidence
		} // Includes this operation's already reserved unit.
	}
	var blocked bool
	var inFlight, accountConcurrency int
	e = tx.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=$1 AND server_id=$2)
 OR EXISTS(SELECT 1 FROM ip_mutation_intents WHERE account_id=$1 AND state IN('UNKNOWN','BLOCKED'))
 OR EXISTS(SELECT 1 FROM ip_wire_consumptions w JOIN ip_mutation_intents pending ON pending.id=w.operation_id WHERE pending.account_id=$1 AND NOT EXISTS(SELECT 1 FROM ip_mutation_receipts receipt WHERE receipt.operation_id=pending.id AND receipt.receipt_key='wire:'||w.dispatch_id::text))
 OR EXISTS(SELECT 1 FROM ip_mutation_lanes WHERE account_id=$1 AND (rate_limit_until>clock_timestamp() OR next_dispatch_at>clock_timestamp())),
 (SELECT count(*) FROM ip_mutation_intents WHERE account_id=$1 AND state IN('DISPATCHED','ACCEPTED')),
 f.max_concurrent
 FROM ip_account_funding f JOIN ip_approvals g ON g.funding_scope=f.id WHERE g.id=$3 AND f.account_id=$1 AND f.revision=g.funding_revision AND f.funded_until>clock_timestamp()`, o.AccountID, o.ServerID, o.ApprovalID).Scan(&blocked, &inFlight, &accountConcurrency)
	if e != nil {
		return o, ErrEvidence
	}
	if blocked || inFlight >= min(accountConcurrency, *p.Input.Policy.MaxConcurrent) {
		return o, ErrProtected
	}
	var request []byte
	if e = tx.QueryRowContext(ctx, "SELECT request FROM ip_mutation_intents WHERE id=$1", id).Scan(&request); e != nil {
		return o, e
	}
	o.RequestDigest = digestBytes(request)
	result, e := tx.ExecContext(ctx, `INSERT INTO ip_cleanup_credentials(operation_id,account_id,original_id,kind,ciphertext,nonce,key_version)
 SELECT $1,a.id,s.id,s.kind,s.ciphertext,s.nonce,s.key_version FROM accounts a JOIN secrets s ON s.account_id=a.id AND s.id=a.secret_ref WHERE a.id=$2`, id, o.AccountID)
	if e != nil {
		return o, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return o, e
	}
	if n != 1 {
		return o, ErrEvidence
	}
	e = tx.QueryRowContext(ctx, `UPDATE ip_mutation_intents SET state='DISPATCHED',dispatch_id=gen_random_uuid(),dispatched_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1 RETURNING dispatch_id::text,dispatched_at`, id).Scan(&o.DispatchID, &o.DispatchedAt)
	if e != nil {
		return o, e
	}
	spacing, e := ChooseSeconds(*p.Input.Policy.SpacingMinSeconds, *p.Input.Policy.SpacingMaxSeconds)
	if e != nil {
		return o, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO ip_mutation_lanes(account_id,family,next_dispatch_at) VALUES($1,$2,clock_timestamp()+$3*interval '1 second') ON CONFLICT(account_id,family) DO UPDATE SET next_dispatch_at=excluded.next_dispatch_at`, o.AccountID, o.Family, spacing); e != nil {
		return o, e
	}
	o.State = "DISPATCHED"
	return o, nil
}
func (s Store) RecoverDispatched(ctx context.Context, account string, olderThan time.Time) error {
	// No reset to PLANNED, no provider retry, and no deletion of a reservation.
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE ip_mutation_intents SET state='UNKNOWN',error_class='ambiguous_outcome',updated_at=clock_timestamp()
 WHERE account_id=$1 AND state='DISPATCHED' AND dispatched_at<$2`, account, olderThan)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func safeIPError(c string) string {
	switch c {
	case "", "rate_limited", "capacity", "region_capacity", "account_locked", "network", "authentication", "permission_denied", "unsupported", "transport", "unavailable", "invalid_request", "billing", "not_found", "unknown", "ambiguous_outcome":
		return c
	}
	return "unknown"
}
func (s Store) RecordResponse(ctx context.Context, id, key string, r DispatchReceipt) error {
	if key == "" || len(key) > 128 || r.DispatchID == "" || len(r.ResourceID) > 256 || len(r.ActionID) > 256 || r.HTTPStatus < 0 || r.HTTPStatus > 599 || r.RetryAfterSeconds < 0 || r.RetryAfterSeconds > 31536000 {
		return ErrEvidence
	}

	r.ErrorClass = safeIPError(r.ErrorClass)
	if r.HTTPStatus == 429 || r.ErrorClass == "rate_limited" {
		r.RetryAfterSeconds = max(r.RetryAfterSeconds, 60)
	}
	raw, _ := json.Marshal(r)
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account, dispatch, state string
	var family int
	var provider, product, action string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_mutation_intents WHERE id=$1", id).Scan(&account); e != nil {
		return e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}
	e = tx.QueryRowContext(ctx, "SELECT COALESCE(dispatch_id::text,''),state,family,provider,product,action FROM ip_mutation_intents WHERE id=$1 FOR UPDATE", id).Scan(&dispatch, &state, &family, &provider, &product, &action)
	if e != nil {
		return e
	}
	if dispatch != r.DispatchID {
		return ErrDispatch
	}
	var request []byte
	if e = tx.QueryRowContext(ctx, "SELECT request FROM ip_mutation_intents WHERE id=$1", id).Scan(&request); e != nil {
		return e
	}
	if r.RequestDigest != digestBytes(request) {
		return ErrDispatch
	}
	// Existing receipt identity is checked before new-verdict validation so retries
	// cannot rewrite an immutable outcome, even if their new payload is invalid.
	var same bool
	existing := tx.QueryRowContext(ctx, "SELECT kind='RESPONSE' AND body=$3::jsonb FROM ip_mutation_receipts WHERE operation_id=$1 AND receipt_key=$2", id, key, string(raw)).Scan(&same)
	if existing == nil {
		if !same {
			return ErrReceiptConflict
		}
		return tx.Commit()
	}
	if !errors.Is(existing, sql.ErrNoRows) {
		return existing
	}
	if e = validateOutcome(provider, product, action, family, r); e != nil {
		return e
	}
	if r.Outcome == "ACQUIRED" && provider == "digitalocean" && family == 4 {
		var acknowledged bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_mutation_receipts WHERE operation_id=$1 AND receipt_key=$2 AND kind='RESPONSE'
 AND body->>'outcome'='ACKNOWLEDGED' AND (body->>'http_status')::integer=202 AND body->>'resource_id'=$3 AND body->>'request_digest'=$4 AND body->>'dispatch_id'=$5)`, id, r.AcknowledgementKey, r.ResourceID, r.RequestDigest, r.DispatchID).Scan(&acknowledged)
		if e != nil {
			return e
		}
		if !acknowledged {
			return ErrEvidence
		}
	}
	res, e := tx.ExecContext(ctx, `INSERT INTO ip_mutation_receipts(operation_id,receipt_key,kind,body) VALUES($1,$2,'RESPONSE',$3::jsonb) ON CONFLICT(operation_id,receipt_key) DO NOTHING`, id, key, string(raw))
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		var same bool
		e = tx.QueryRowContext(ctx, "SELECT kind='RESPONSE' AND body=$3::jsonb FROM ip_mutation_receipts WHERE operation_id=$1 AND receipt_key=$2", id, key, string(raw)).Scan(&same)
		if e != nil {
			return e
		}
		if !same {
			return ErrReceiptConflict
		}
		return tx.Commit()
	}
	if r.RetryAfterSeconds > 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO ip_mutation_lanes(account_id,family,rate_limit_until)
 VALUES($1,$2,clock_timestamp()+$3*interval '1 second') ON CONFLICT(account_id,family) DO UPDATE SET rate_limit_until=GREATEST(ip_mutation_lanes.rate_limit_until,excluded.rate_limit_until)`, account, family, r.RetryAfterSeconds)
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE ip_inventory_lanes SET rate_limit_until=GREATEST(rate_limit_until,clock_timestamp()+$2*interval '1 second') WHERE account_id=$1`, account, r.RetryAfterSeconds)
		if e != nil {
			return e
		}
	}
	var accepted, rejected bool
	var identities int
	e = tx.QueryRowContext(ctx, `SELECT COALESCE(bool_or(body->>'outcome' IN('ACCEPTED','ACKNOWLEDGED','ACQUIRED')),false),COALESCE(bool_or(body->>'outcome'='REJECTED'),false),
 count(DISTINCT NULLIF(body->>'resource_id','')) FROM ip_mutation_receipts WHERE operation_id=$1 AND kind='RESPONSE'`, id).Scan(&accepted, &rejected, &identities)
	if e != nil {
		return e
	}
	next := "UNKNOWN"
	if accepted {
		next = "ACCEPTED"
	}
	if rejected {
		next = "REJECTED"
	}
	if accepted && rejected || identities > 1 {
		next = "BLOCKED"
	}
	if state == "CONFIRMED" && next != "BLOCKED" {
		next = state
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_mutation_intents SET state=$2,error_class=$3,updated_at=clock_timestamp() WHERE id=$1", id, next, r.ErrorClass)
	if e != nil {
		return e
	}
	return tx.Commit()
}
