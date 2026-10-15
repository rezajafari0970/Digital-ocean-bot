package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/netip"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

var ErrDiversity = errors.New("IP address or diversity prefix already used; allocation retained")

type Endpoint struct {
	ID              string    `json:"id"`
	AllocationID    string    `json:"allocation_id"`
	AccountID       string    `json:"account_id"`
	ServerID        string    `json:"server_id"`
	Address         string    `json:"address"`
	ParentPrefix    string    `json:"parent_prefix"`
	DiversityPrefix string    `json:"diversity_prefix"`
	AcquiredAt      time.Time `json:"acquired_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Phase           string    `json:"phase"`
	Revision        int64     `json:"revision"`
}

// ConfirmAcquisition needs BOTH a correlated successful response and a subsequent
// complete durable provider inventory. A list-difference, existing/manual IP, or
// absence after an unknown request never confers ownership or retry authority.
func (s Store) ConfirmAcquisition(ctx context.Context, id, readRequest string) (string, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_mutation_intents WHERE id=$1", id).Scan(&account); e != nil {
		return "", e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return "", e
	}
	var state, provider, product, server, region string
	var family int
	var dispatched time.Time
	e = tx.QueryRowContext(ctx, `SELECT state,provider,product,server_id,region,family,dispatched_at FROM ip_mutation_intents WHERE id=$1 AND action='ALLOCATE' FOR UPDATE`, id).Scan(&state, &provider, &product, &server, &region, &family, &dispatched)
	if e != nil {
		return "", e
	}
	if state == "CONFIRMED" {
		var a string
		e = tx.QueryRowContext(ctx, "SELECT id::text FROM ip_allocations WHERE acquisition_operation=$1", id).Scan(&a)
		return a, e
	}
	if state != "ACCEPTED" {
		return "", ErrProtected
	}
	var identity string
	var responseAt time.Time
	e = tx.QueryRowContext(ctx, `SELECT body->>'resource_id',at FROM ip_mutation_receipts WHERE operation_id=$1 AND kind='RESPONSE' AND body->>'outcome'='ACQUIRED' AND body->>'resource_id'<>'' ORDER BY id LIMIT 1`, id).Scan(&identity, &responseAt)
	if e != nil {
		return "", ErrEvidence
	}
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT observation FROM ip_inventory_events WHERE account_id=$1 AND request_id=$2 AND state='OBSERVED' AND started_at>=$3 AND finished_at>=$3
 AND EXISTS(SELECT 1 FROM ip_inventory_lanes l WHERE l.account_id=ip_inventory_events.account_id AND l.state='OBSERVED' AND l.request_id=ip_inventory_events.request_id AND l.last_good=ip_inventory_events.observation)`, account, readRequest, responseAt).Scan(&raw)
	if e != nil {
		return "", ErrEvidence
	}
	var o providers.IPObservation
	if e = json.Unmarshal(raw, &o); e != nil {
		return "", e
	}
	if e = providers.ValidateIPObservation(o); e != nil {
		return "", e
	}
	var admittedGeneration string
	if e = tx.QueryRowContext(ctx, "SELECT p.body->'input'->>'credential_generation' FROM ip_mutation_intents m JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest WHERE m.id=$1", id).Scan(&admittedGeneration); e != nil {
		return "", e
	}
	if admittedGeneration != o.CredentialGeneration {
		return "", ErrEvidence
	}
	if e = currentObservationGeneration(ctx, tx, account, o.CredentialGeneration); e != nil {
		return "", e
	}
	if o.ObservedAt.Before(responseAt) || time.Since(o.ObservedAt) > 5*time.Minute || o.ObservedAt.After(time.Now()) {
		return "", ErrEvidence
	}
	var found *providers.IPResource
	for _, r := range o.Resources {
		if r.ID == identity && r.Family == family && r.Kind == product {
			copy := r
			found = &copy
		}
	}
	if found == nil || found.Region != region || (found.ServerID != "" && found.ServerID != server) {
		return "", ErrEvidence
	}
	var existed bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_mutation_intents m JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest
 JOIN ip_execution_evidence ev ON ev.id=(p.body->'input'->'evidence_ids'->>m.server_id)::uuid
 JOIN ip_lifecycle_inventory_receipts i ON i.request_id=ev.inventory_request_id, jsonb_array_elements(i.observation->'resources') x
 WHERE m.id=$1 AND x->>'id'=$2 AND x->>'kind'=m.product AND (x->>'family')::integer=m.family)`, id, identity).Scan(&existed)
	if e != nil {
		return "", e
	}
	if existed {
		return "", ErrProtected
	}
	prefix := found.Prefix
	if prefix == "" {
		a, err := NormalizeAddress(found.Address)
		if err != nil {
			return "", err
		}
		prefix = netip.PrefixFrom(a, a.BitLen()).String()
	}
	p, e := netip.ParsePrefix(prefix)
	if e != nil || p.Addr().Is4() != (family == 4) {
		return "", ErrEvidence
	}
	if e = retainInventory(ctx, tx, account, readRequest); e != nil {
		return "", e
	}
	var allocation string
	e = tx.QueryRowContext(ctx, `INSERT INTO ip_allocations(account_id,provider,family,product,provider_resource_id,server_id,region,acquisition_operation,parent_prefix,acquired_at,last_observed_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::cidr,$10,$11) RETURNING id::text`, account, provider, family, product, identity, server, region, id, p.Masked().String(), dispatched, o.ObservedAt).Scan(&allocation)
	if e != nil {
		return "", e
	}
	receipt, _ := json.Marshal(map[string]string{"inventory_request_id": readRequest, "provider_resource_id": identity, "allocation_id": allocation})
	_, e = tx.ExecContext(ctx, "INSERT INTO ip_mutation_receipts(operation_id,receipt_key,kind,body) VALUES($1,'ownership','OBSERVATION',$2::jsonb)", id, string(receipt))
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_mutation_intents SET state='CONFIRMED',updated_at=clock_timestamp(),error_class='' WHERE id=$1", id)
	if e != nil {
		return "", e
	}
	return allocation, tx.Commit()
}
func (s Store) SelectEndpoint(ctx context.Context, allocation, address string) (Endpoint, error) {
	var out Endpoint
	a, e := NormalizeAddress(address)
	if e != nil {
		return out, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_allocations WHERE id=$1", allocation).Scan(&account); e != nil {
		return out, e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return out, e
	}
	// All active/history checks are serialized across accounts, including mixed /56-/64 scopes.
	if _, e = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('ip-endpoint-diversity',0))"); e != nil {
		return out, e
	}
	var family int
	var parent, server, state string
	var acquired time.Time
	var raw []byte
	var revision int64
	e = tx.QueryRowContext(ctx, `SELECT a.family,a.parent_prefix::text,a.server_id,a.state,a.acquired_at,p.policy,p.revision
 FROM ip_allocations a JOIN ip_policies p ON p.account_id=a.account_id AND p.family=a.family WHERE a.id=$1 FOR UPDATE OF a`, allocation).Scan(&family, &parent, &server, &state, &acquired, &raw, &revision)
	if e != nil {
		return out, e
	}
	if state != "OWNED" || a.Is4() != (family == 4) {
		return out, ErrProtected
	}

	// Idempotent recovery returns the previously selected address without extending its lifetime.
	e = tx.QueryRowContext(ctx, `SELECT id::text,allocation_id::text,account_id::text,server_id,host(address),parent_prefix::text,COALESCE(diversity_prefix::text,''),acquired_at,expires_at,phase,revision FROM ip_endpoints WHERE allocation_id=$1 AND address=$2::inet AND retired_at IS NULL`, allocation, a.String()).Scan(&out.ID, &out.AllocationID, &out.AccountID, &out.ServerID, &out.Address, &out.ParentPrefix, &out.DiversityPrefix, &out.AcquiredAt, &out.ExpiresAt, &out.Phase, &out.Revision)
	if e == nil {
		return out, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	var approved []byte
	var approvedRevision int64
	var selections int
	e = tx.QueryRowContext(ctx, `SELECT p.body->'input'->'policy',p.policy_revision,
 (SELECT count(*) FROM ip_endpoints WHERE allocation_id=$1)
 FROM ip_allocations a JOIN ip_mutation_intents m ON m.id=a.acquisition_operation JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest WHERE a.id=$1`, allocation).Scan(&approved, &approvedRevision, &selections)
	if e != nil {
		return out, e
	}
	overSelection := selections > 0
	p, e := decodePolicy(approved)
	if e != nil || !p.Enabled {
		return out, ErrPolicy
	}
	current, e := decodePolicy(raw)
	if e != nil || !current.Enabled || revision != approvedRevision {
		return out, ErrRevision
	}
	parentNet, e := netip.ParsePrefix(parent)
	if e != nil || !parentNet.Contains(a) {
		return out, ErrEvidence
	}
	if family == 6 && (p.AllocationBits == nil || *p.AllocationBits != parentNet.Bits()) {
		return out, ErrPolicy
	}
	var diversity any
	var diversityText string
	if family == 6 {
		d, err := DiversityPrefix(a.String(), parent, *p.DiversityBits)
		if err != nil {
			return out, err
		}
		diversityText = d.String()
		diversity = diversityText
	}
	var blocked bool
	e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=$1 AND server_id=$2)", account, server).Scan(&blocked)
	if e != nil {
		return out, e
	}
	if blocked {
		return out, ErrProtected
	}
	e = tx.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM ip_endpoints WHERE retired_at IS NULL AND (address=$1::inet OR
 ($2::cidr IS NOT NULL AND diversity_prefix && $2::cidr AND (active_scope='GLOBAL' OR $3='GLOBAL' OR account_id=$4))))
 OR EXISTS(SELECT 1 FROM ip_endpoint_history h WHERE h.result='SELECTED' AND
 (h.history_scope='GLOBAL' OR $5='GLOBAL' OR h.account_id=$4) AND
 h.at>clock_timestamp()-GREATEST(h.history_seconds,$6)*interval '1 second' AND
 (h.address=$1::inet OR ($2::cidr IS NOT NULL AND h.diversity_prefix && $2::cidr)))`, a.String(), diversity, p.ActiveScope, account, p.HistoryScope, *p.HistorySeconds).Scan(&blocked)
	if e != nil {
		return out, e
	}
	blocked = blocked || overSelection
	history := func(endpoint any, result string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO ip_endpoint_history(account_id,server_id,allocation_id,endpoint_id,address,parent_prefix,diversity_prefix,active_scope,history_scope,history_seconds,result)
 VALUES($1,$2,$3,$4,$5::inet,$6::cidr,$7::cidr,$8,$9,$10,$11)`, account, server, allocation, endpoint, a.String(), parent, diversity, p.ActiveScope, p.HistoryScope, *p.HistorySeconds, result)
		return err
	}
	if blocked {
		if e = history(nil, "DUPLICATE"); e != nil {
			return out, e
		}
		if e = tx.Commit(); e != nil {
			return out, e
		}
		return out, ErrDiversity
	}
	life, e := ChooseSeconds(*p.LifetimeMinSeconds, *p.LifetimeMaxSeconds)
	if e != nil {
		return out, e
	}
	if !acquired.Add(time.Duration(life) * time.Second).After(time.Now()) {
		return out, ErrProtected
	}
	out = Endpoint{AllocationID: allocation, AccountID: account, ServerID: server, Address: a.String(), ParentPrefix: parent, DiversityPrefix: diversityText, AcquiredAt: acquired, Phase: "ALLOCATED", Revision: 1}
	e = tx.QueryRowContext(ctx, `INSERT INTO ip_endpoints(allocation_id,account_id,server_id,address,parent_prefix,diversity_prefix,active_scope,history_scope,history_seconds,policy_revision,acquired_at,lifetime_seconds,expires_at)
 VALUES($1,$2,$3,$4::inet,$5::cidr,$6::cidr,$7,$8,$9,$10,$11,$12::integer,$11::timestamptz+$12::integer*interval '1 second') RETURNING id::text,expires_at`, allocation, account, server, out.Address, parent, diversity, p.ActiveScope, p.HistoryScope, *p.HistorySeconds, revision, acquired, life).Scan(&out.ID, &out.ExpiresAt)
	if e != nil {
		return out, e
	}
	if e = history(out.ID, "SELECTED"); e != nil {
		return out, e
	}
	return out, tx.Commit()
}
