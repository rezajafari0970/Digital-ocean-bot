package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrDeploymentNotFound = errors.New("deployment not found")
var ErrDeploymentVersionConflict = errors.New("deployment version conflict")
var ErrStepRetryDeferred = errors.New("workflow step retry deferred")
var ErrStepRetryLimit = errors.New("workflow step retry limit reached")
var ErrStepTerminal = errors.New("workflow step has terminal failure")

type Store interface {
	AdmitStep(context.Context, *Deployment, string, int) error
	// Finalize atomically commits deployment, mandatory event and optional resource effects.
	Finalize(context.Context, *Deployment, string, string, func(context.Context, DBTX) error) error
	Reserve(context.Context, Request) (Deployment, bool, error)
	Update(context.Context, *Deployment) error
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
		err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),COALESCE(host,''),state,current_step,attempt,lock_version,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE id=$1 AND account_id=$2`, r.DeploymentID, r.AccountID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.Host, &d.State, &d.CurrentStep, &d.Attempt, &d.LockVersion, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
		return d, false, err
	}
	err := s.DB.QueryRowContext(ctx, `INSERT INTO deployments(id,account_id,profile_id,state,current_step) VALUES(gen_random_uuid(),$1,$2,'PLANNED','create') RETURNING id::text,account_id::text,profile_id::text,state,current_step,attempt,lock_version,created_at,updated_at`, r.AccountID, r.ProfileID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.State, &d.CurrentStep, &d.Attempt, &d.LockVersion, &d.CreatedAt, &d.UpdatedAt)
	return d, true, err
}

func (s SQLStore) Update(ctx context.Context, d *Deployment) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE deployments SET droplet_id=NULLIF($3,'')::uuid,provider_id=NULLIF($4,''),host=NULLIF($5,''),state=$6,current_step=$7,attempt=$8,last_error=NULLIF($9,''),lock_version=lock_version+1,updated_at=now() WHERE id=$1 AND account_id=$2 AND lock_version=$10`, d.ID, d.AccountID, d.DropletID, d.ProviderID, d.Host, d.State, d.CurrentStep, d.Attempt, d.LastError, d.LockVersion)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrDeploymentVersionConflict
	}
	d.LockVersion++
	return nil
}
func (s SQLStore) Event(ctx context.Context, id, step string, state State, message string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,$2,$3,NULLIF($4,''))`, id, step, state, message)
	return err
}

func (s SQLStore) Get(ctx context.Context, id, accountID string) (Deployment, error) {
	var d Deployment
	err := s.DB.QueryRowContext(ctx, `SELECT id::text,account_id::text,profile_id::text,COALESCE(droplet_id::text,''),COALESCE(provider_id,''),COALESCE(host,''),state,current_step,attempt,lock_version,COALESCE(last_error,''),created_at,updated_at FROM deployments WHERE id=$1 AND account_id=$2`, id, accountID).Scan(&d.ID, &d.AccountID, &d.ProfileID, &d.DropletID, &d.ProviderID, &d.Host, &d.State, &d.CurrentStep, &d.Attempt, &d.LockVersion, &d.LastError, &d.CreatedAt, &d.UpdatedAt)
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
	err = s.DB.QueryRowContext(ctx, `INSERT INTO deployment_step_attempts(deployment_id,step,generation,attempts,last_started_at,last_error,last_error_class,next_retry_at) SELECT $1,$2,CASE WHEN $2 IN ('database','panel') THEN postinstall_generation ELSE 0 END,1,now(),NULL,NULL,NULL FROM deployments WHERE id=$1 ON CONFLICT(deployment_id,step,generation) DO UPDATE SET attempts=deployment_step_attempts.attempts+1,last_started_at=now(),last_error=NULL,last_error_class=NULL,next_retry_at=NULL,result_snapshot=NULL RETURNING attempts`, deploymentID, step).Scan(&n)
	return n, err
}
func (s SQLStore) FinishStep(ctx context.Context, deploymentID, step string, stepErr error, class ErrorClass) error {
	return s.finishStep(ctx, deploymentID, step, stepErr, class, nil)
}
func (s SQLStore) finishStep(ctx context.Context, deploymentID, step string, stepErr error, class ErrorClass, result any) error {
	msg := ""
	if stepErr != nil {
		msg = stepErr.Error()
	}
	var next any
	if stepErr != nil && RetryableClass(class) {
		var attempts int
		if err := s.DB.QueryRowContext(ctx, `SELECT a.attempts FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id WHERE a.deployment_id=$1 AND a.step=$2 AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END`, deploymentID, step).Scan(&attempts); err != nil {
			return err
		}
		if attempts < 1 {
			attempts = 1
		}
		backoff := time.Duration(1<<minInt(attempts, 6)) * time.Second
		next = time.Now().Add(backoff)
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE deployment_step_attempts a SET last_finished_at=now(),last_error=NULLIF($3,''),last_error_class=NULLIF($4,''),next_retry_at=$5,result_snapshot=$6::jsonb FROM deployments d WHERE a.deployment_id=$1 AND a.step=$2 AND d.id=a.deployment_id AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END`, deploymentID, step, msg, string(class), next, result)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrDeploymentVersionConflict
	}
	return nil
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// A durable result distinguishes a lost acknowledgement from a native retry.
// It is scoped to the current post-install generation and never replaces fresh
// identity, version, or attempt fields from the reserved deployment.
func (s SQLStore) ReadCompletedStep(ctx context.Context, d Deployment, step string) (Deployment, bool, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT a.result_snapshot FROM deployment_step_attempts a JOIN deployments d ON d.id=a.deployment_id
 WHERE a.deployment_id=$1 AND a.step=$2 AND d.account_id=$3
 AND a.generation=CASE WHEN $2 IN ('database','panel') THEN d.postinstall_generation ELSE 0 END
 AND a.result_snapshot IS NOT NULL AND a.last_error IS NULL AND a.last_error_class IS NULL
 AND a.last_finished_at>=a.last_started_at`, d.ID, step, d.AccountID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return d, false, nil
	}
	if err != nil {
		return d, false, err
	}
	var saved Deployment
	if err = json.Unmarshal(raw, &saved); err != nil {
		return d, false, err
	}
	if saved.ID != d.ID || saved.AccountID != d.AccountID || saved.ProfileID != d.ProfileID {
		return d, false, fmt.Errorf("workflow result identity mismatch")
	}
	d.DropletID = saved.DropletID
	d.ProviderID = saved.ProviderID
	d.Host = saved.Host
	if saved.State == WaitingInstaller {
		d.State = WaitingInstaller
		d.CurrentStep = "provision"
		d.LastError = saved.LastError
	}
	return d, true, nil
}
func (s SQLStore) FinishStepResult(ctx context.Context, d Deployment, step string, stepErr error, class ErrorClass) error {
	var raw any
	if stepErr == nil {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	return s.finishStep(ctx, d.ID, step, stepErr, class, raw)
}

// Finalize commits the deployment, resource finalizer and terminal event as one
// unit. Only publish the new optimistic version after the transaction commits.
func (s SQLStore) Finalize(ctx context.Context, d *Deployment, step, message string, apply func(context.Context, DBTX) error) error {
	db, ok := s.DB.(*sql.DB)
	if !ok {
		return fmt.Errorf("workflow finalization requires database transaction owner")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	pending := *d
	if err = (SQLStore{DB: tx}).Update(ctx, &pending); err != nil {
		return err
	}
	if apply != nil {
		if err = apply(ctx, tx); err != nil {
			return err
		}
	}
	if err = (SQLStore{DB: tx}).Event(ctx, d.ID, step, d.State, message); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*d = pending
	return nil
}

// AdmitStep charges an attempt only together with the required pre-native state
// and event. A persistence outage cannot exhaust native retry budgets.
func (s SQLStore) AdmitStep(ctx context.Context, d *Deployment, step string, maxAttempts int) error {
	db, ok := s.DB.(*sql.DB)
	if !ok {
		return errors.New("workflow admission requires transaction owner")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	store := SQLStore{DB: tx}
	pending := *d
	if _, err = store.BeginStep(ctx, d.ID, step, maxAttempts); err != nil {
		return err
	}
	if err = store.Update(ctx, &pending); err != nil {
		return err
	}
	if err = store.Event(ctx, d.ID, step, d.State, ""); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*d = pending
	return nil
}
