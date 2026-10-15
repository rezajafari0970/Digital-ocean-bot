package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

var ErrLaneBusy = errors.New("IP inventory account lane busy or cooling down")
var ErrReceiptConflict = errors.New("IP inventory receipt conflicts with durable outcome")
var ErrSuperseded = errors.New("IP inventory request superseded")

type InventoryStore struct{ DB *sql.DB }
type ReadTicket struct {
	AccountID, RequestID string
	StartedAt            time.Time
}

func (s InventoryStore) Begin(ctx context.Context, account string) (ReadTicket, error) {
	t := ReadTicket{AccountID: account}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return t, e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, account); e != nil {
		return t, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_inventory_lanes(account_id) VALUES($1) ON CONFLICT DO NOTHING`, account)
	if e != nil {
		return t, e
	}
	e = tx.QueryRowContext(ctx, `UPDATE ip_inventory_lanes SET state='READING',request_id=gen_random_uuid(),started_at=clock_timestamp(),lease_until=clock_timestamp()+interval '50 seconds',error_class='',http_status=0
 WHERE account_id=$1 AND (lease_until IS NULL OR lease_until<clock_timestamp()) AND (next_attempt_at IS NULL OR next_attempt_at<=clock_timestamp()) AND (rate_limit_until IS NULL OR rate_limit_until<=clock_timestamp()) RETURNING request_id::text,started_at`, account).Scan(&t.RequestID, &t.StartedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return t, ErrLaneBusy
	}
	if e != nil {
		return t, e
	}
	return t, tx.Commit()
}

// Finish never replaces last valid evidence with an error, unknown quota, or older result.
// Read tickets fence inventory commits; they are NOT provider-mutation authorizations.
func (s InventoryStore) Finish(ctx context.Context, t ReadTicket, o *providers.IPObservation, class string, status int, retryAfter time.Duration) error {
	if class != "" {
		o = nil
	}
	if o != nil {
		if e := providers.ValidateIPObservation(*o); e != nil {
			return e
		}
		if o.ObservedAt.Before(t.StartedAt.Add(-time.Second)) || o.ObservedAt.After(time.Now().Add(time.Second)) {
			return providers.IPDataError()
		}
	}
	if o == nil && class == "" {
		return providers.IPDataError()
	}
	switch class {
	case "", "rate_limited", "capacity", "region_capacity", "account_locked", "network", "authentication", "permission_denied", "unsupported", "transport", "unavailable", "invalid_request", "billing", "not_found", "unknown", "ambiguous_outcome":
	default:
		class = "unavailable"
	}
	restrictionSeconds := max(int64(0), int64(retryAfter/time.Second))
	if retryAfter%time.Second > 0 {
		restrictionSeconds++
	}
	if class == "rate_limited" || status == 429 {
		restrictionSeconds = max(restrictionSeconds, 60)
	}
	var raw any
	if o != nil {
		b, e := json.Marshal(o)
		if e != nil {
			return e
		}
		raw = string(b)
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, t.AccountID); e != nil {
		return e
	}
	var id string
	var failures int
	e = tx.QueryRowContext(ctx, "SELECT COALESCE(request_id::text,''),failures FROM ip_inventory_lanes WHERE account_id=$1 FOR UPDATE", t.AccountID).Scan(&id, &failures)
	if e != nil {
		return e
	}
	state := "FAILED"
	if o != nil {
		state = "OBSERVED"
	}
	if id != t.RequestID {
		state = "SUPERSEDED"
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO ip_inventory_events(account_id,request_id,started_at,state,error_class,http_status,observation,retry_after_seconds,rate_limit_until) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,CASE WHEN $8::bigint>0 THEN clock_timestamp()+($8*interval '1 second') END) ON CONFLICT(request_id) DO NOTHING`, t.AccountID, t.RequestID, t.StartedAt, state, class, status, raw, restrictionSeconds)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		var savedState string
		var same bool
		e = tx.QueryRowContext(ctx, `SELECT state, error_class=$2 AND http_status=$3 AND observation IS NOT DISTINCT FROM $4::jsonb AND account_id=$5 AND started_at=$6 AND retry_after_seconds=$7 FROM ip_inventory_events WHERE request_id=$1`, t.RequestID, class, status, raw, t.AccountID, t.StartedAt, restrictionSeconds).Scan(&savedState, &same)
		if e != nil {
			return e
		}
		if savedState == "SUPERSEDED" {
			return ErrSuperseded
		}
		wanted := "FAILED"
		if o != nil {
			wanted = "OBSERVED"
		}
		if !same || savedState != wanted {
			return ErrReceiptConflict
		}
		return tx.Commit()
	}
	// Backoff is account evidence even when an inventory result is superseded.
	_, e = tx.ExecContext(ctx, `UPDATE ip_inventory_lanes SET rate_limit_until=GREATEST(rate_limit_until,(SELECT rate_limit_until FROM ip_inventory_events WHERE request_id=$2)) WHERE account_id=$1`, t.AccountID, t.RequestID)
	if e != nil {
		return e
	}
	if state == "SUPERSEDED" {
		if e = tx.Commit(); e != nil {
			return e
		}
		return ErrSuperseded
	}
	delay := 30
	if o == nil {
		failures = min(failures+1, 10)
		delay = min(60*(1<<min(failures-1, 6)), 3600)
		j, e := ChooseSeconds(delay, delay+max(1, delay/4))
		if e != nil {
			return e
		}
		delay = max(j, int(retryAfter/time.Second)+1)
	} else {
		failures = 0
	}
	_, e = tx.ExecContext(ctx, `UPDATE ip_inventory_lanes SET state=$2,finished_at=clock_timestamp(),lease_until=NULL,next_attempt_at=GREATEST(clock_timestamp()+($3*interval '1 second'),rate_limit_until),failures=$4,error_class=$5,http_status=$6,last_good=COALESCE($7::jsonb,last_good),last_good_at=CASE WHEN $7::jsonb IS NULL THEN last_good_at ELSE $8 END WHERE account_id=$1`, t.AccountID, state, delay, failures, class, status, raw, t.StartedAt)
	if e != nil {
		return e
	}
	if o != nil {
		// A successful observation that no longer supplies a product/scope invalidates its current evidence.
		_, e = tx.ExecContext(ctx, `UPDATE ip_quota_evidence SET current_limit=NULL,used=NULL,source='not_present_in_latest_observation',observed_at=$2,capacity_changed=false WHERE account_id=$1`, t.AccountID, o.ObservedAt)
		if e != nil {
			return e
		}
		for _, q := range o.Quotas {
			_, e = tx.ExecContext(ctx, `INSERT INTO ip_quota_evidence(account_id,family,product,scope,current_limit,used,observed_at,source,last_known_limit,last_known_at,last_known_source)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$5,CASE WHEN $5::integer IS NOT NULL THEN $7::timestamptz END,CASE WHEN $5::integer IS NOT NULL THEN $8::text END)
 ON CONFLICT(account_id,family,product,scope) DO UPDATE SET
 current_limit=excluded.current_limit,used=excluded.used,observed_at=excluded.observed_at,source=excluded.source,
 capacity_changed=excluded.current_limit IS NOT NULL AND ip_quota_evidence.last_known_limit IS NOT NULL AND excluded.current_limit<>ip_quota_evidence.last_known_limit,
 last_known_limit=COALESCE(excluded.current_limit,ip_quota_evidence.last_known_limit),
 last_known_at=CASE WHEN excluded.current_limit IS NULL THEN ip_quota_evidence.last_known_at ELSE excluded.observed_at END,
 last_known_source=CASE WHEN excluded.current_limit IS NULL THEN ip_quota_evidence.last_known_source ELSE excluded.source END`, t.AccountID, q.Family, q.Kind, q.Scope, q.Limit, q.Used, q.ObservedAt, q.Source)
			if e != nil {
				return e
			}
		}
	}
	return tx.Commit()
}
