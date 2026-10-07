package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrOperationNotFound = errors.New("operation not found")
var ErrOperationConflict = errors.New("operation idempotency conflict")
var ErrOperationVersionConflict = errors.New("operation version conflict")

type Store interface {
	Reserve(context.Context, Operation) (Operation, bool, error)
	Get(context.Context, string, string) (Operation, error)
	Update(context.Context, *Operation) error
}

type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Reserve(ctx context.Context, o Operation) (Operation, bool, error) {
	if o.AccountID == "" || o.IdempotencyKey == "" {
		return Operation{}, false, ErrOperationConflict
	}
	if o.State == "" {
		o.State = OperationPlanned
	}
	now := time.Now().UTC()
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO operations(id,account_id,kind,idempotency_key,state,attempt,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,0,$5,$5) ON CONFLICT(account_id,idempotency_key) DO NOTHING RETURNING id::text`, o.AccountID, o.Kind, o.IdempotencyKey, o.State, now).Scan(&id)
	if err == nil {
		o.ID = id
		o.ErrorCode, o.ErrorMessage = "", ""
		o.CreatedAt = now
		o.UpdatedAt = now
		return o, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Operation{}, false, err
	}
	existing, err := s.Get(ctx, o.AccountID, o.IdempotencyKey)
	return existing, false, err
}

func (s SQLStore) Get(ctx context.Context, accountID, key string) (Operation, error) {
	var o Operation
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,kind,idempotency_key,state,COALESCE(provider_action_id,''),COALESCE(resource_id,''),attempt,lock_version,created_at,updated_at,COALESCE(error_code,''),COALESCE(error_message,'') FROM operations WHERE account_id=$1 AND idempotency_key=$2`, accountID, key).Scan(&o.ID, &o.AccountID, &o.Kind, &o.IdempotencyKey, &o.State, &o.ProviderActionID, &o.ResourceID, &o.Attempt, &o.LockVersion, &o.CreatedAt, &o.UpdatedAt, &o.ErrorCode, &o.ErrorMessage)
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrOperationNotFound
	}
	if err == nil {
		o.normalizeDiagnostics()
	}
	return o, err
}

func (s SQLStore) Update(ctx context.Context, o *Operation) error {
	// Diagnostics describe the current outcome. Missing metadata on a failed or
	// unknown operation must not erase evidence written by recovery.
	code, message := normalizeDiagnostic(o.ErrorCode, o.ErrorMessage)
	err := s.DB.QueryRowContext(ctx, `UPDATE operations SET state=$3,provider_action_id=NULLIF($4,''),resource_id=NULLIF($5,''),attempt=$6,
 error_code=CASE WHEN $3 IN ('running','verifying','succeeded') THEN NULL WHEN $8<>'' OR $9<>'' THEN NULLIF($8,'') ELSE error_code END,
 error_message=CASE WHEN $3 IN ('running','verifying','succeeded') THEN NULL WHEN $8<>'' OR $9<>'' THEN NULLIF($9,'') ELSE error_message END,
 lock_version=lock_version+1,updated_at=now()
 WHERE id=$1 AND account_id=$2 AND lock_version=$7
 RETURNING COALESCE(error_code,''),COALESCE(error_message,'')`, o.ID, o.AccountID, o.State, o.ProviderActionID, o.ResourceID, o.Attempt, o.LockVersion, code, message).Scan(&o.ErrorCode, &o.ErrorMessage)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM operations WHERE id=$1 AND account_id=$2)`, o.ID, o.AccountID).Scan(&exists); e != nil {
			return e
		} else if exists {
			return ErrOperationVersionConflict
		}
		return ErrOperationNotFound
	}
	if err != nil {
		return err
	}
	o.LockVersion++
	o.normalizeDiagnostics()
	return nil
}
