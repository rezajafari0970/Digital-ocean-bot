package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Kind string
type State string

const (
	KindCreate Kind = "CREATE"
	KindUpdate Kind = "UPDATE"
	KindDelete Kind = "DELETE"

	StatePending   State = "PENDING"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateFailed    State = "FAILED"
	StateObsolete  State = "OBSOLETE"
)

var (
	ErrInvalidRequest      = errors.New("invalid client mutation request")
	ErrIdempotencyConflict = errors.New("client mutation idempotency conflict")
)

type Request struct {
	AccountID      string
	PanelID        string
	InboundID      int64
	ClientID       string
	Kind           Kind
	IdempotencyKey string
	Payload        json.RawMessage
}

type Job struct {
	ID             string
	AccountID      string
	PanelID        string
	InboundID      int64
	ClientID       string
	Kind           Kind
	IdempotencyKey string
	Payload        json.RawMessage
	State          State
	Attempts       int
	NextRetryAt    *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

type Journal struct{ DB *sql.DB }

func (r Request) Validate() error {
	if strings.TrimSpace(r.AccountID) == "" ||
		strings.TrimSpace(r.PanelID) == "" ||
		r.InboundID <= 0 ||
		strings.TrimSpace(r.ClientID) == "" ||
		strings.TrimSpace(r.IdempotencyKey) == "" ||
		len(r.IdempotencyKey) > 200 {
		return ErrInvalidRequest
	}
	switch r.Kind {
	case KindCreate, KindUpdate, KindDelete:
	default:
		return ErrInvalidRequest
	}
	if len(r.Payload) == 0 || !json.Valid(r.Payload) {
		return ErrInvalidRequest
	}
	return nil
}

func sameRequest(j Job, r Request) bool {
	if j.AccountID != r.AccountID || j.PanelID != r.PanelID ||
		j.InboundID != r.InboundID || j.ClientID != r.ClientID ||
		j.Kind != r.Kind || j.IdempotencyKey != r.IdempotencyKey {
		return false
	}
	var a, b any
	if json.Unmarshal(j.Payload, &a) != nil || json.Unmarshal(r.Payload, &b) != nil {
		return false
	}
	return jsonEqual(a, b)
}

func jsonEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}

const selectJob = "SELECT id::text,account_id::text,panel_id::text,inbound_id,client_id,kind,idempotency_key,payload,state,attempts,next_retry_at,last_error,created_at,updated_at,completed_at FROM client_mutation_jobs"

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var next, completed sql.NullTime
	err := row.Scan(
		&j.ID, &j.AccountID, &j.PanelID, &j.InboundID, &j.ClientID,
		&j.Kind, &j.IdempotencyKey, &j.Payload, &j.State, &j.Attempts,
		&next, &j.LastError, &j.CreatedAt, &j.UpdatedAt, &completed,
	)
	if next.Valid {
		t := next.Time.UTC()
		j.NextRetryAt = &t
	}
	if completed.Valid {
		t := completed.Time.UTC()
		j.CompletedAt = &t
	}
	return j, err
}

func (j Journal) Get(ctx context.Context, id string) (Job, error) {
	if j.DB == nil || strings.TrimSpace(id) == "" {
		return Job{}, ErrInvalidRequest
	}
	return scanJob(j.DB.QueryRowContext(ctx, selectJob+" WHERE id=$1", id))
}

func (j Journal) Reserve(ctx context.Context, r Request) (Job, bool, error) {
	if j.DB == nil {
		return Job{}, false, ErrInvalidRequest
	}
	if err := r.Validate(); err != nil {
		return Job{}, false, err
	}
	var id string
	err := j.DB.QueryRowContext(ctx,
		"INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(account_id,idempotency_key) DO NOTHING RETURNING id::text",
		r.AccountID, r.PanelID, r.InboundID, r.ClientID, string(r.Kind),
		r.IdempotencyKey, r.Payload,
	).Scan(&id)
	if err == nil {
		job, getErr := j.Get(ctx, id)
		return job, true, getErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, err
	}
	existing, err := scanJob(j.DB.QueryRowContext(ctx,
		selectJob+" WHERE account_id=$1 AND idempotency_key=$2",
		r.AccountID, r.IdempotencyKey,
	))
	if err != nil {
		return Job{}, false, err
	}
	if !sameRequest(existing, r) {
		return Job{}, false, ErrIdempotencyConflict
	}
	return existing, false, nil
}

func (j Journal) Reconcile(ctx context.Context) error {
	if j.DB == nil {
		return ErrInvalidRequest
	}
	if _, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='PENDING',next_retry_at=now(),last_error='recovered stale runner',updated_at=now() WHERE state='RUNNING' AND updated_at<now()-interval '60 seconds'",
	); err != nil {
		return err
	}
	_, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs m SET state='OBSOLETE',next_retry_at=NULL,completed_at=now(),last_error='panel lifecycle no longer accepts client mutation',updated_at=now() WHERE m.state IN ('PENDING','RUNNING') AND (NOT EXISTS (SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=m.panel_id) OR EXISTS (SELECT 1 FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=m.panel_id AND d.state IN ('RETIRING','DELETING','DELETED')))",
	)
	return err
}

