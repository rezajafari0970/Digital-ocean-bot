package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type CapacityMember struct {
	ID      string `json:"id"`
	Product string `json:"product"`
	Family  int    `json:"family"`
}
type CapacityEvidence struct {
	IncludedResources         []CapacityMember `json:"included_resources"`
	ID                        string           `json:"id"`
	Scope                     string           `json:"scope"`
	Family                    int              `json:"family"` // 0 is an explicitly documented shared-family constraint.
	Product, Region, ServerID string
	Limit, Used               *int
	Source                    string
}
type ExecutionEvidence struct {
	WireRequest            *WireRequest `json:"wire_request,omitempty"`
	ID                     string       `json:"id"`
	AccountID              string       `json:"account_id"`
	Generation             string       `json:"generation"`
	Provider               string       `json:"provider"`
	Family                 int          `json:"family"`
	Product                string       `json:"product"`
	Region                 string       `json:"region"`
	ServerID               string       `json:"server_id"`
	InventoryRequestID     string       `json:"inventory_request_id"`
	Supported, Healthy     bool
	NetworkImpact          string
	ObservedAt, ValidUntil time.Time
	Capacities             []CapacityEvidence
}
type Funding struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	Revision      int64     `json:"revision"`
	Currency      string    `json:"currency"`
	BudgetMicros  int64     `json:"budget_micros"`
	FundedUntil   time.Time `json:"funded_until"`
	MaxConcurrent int       `json:"max_concurrent"`
}

// CredentialGeneration hashes only identity/version metadata; no key material is returned.
// Runtime inventory reads compare this value both before and after the provider read.
func CredentialGeneration(ctx context.Context, q SQLFence, account string) (string, error) {
	var raw []byte
	e := q.QueryRowContext(ctx, `SELECT jsonb_build_object('account',a.id,'provider',a.provider,'secret_ref',a.secret_ref,
 'credential_version',(SELECT jsonb_build_array(s.id,s.key_version,s.updated_at) FROM secrets s WHERE s.account_id=a.id AND s.id=a.secret_ref),
 'network',(SELECT jsonb_build_array(n.mode,n.proxy_id,n.updated_at) FROM network_profiles n WHERE n.account_id=a.id))
 FROM accounts a WHERE a.id=$1`, account).Scan(&raw)
	if e != nil {
		return "", e
	}
	return digestBytes(raw), nil
}
func (s Store) SaveFunding(ctx context.Context, f Funding, expected int64) (Funding, error) {
	if f.BudgetMicros < 0 || f.BudgetMicros > 1000000000000 || (f.Currency != "USD" && f.Currency != "EUR") || !f.FundedUntil.After(time.Now()) || f.FundedUntil.After(time.Now().Add(366*24*time.Hour)) || f.MaxConcurrent < 1 || f.MaxConcurrent > 8 {
		return f, ErrPolicy
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return f, e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, f.AccountID); e != nil {
		return f, e
	}
	var exists bool
	if e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND deleted_at IS NULL)", f.AccountID).Scan(&exists); e != nil {
		return f, e
	}
	if !exists {
		return f, ErrProtected
	}
	var mixed bool
	e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_funding_reservations fr JOIN ip_mutation_intents m ON m.id=fr.operation_id WHERE m.account_id=$1 AND fr.currency<>$2 AND fr.settled_at IS NULL AND m.state NOT IN('REJECTED','CANCELLED'))", f.AccountID, f.Currency).Scan(&mixed)
	if e != nil {
		return f, e
	}
	if mixed {
		return f, ErrProtected
	}
	if expected == 0 {
		e = tx.QueryRowContext(ctx, `INSERT INTO ip_account_funding(account_id,currency,budget_micros,funded_until,max_concurrent) VALUES($1,$2,$3,$4,$5) ON CONFLICT(account_id) DO NOTHING RETURNING id::text,revision`, f.AccountID, f.Currency, f.BudgetMicros, f.FundedUntil, f.MaxConcurrent).Scan(&f.ID, &f.Revision)
	} else {
		e = tx.QueryRowContext(ctx, `UPDATE ip_account_funding SET currency=$2,budget_micros=$3,funded_until=$4,max_concurrent=$5,revision=revision+1 WHERE account_id=$1 AND revision=$6 RETURNING id::text,revision`, f.AccountID, f.Currency, f.BudgetMicros, f.FundedUntil, f.MaxConcurrent, expected).Scan(&f.ID, &f.Revision)
	}
	if errors.Is(e, sql.ErrNoRows) {
		return f, ErrRevision
	}
	if e != nil {
		return f, e
	}
	raw, _ := json.Marshal(f)
	if _, e = tx.ExecContext(ctx, "INSERT INTO ip_funding_events(account_id,revision,body) VALUES($1,$2,$3::jsonb)", f.AccountID, f.Revision, string(raw)); e != nil {
		return f, e
	}
	return f, tx.Commit()
}

