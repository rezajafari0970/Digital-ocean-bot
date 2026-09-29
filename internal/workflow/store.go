package workflow

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrDeploymentNotFound = errors.New("deployment not found")
var ErrStepRetryDeferred = errors.New("workflow step retry deferred")
var ErrStepRetryLimit = errors.New("workflow step retry limit reached")
var ErrStepTerminal = errors.New("workflow step has terminal failure")

type Store interface {
	Reserve(context.Context, Request) (Deployment, bool, error)
	Update(context.Context, Deployment) error
	Event(context.Context, string, string, State, string) error
	BeginStep(context.Context, string, string, int) (int, error)
	FinishStep(context.Context, string, string, error, ErrorClass) error
}
type DBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SQLStore struct{ DB DBTX }

func (s SQLStore) Reserve(ctx context.Context, r Request) (Deployment, bool, error) {
	var d Deployment
	if r.DeploymentID != "" {
		err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),COALESCE(host,''),state,current_step,attempt,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE id=$1 AND account_id=$2`, r.DeploymentID, r.AccountID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.Host, &d.State, &d.CurrentStep, &d.Attempt, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
		return d, false, err
	}
	err := s.DB.QueryRowContext(ctx, `INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create') RETURNING id::text,account_id::text,profile_id::text,state,current_step,attempt,created_at,updated_at`, r.AccountID, r.ProfileID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.State, &d.CurrentStep, &d.Attempt, &d.CreatedAt, &d.UpdatedAt)
	return d, true, err
}

func (s SQLStore) Update(ctx context.Context, d Deployment) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE deployments SET droplet_id=NULLIF($3,'')::uuid,provider_id=NULLIF($4,''),host=NULLIF($5,''),state=$6,current_step=$7,attempt=$8,last_error=NULLIF($9,''),lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2`, d.ID, d.AccountID, d.DropletID, d.ProviderID, d.Host, d.State, d.CurrentStep, d.Attempt, d.LastError)
	return err
}
func (s SQLStore) Event(ctx context.Context, id, step string, state State, message string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,$2,$3,NULLIF($4,''))`, id, step, state, message)
	return err
}

func (s SQLStore) Get(ctx context.Context, id, accountID string) (Deployment, error) {
	var d Deployment
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),COALESCE(host,''),state,current_step,attempt,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE id=$1 AND account_id=$2`, id, accountID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.Host, &d.State, &d.CurrentStep, &d.Attempt, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s SQLStore) BeginStep(ctx context.Context, deploymentID, step string, maxAttempts int) (int, error) {
	var attempts int
	var class sql.NullString
	var next sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT a.attempts,a.last_error_class,a.next_retry_at FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id WHERE a.deployment_id=$1 AND a.step=$2 AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END`, deploymentID, step).Scan(&attempts, &class, &next)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil {
		if class.Valid && !RetryableClass(ErrorClass(class.String)) {
			return attempts, ErrStepTerminal
		}
		if next.Valid && time.Now().Before(next.Time) {
			return attempts, ErrStepRetryDeferred
		}
		if maxAttempts > 0 && attempts >= maxAttempts {
			return attempts, ErrStepRetryLimit
		}
	}
	var n int
	err = s.DB.QueryRowContext(ctx, `INSERT INTO deployment_step_attempts(deployment_id,step,generation,attempts,last_started_at,last_error,last_error_class,next_retry_at) SELECT $1,$2,CASE WHEN $2 IN ('database','panel') THEN postinstall_generation ELSE 0 END,1,now(),NULL,NULL,NULL FROM deployments WHERE id=$1 ON CONFLICT(deployment_id,step,generation) DO UPDATE SET attempts=deployment_step_attempts.attempts+1,last_started_at=now(),last_error=NULL,last_error_class=NULL,next_retry_at=NULL RETURNING attempts`, deploymentID, step).Scan(&n)
	return n, err
}
func (s SQLStore) FinishStep(ctx context.Context, deploymentID, step string, stepErr error, class ErrorClass) error {
	msg := ""
	if stepErr != nil {
		msg = stepErr.Error()
	}
	var next any
	if stepErr != nil && RetryableClass(class) {
		var attempts int
		_ = s.DB.QueryRowContext(ctx, `SELECT a.attempts FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id WHERE a.deployment_id=$1 AND a.step=$2 AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END`, deploymentID, step).Scan(&attempts)
		if attempts < 1 {
			attempts = 1
		}
		backoff := time.Duration(1<<minInt(attempts, 6)) * time.Second
		next = time.Now().Add(backoff)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE deployment_step_attempts a SET last_finished_at=now(),last_error=NULLIF($3,''),last_error_class=NULLIF($4,''),next_retry_at=$5 FROM deployments d WHERE a.deployment_id=$1 AND a.step=$2 AND d.id=a.deployment_id AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END`, deploymentID, step, msg, string(class), next)
	return err
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
