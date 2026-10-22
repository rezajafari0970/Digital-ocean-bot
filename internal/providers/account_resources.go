package providers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"
)

// AccountResources journals non-compute resources across failed builds and restarts.
// WithLock serializes each scope, while the provider's mutation fence independently
// prevents a deleting/disabled account from issuing any new mutation.
type AccountResource struct {
	Scope, Name, Nonce, ID string
	Attempted, Complete    bool
}
type AccountResourceSession interface {
	Record() AccountResource
	BeginAttempt(context.Context) (bool, error)
	Bind(context.Context, string) error
	Missing(context.Context) (bool, error)
	Finish(context.Context) error
}
type AccountResources interface {
	WithLock(context.Context, string, string, func(AccountResourceSession) error) error
	List(context.Context) ([]AccountResource, error)
}
type AccountResourceCleaner interface{ CleanupAccountResources(context.Context) error }
type SQLAccountResources struct {
	DB                  *sql.DB
	AccountID, Provider string
}
type sqlResourceSession struct {
	conn   *sql.Conn
	owner  SQLAccountResources
	record AccountResource
}

func (s *sqlResourceSession) Record() AccountResource { return s.record }
func (s *sqlResourceSession) BeginAttempt(ctx context.Context) (bool, error) {
	result, e := s.conn.ExecContext(ctx, `UPDATE provider_account_resources SET last_attempt_at=now(),missing_since=NULL,last_missing_at=NULL,missing_count=0 WHERE account_id=$1 AND provider=$2 AND scope=$3 AND resource_id='' AND completed_at IS NULL AND (last_attempt_at IS NULL OR last_attempt_at<now()-interval '2 minutes')`, s.owner.AccountID, s.owner.Provider, s.record.Scope)
	if e != nil {
		return false, e
	}
	n, e := result.RowsAffected()
	if n > 0 {
		s.record.Attempted = true
	}
	return n == 1, e
}
func (s *sqlResourceSession) Bind(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("resource identity missing")
	}
	result, e := s.conn.ExecContext(ctx, `UPDATE provider_account_resources SET resource_id=$4,missing_since=NULL,last_missing_at=NULL,missing_count=0 WHERE account_id=$1 AND provider=$2 AND scope=$3 AND completed_at IS NULL AND (resource_id='' OR resource_id=$4)`, s.owner.AccountID, s.owner.Provider, s.record.Scope, id)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("resource binding conflict")
	}
	s.record.ID = id
	return nil
}
func (s *sqlResourceSession) Missing(ctx context.Context) (bool, error) {
	if !s.record.Attempted && s.record.ID == "" {
		return true, nil
	}
	var stable bool
	e := s.conn.QueryRowContext(ctx, `UPDATE provider_account_resources SET missing_since=COALESCE(missing_since,now()),missing_count=CASE WHEN last_missing_at IS NULL OR last_missing_at<=now()-interval '10 seconds' THEN missing_count+1 ELSE missing_count END,last_missing_at=CASE WHEN last_missing_at IS NULL OR last_missing_at<=now()-interval '10 seconds' THEN now() ELSE last_missing_at END WHERE account_id=$1 AND provider=$2 AND scope=$3 RETURNING missing_since<=now()-interval '2 minutes' AND missing_count>=3 AND (last_attempt_at IS NULL OR last_attempt_at<=now()-interval '2 minutes')`, s.owner.AccountID, s.owner.Provider, s.record.Scope).Scan(&stable)
	return stable, e
}
func (s *sqlResourceSession) Finish(ctx context.Context) error {
	_, e := s.conn.ExecContext(ctx, `UPDATE provider_account_resources SET completed_at=now() WHERE account_id=$1 AND provider=$2 AND scope=$3`, s.owner.AccountID, s.owner.Provider, s.record.Scope)
	return e
}
func (j SQLAccountResources) WithLock(ctx context.Context, scope, name string, fn func(AccountResourceSession) error) error {
	if j.DB == nil || scope == "" || name == "" {
		return errors.New("resource journal unavailable")
	}
	conn, e := j.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	key := "provider-resource:" + j.AccountID + ":" + j.Provider + ":" + scope
	if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(hashtextextended($1,0))", key); e != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return e
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(release, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	_, e = conn.ExecContext(ctx, `INSERT INTO provider_account_resources(account_id,provider,scope,name) SELECT id,$2,$3,$4 FROM accounts WHERE id=$1 AND provider=$2 AND enabled AND deletion_requested_at IS NULL ON CONFLICT(account_id,provider,scope) DO NOTHING`, j.AccountID, j.Provider, scope, name)
	if e != nil {
		return e
	}
	session := &sqlResourceSession{conn: conn, owner: j}
	e = conn.QueryRowContext(ctx, `SELECT scope,name,nonce::text,resource_id,last_attempt_at IS NOT NULL,completed_at IS NOT NULL FROM provider_account_resources WHERE account_id=$1 AND provider=$2 AND scope=$3`, j.AccountID, j.Provider, scope).Scan(&session.record.Scope, &session.record.Name, &session.record.Nonce, &session.record.ID, &session.record.Attempted, &session.record.Complete)
	if e != nil {
		return e
	}
	if session.record.Name != name {
		return errors.New("resource intent changed")
	}
	return fn(session)
}
func (j SQLAccountResources) List(ctx context.Context) ([]AccountResource, error) {
	rows, e := j.DB.QueryContext(ctx, `SELECT scope,name,nonce::text,resource_id,last_attempt_at IS NOT NULL,completed_at IS NOT NULL FROM provider_account_resources WHERE account_id=$1 AND provider=$2 AND completed_at IS NULL ORDER BY scope`, j.AccountID, j.Provider)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AccountResource{}
	for rows.Next() {
		var x AccountResource
		if e = rows.Scan(&x.Scope, &x.Name, &x.Nonce, &x.ID, &x.Attempted, &x.Complete); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