// RecordExecutionEvidence is only for trusted account observers, never a settings API.
// A capability assessment must include all applicable capacity scopes; missing/unknown
// constraints are represented as nil limits and block dispatch. Live writer is not wired yet.
func (s Store) RecordExecutionEvidence(ctx context.Context, v ExecutionEvidence) (string, error) {
	now := time.Now()
	if v.WireRequest != nil {
		w, err := v.WireRequest.prepare()
		if err != nil || !wireScope(w.Spec(), v.Provider, v.Family, v.Product, v.ServerID, v.Region) || w.Impact() != v.NetworkImpact {
			return "", ErrEvidence
		}
	}
	if v.AccountID == "" || v.Generation == "" || v.Provider == "" || (v.Family != 4 && v.Family != 6) || v.Product == "" || v.Region == "" || v.ServerID == "" || v.InventoryRequestID == "" || v.NetworkImpact == "" || v.ObservedAt.After(now) || now.Sub(v.ObservedAt) > 5*time.Minute || !v.ValidUntil.After(now) || v.ValidUntil.After(v.ObservedAt.Add(5*time.Minute)) || len(v.Capacities) == 0 || len(v.Capacities) > 16 {
		return "", ErrEvidence
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, v.AccountID); e != nil {
		return "", e
	}
	generation, e := CredentialGeneration(ctx, tx, v.AccountID)
	if e != nil {
		return "", e
	}
	if generation != v.Generation {
		return "", ErrEvidence
	}
	var valid bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_inventory_events i JOIN accounts a ON a.id=i.account_id JOIN ip_inventory_lanes l ON l.account_id=i.account_id
 WHERE i.account_id=$1 AND i.request_id=$2 AND i.state='OBSERVED' AND i.observation->>'credential_generation'=$3
 AND i.observation->>'credential_generation'=l.last_good->>'credential_generation'
 AND i.observation=l.last_good AND i.started_at=l.last_good_at AND a.provider=$4
 AND i.finished_at<=clock_timestamp() AND i.started_at>=clock_timestamp()-interval '5 minutes')`, v.AccountID, v.InventoryRequestID, v.Generation, v.Provider).Scan(&valid)
	if e != nil {
		return "", e
	}
	if !valid {
		return "", ErrEvidence
	}
	var inventoryDeadline time.Time
	if e = tx.QueryRowContext(ctx, "SELECT LEAST(started_at,(observation->>'observed_at')::timestamptz)+interval '5 minutes' FROM ip_inventory_events WHERE request_id=$1", v.InventoryRequestID).Scan(&inventoryDeadline); e != nil {
		return "", e
	}
	if inventoryDeadline.Before(v.ValidUntil) {
		v.ValidUntil = inventoryDeadline
	}
	if !v.ValidUntil.After(v.ObservedAt) || !v.ValidUntil.After(time.Now()) {
		return "", ErrEvidence
	}
	if e = retainInventory(ctx, tx, v.AccountID, v.InventoryRequestID); e != nil {
		return "", e
	}
	seen := map[string]bool{}
	for i, c := range v.Capacities {
		if c.Scope == "" || len(c.Scope) > 200 || seen[c.Scope] || (c.Family != 0 && c.Family != v.Family) || (c.Product != "" && c.Product != v.Product) || (c.Region != "" && c.Region != v.Region) || (c.ServerID != "" && c.ServerID != v.ServerID) || c.Source == "" || (c.Limit != nil && *c.Limit < 0) || (c.Used != nil && *c.Used < 0) {
			return "", ErrEvidence
		}
		seen[c.Scope] = true
		if c.IncludedResources == nil {
			c.IncludedResources = []CapacityMember{}
			v.Capacities[i].IncludedResources = c.IncludedResources
		}
		if len(c.IncludedResources) > 0 && (c.Used == nil || len(c.IncludedResources) > *c.Used) {
			return "", ErrEvidence
		}
		memberSeen := map[string]bool{}
		for _, m := range c.IncludedResources {
			key := fmt.Sprintf("%d/%s/%s", m.Family, m.Product, m.ID)
			if memberSeen[key] || m.ID == "" || (m.Family != 4 && m.Family != 6) || m.Product == "" {
				return "", ErrEvidence
			}
			memberSeen[key] = true
			var included bool
			e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ip_lifecycle_inventory_receipts i,jsonb_array_elements(i.observation->'resources') x
   WHERE i.request_id=$1 AND x->>'id'=$2 AND x->>'kind'=$3 AND (x->>'family')::integer=$4
   AND ($5::integer=0 OR (x->>'family')::integer=$5) AND ($6='' OR x->>'kind'=$6)
   AND ($7='' OR x->>'region'=$7) AND ($8='' OR x->>'server_id'=$8))`, v.InventoryRequestID, m.ID, m.Product, m.Family, c.Family, c.Product, c.Region, c.ServerID).Scan(&included)
			if e != nil {
				return "", e
			}
			if !included {
				return "", ErrEvidence
			}
		}
		members, _ := json.Marshal(c.IncludedResources)
		// Reuse an identical shared scope from this exact inventory; never extend its validity.
		var sharedID string
		sameErr := tx.QueryRowContext(ctx, `SELECT id::text FROM ip_capacity_evidence WHERE account_id=$1 AND scope_key=$2 AND superseded_at IS NULL
 AND credential_generation=$3 AND inventory_request_id=$4 AND family=$5 AND product=$6 AND region=$7 AND server_id=$8
 AND resource_limit IS NOT DISTINCT FROM $9::integer AND used IS NOT DISTINCT FROM $10::integer AND source=$11 AND included_resources=$12::jsonb AND valid_until>clock_timestamp()`, v.AccountID, c.Scope, v.Generation, v.InventoryRequestID, c.Family, c.Product, c.Region, c.ServerID, c.Limit, c.Used, c.Source, string(members)).Scan(&sharedID)
		if sameErr == nil {
			v.Capacities[i].ID = sharedID
			continue
		}
		if !errors.Is(sameErr, sql.ErrNoRows) {
			return "", sameErr
		}
		// Older observers cannot supersede newer capacity evidence.
		e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_capacity_evidence WHERE account_id=$1 AND scope_key=$2 AND observed_at>$3)", v.AccountID, c.Scope, v.ObservedAt).Scan(&valid)
		if e != nil {
			return "", e
		}
		if valid {
			return "", ErrSuperseded
		}
		_, e = tx.ExecContext(ctx, "UPDATE ip_capacity_evidence SET superseded_at=clock_timestamp() WHERE account_id=$1 AND scope_key=$2 AND superseded_at IS NULL", v.AccountID, c.Scope)
		if e != nil {
			return "", e
		}
		e = tx.QueryRowContext(ctx, `INSERT INTO ip_capacity_evidence(account_id,scope_key,credential_generation,inventory_request_id,family,product,region,server_id,resource_limit,used,observed_at,valid_until,source,included_resources)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb) RETURNING id::text`, v.AccountID, c.Scope, v.Generation, v.InventoryRequestID, c.Family, c.Product, c.Region, c.ServerID, c.Limit, c.Used, v.ObservedAt, v.ValidUntil, c.Source, string(members)).Scan(&v.Capacities[i].ID)
		if e != nil {
			return "", e
		}
	}
	e = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM ip_execution_evidence WHERE account_id=$1 AND provider=$2 AND family=$3 AND product=$4 AND region=$5 AND server_id=$6 AND observed_at>$7)", v.AccountID, v.Provider, v.Family, v.Product, v.Region, v.ServerID, v.ObservedAt).Scan(&valid)
	if e != nil {
		return "", e
	}
	if valid {
		return "", ErrSuperseded
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_execution_evidence SET superseded_at=clock_timestamp() WHERE account_id=$1 AND provider=$2 AND family=$3 AND product=$4 AND region=$5 AND server_id=$6 AND superseded_at IS NULL", v.AccountID, v.Provider, v.Family, v.Product, v.Region, v.ServerID)
	if e != nil {
		return "", e
	}
	raw, _ := json.Marshal(v)
	var id string
	e = tx.QueryRowContext(ctx, `INSERT INTO ip_execution_evidence(account_id,credential_generation,provider,family,product,region,server_id,inventory_request_id,body,observed_at,valid_until)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11) RETURNING id::text`, v.AccountID, v.Generation, v.Provider, v.Family, v.Product, v.Region, v.ServerID, v.InventoryRequestID, string(raw), v.ObservedAt, v.ValidUntil).Scan(&id)
	if e != nil {
		return "", e
	}
	return id, tx.Commit()
}
func executionEvidence(ctx context.Context, tx SQLFence, p Plan, a Assignment) (ExecutionEvidence, error) {
	var v ExecutionEvidence
	id := p.Input.EvidenceIDs[a.ServerID]
	if id == "" {
		return v, ErrEvidence
	}
	var raw []byte
	e := tx.QueryRowContext(ctx, `SELECT e.body FROM ip_execution_evidence e JOIN accounts ac ON ac.id=e.account_id
 WHERE e.id=$1 AND e.account_id=$2 AND e.provider=ac.provider AND e.family=$3 AND e.product=$4 AND e.region=$5 AND e.server_id=$6
 AND e.credential_generation=$7 AND e.superseded_at IS NULL AND e.observed_at<=clock_timestamp() AND e.valid_until>clock_timestamp()
 AND EXISTS(SELECT 1 FROM ip_inventory_events i JOIN ip_inventory_lanes l ON l.account_id=i.account_id WHERE i.request_id=e.inventory_request_id AND i.account_id=e.account_id AND i.state='OBSERVED' AND i.observation=l.last_good AND i.started_at=l.last_good_at
 AND i.observation->>'credential_generation'=e.credential_generation AND i.started_at>clock_timestamp()-interval '5 minutes' AND (i.observation->>'observed_at')::timestamptz>clock_timestamp()-interval '5 minutes')`, id, p.Input.AccountID, p.Input.Policy.Family, p.Input.Policy.Product, a.Region, a.ServerID, p.Input.CredentialGeneration).Scan(&raw)
	if e != nil {
		return v, ErrEvidence
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		return v, e
	}
	if !v.Supported || !v.Healthy || v.NetworkImpact != a.NetworkImpact {
		return v, ErrEvidence
	}
	if _, e = boundWire(p, a, v, false); e != nil {
		return v, e
	}
	generation, e := CredentialGeneration(ctx, tx, p.Input.AccountID)
	if e != nil {
		return v, e
	}
	if generation != v.Generation {
		return v, ErrEvidence
	}
	return v, nil
}
func capacityHeadroom(ctx context.Context, tx SQLFence, account string, c CapacityEvidence, generation string) (int, error) {
	var limit, used sql.NullInt64
	var reserved int
	var valid bool
	e := tx.QueryRowContext(ctx, `SELECT resource_limit,used,
 superseded_at IS NULL AND credential_generation=$2 AND valid_until>clock_timestamp() AND observed_at<=clock_timestamp()
 AND EXISTS(SELECT 1 FROM ip_lifecycle_inventory_receipts i WHERE i.request_id=inventory_request_id AND i.started_at>clock_timestamp()-interval '5 minutes' AND (i.observation->>'observed_at')::timestamptz>clock_timestamp()-interval '5 minutes' AND i.observation->>'credential_generation'=$2)
 FROM ip_capacity_evidence WHERE id=$1 AND account_id=$3`, c.ID, generation, account).Scan(&limit, &used, &valid)
	if e != nil || !valid || !limit.Valid || !used.Valid {
		return 0, ErrEvidence
	}
	// Only explicit membership in Used discharges a scope reservation.
	// Detached/other-server presence never frees an attachment slot.
	e = tx.QueryRowContext(ctx, `SELECT count(*) FROM ip_operation_reservations r JOIN ip_mutation_intents m ON m.id=r.operation_id
 WHERE m.account_id=$1 AND r.scope_key=$2 AND m.state NOT IN('REJECTED','CANCELLED')
 AND NOT EXISTS(
 SELECT 1 FROM ip_allocations a JOIN ip_capacity_evidence ce ON ce.id=$3 WHERE a.acquisition_operation=m.id AND a.state='OWNED' AND EXISTS(SELECT 1 FROM jsonb_array_elements(ce.included_resources) x
 WHERE x->>'id'=a.provider_resource_id AND x->>'product'=a.product AND (x->>'family')::integer=a.family))`, account, c.Scope, c.ID).Scan(&reserved)
	if e != nil {
		return 0, e
	}
	return int(limit.Int64-used.Int64) - reserved, nil
}

func retainInventory(ctx context.Context, tx SQLFence, account, request string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO ip_lifecycle_inventory_receipts(request_id,account_id,started_at,finished_at,observation)
 SELECT request_id,account_id,started_at,finished_at,observation FROM ip_inventory_events WHERE account_id=$1 AND request_id=$2 AND state='OBSERVED'
 ON CONFLICT(request_id) DO NOTHING`, account, request)
	return e
}
