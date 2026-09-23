package workflow

import (
	"context"
	"database/sql"
	"errors"
)

var ErrDeploymentNotFound = errors.New("deployment not found")

type Store interface {
	Reserve(context.Context, Request) (Deployment, bool, error)
	Update(context.Context, Deployment) error
	Event(context.Context, string, string, State, string) error
}
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) Reserve(ctx context.Context, r Request) (Deployment, bool, error) {
	var d Deployment
	err := s.DB.QueryRowContext(ctx, `INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create') ON CONFLICT(account_id,profile_id) WHERE state NOT IN ('READY','FAILED') DO NOTHING RETURNING id::text,account_id::text,profile_id::text,state,current_step,attempt,created_at,updated_at`, r.AccountID, r.ProfileID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.State, &d.CurrentStep, &d.Attempt, &d.CreatedAt, &d.UpdatedAt)
	if err == nil {
		return d, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return d, false, err
	}
	err = s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),state,current_step,attempt,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE account_id=$1 AND profile_id=$2 AND state NOT IN ('READY','FAILED') ORDER BY created_at DESC LIMIT 1`, r.AccountID, r.ProfileID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.State, &d.CurrentStep, &d.Attempt, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
	return d, false, err
}

func (s SQLStore) Update(ctx context.Context, d Deployment) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE deployments SET droplet_id=NULLIF($3,'')::uuid,provider_id=NULLIF($4,''),state=$5,current_step=$6,attempt=$7,last_error=NULLIF($8,''),lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, d.AccountID, d.DropletID, d.ProviderID, d.State, d.CurrentStep, d.Attempt, d.LastError)
	return err
}
func (s SQLStore) Event(ctx context.Context, id, step string, state State, message string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,$2,$3,NULLIF($4,''))`, id, step, state, message)
	return err
}

func (s SQLStore) Get(ctx context.Context, id, accountID string) (Deployment, error) {
	var d Deployment
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),state,current_step,attempt,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE id=$1 AND account_id=$2`, id, accountID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.State, &d.CurrentStep, &d.Attempt, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
