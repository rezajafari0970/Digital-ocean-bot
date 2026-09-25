package provisioning

import (
	"context"
	"database/sql"
	"errors"
)

var ErrRunNotFound = errors.New("provision run not found")

type Store interface {
	Reserve(context.Context, Run) (Run, bool, error)
	Update(context.Context, Run) error
}
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Reserve(ctx context.Context, r Run) (Run, bool, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `INSERT INTO provision_runs(id,account_id,droplet_id,state,current_step,attempt) VALUES(gen_random_uuid(),$1,$2,$3,$4,0) ON CONFLICT(account_id,droplet_id) DO NOTHING RETURNING id::text`, r.AccountID, r.DropletID, r.State, r.CurrentStep).Scan(&id)
	if err == nil {
		r.ID = id
		return r, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, err
	}
	err = s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,droplet_id::text,state,current_step,attempt,COALESCE(last_error,''),next_retry_at,created_at,updated_at FROM provision_runs WHERE account_id=$1 AND droplet_id=$2`, r.AccountID, r.DropletID).Scan(&r.ID, &r.AccountID, &r.DropletID, &r.State, &r.CurrentStep, &r.Attempt, &r.LastError, &r.NextRetryAt, &r.CreatedAt, &r.UpdatedAt)
	return r, false, err
}

func (s SQLStore) Update(ctx context.Context, r Run) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE provision_runs SET state=$3,current_step=$4,attempt=$5,last_error=NULLIF($6,''),next_retry_at=$7,updated_at=now() WHERE id=$1 AND account_id=$2`, r.ID, r.AccountID, r.State, r.CurrentStep, r.Attempt, r.LastError, r.NextRetryAt)
	return err
}