const claimSQL = "SELECT m.id::text,m.account_id::text,m.panel_id::text,m.inbound_id,m.client_id,m.kind,m.idempotency_key,m.payload,m.state,m.attempts,m.next_retry_at,m.last_error,m.created_at,m.updated_at,m.completed_at FROM client_mutation_jobs m JOIN panel_instances p ON p.id=m.panel_id JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id JOIN client_mutation_execution_gate g ON g.singleton=true WHERE g.enabled=true AND g.kill_switch=false AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=m.panel_id) AND (g.inbound_id IS NULL OR g.inbound_id=m.inbound_id) AND m.state='PENDING' AND (m.next_retry_at IS NULL OR m.next_retry_at<=now()) AND p.enabled=true AND a.enabled=true AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND EXISTS (SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=m.panel_id AND i.remote_id=m.inbound_id AND i.present=true AND i.enabled=true) ORDER BY m.created_at,m.id FOR UPDATE OF m SKIP LOCKED LIMIT 1"

func (j Journal) Claim(ctx context.Context) (Job, bool, error) {
	if j.DB == nil {
		return Job{}, false, ErrInvalidRequest
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()
	job, err := scanJob(tx.QueryRowContext(ctx, claimSQL))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	res, err := tx.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='RUNNING',attempts=attempts+1,next_retry_at=NULL,last_error='',updated_at=now() WHERE id=$1 AND state='PENDING'",
		job.ID,
	)
	if err != nil {
		return Job{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	job.State = StateRunning
	job.Attempts++
	return job, true, nil
}

func (j Journal) Succeed(ctx context.Context, id string) error {
	res, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='SUCCEEDED',completed_at=now(),next_retry_at=NULL,last_error='',updated_at=now() WHERE id=$1 AND state='RUNNING'",
		id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidRequest
	}
	return nil
}

func (j Journal) Retry(ctx context.Context, id, reason string, delay time.Duration) error {
	if delay < 0 {
		delay = 0
	}
	_, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='PENDING',next_retry_at=now()+($2*interval '1 millisecond'),last_error=$3,updated_at=now() WHERE id=$1 AND state='RUNNING'",
		id, delay.Milliseconds(), reason,
	)
	return err
}

func (j Journal) Fail(ctx context.Context, id, reason string) error {
	_, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='FAILED',completed_at=now(),next_retry_at=NULL,last_error=$2,updated_at=now() WHERE id=$1 AND state='RUNNING'",
		id, reason,
	)
	return err
}

func (j Journal) Obsolete(ctx context.Context, id, reason string) error {
	_, err := j.DB.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='OBSOLETE',completed_at=now(),next_retry_at=NULL,last_error=$2,updated_at=now() WHERE id=$1 AND state='RUNNING'",
		id, reason,
	)
	return err
}

func (j Journal) ClaimID(ctx context.Context, id string) (Job, bool, error) {
	if j.DB == nil || strings.TrimSpace(id) == "" {
		return Job{}, false, ErrInvalidRequest
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()
	const claimIDSQL = "SELECT m.id::text,m.account_id::text,m.panel_id::text,m.inbound_id,m.client_id,m.kind,m.idempotency_key,m.payload,m.state,m.attempts,m.next_retry_at,m.last_error,m.created_at,m.updated_at,m.completed_at FROM client_mutation_jobs m JOIN panel_instances p ON p.id=m.panel_id JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=d.id WHERE m.id=$1 AND m.state='PENDING' AND m.attempts=0 AND (m.next_retry_at IS NULL OR m.next_retry_at<=now()) AND p.enabled=true AND a.enabled=true AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND EXISTS (SELECT 1 FROM panel_inbound_inventory i WHERE i.panel_id=m.panel_id AND i.remote_id=m.inbound_id AND i.present=true AND i.enabled=true) FOR UPDATE OF m SKIP LOCKED"
	job, err := scanJob(tx.QueryRowContext(ctx, claimIDSQL, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	res, err := tx.ExecContext(ctx,
		"UPDATE client_mutation_jobs SET state='RUNNING',attempts=attempts+1,next_retry_at=NULL,last_error='',updated_at=now() WHERE id=$1 AND state='PENDING' AND attempts=0",
		id,
	)
	if err != nil {
		return Job{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Job{}, false, err
	}
	if n != 1 {
		return Job{}, false, ErrInvalidRequest
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	job.State = StateRunning
	job.Attempts++
	return job, true, nil
}
