package jobs

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrOperationNotFound = errors.New("operation not found")
var ErrOperationConflict = errors.New("operation idempotency conflict")

type Store interface {
	Reserve(context.Context, Operation) (Operation, bool, error)
	Get(context.Context, string, string) (Operation, error)
	Update(context.Context, Operation) error
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
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,kind,idempotency_key,state,COALESCE(provider_action_id,''),COALESCE(resource_id,''),attempt,created_at,updated_at FROM operations WHERE account_id=$1 AND idempotency_key=$2`, accountID, key).Scan(&o.ID, &o.AccountID, &o.Kind, &o.IdempotencyKey, &o.State, &o.ProviderActionID, &o.ResourceID, &o.Attempt, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Operation{}, ErrOperationNotFound
	}
	return o, err
}

func (s SQLStore) Update(ctx context.Context, o Operation) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE operations SET state=$3,provider_action_id=NULLIF($4,''),resource_id=NULLIF($5,''),attempt=$6,lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2`, o.ID, o.AccountID, o.State, o.ProviderActionID, o.ResourceID, o.Attempt)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrOperationNotFound
	}
	return nil
}
