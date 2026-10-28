package providers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrRegionalCreateUnresolved fences a new, undispatched intent only. It never
// settles or releases the reservation of the earlier ambiguous create.
var ErrRegionalCreateUnresolved = errors.New("regional create outcome unresolved")

// RegionalCreateSession atomically prepares and claims a new compute intent.
// Acknowledged native tasks may provision concurrently; a lost acknowledgement
// prevents another create in the same account/provider/region until reconciled.
type RegionalCreateSession interface {
	BeginRegionalCreate(context.Context, string) (bool, error)
}

func (s *sqlResourceSession) BeginRegionalCreate(ctx context.Context, intent string) (bool, error) {
	var spec struct {
		Region string `json:"region"`
	}
	if json.Unmarshal([]byte(intent), &spec) != nil || spec.Region == "" || len(spec.Region) > 64 || !strings.HasPrefix(s.record.Scope, "server:") {
		return false, errors.New("invalid regional compute intent")
	}
	if s.record.Attempted || s.record.Complete {
		return false, nil
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	key := "provider-create-region:" + s.owner.AccountID + ":" + s.owner.Provider + ":" + spec.Region
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", key); err != nil {
		return false, err
	}
	var blocked bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM provider_account_resources
	 WHERE account_id=$1 AND provider=$2 AND scope<>$3 AND scope LIKE 'server:%'
	 AND completed_at IS NULL AND last_attempt_at IS NOT NULL
	 AND resource_id='' AND operation_id=''
	 AND (intent->>'region'=$4 OR COALESCE(jsonb_typeof(intent->'region'),'null')<>'string' OR intent->>'region'=''))`, s.owner.AccountID, s.owner.Provider, s.record.Scope, spec.Region).Scan(&blocked)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, ErrRegionalCreateUnresolved
	}
	// Do not prepare the intent on a blocked path: the workflow may select its
	// next persisted region without having dispatched anything in this one.
	result, err := tx.ExecContext(ctx, `UPDATE provider_account_resources r SET intent=$4::jsonb,
	 last_attempt_at=now(),missing_since=NULL,last_missing_at=NULL,missing_count=0
	 WHERE r.account_id=$1 AND r.provider=$2 AND r.scope=$3 AND r.completed_at IS NULL
	 AND r.last_attempt_at IS NULL AND r.resource_id='' AND r.operation_id=''
	 AND (r.intent='{}'::jsonb OR r.intent=$4::jsonb)
	 AND EXISTS(SELECT 1 FROM accounts a WHERE a.id=r.account_id AND a.provider=r.provider AND a.enabled AND a.deletion_requested_at IS NULL)`, s.owner.AccountID, s.owner.Provider, s.record.Scope, intent)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, errors.New("regional compute claim conflict")
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	s.record.Intent = intent
	s.record.Attempted = true
	return true, nil
}
