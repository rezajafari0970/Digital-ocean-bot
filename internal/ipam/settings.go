package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrRevision = errors.New("IP policy revision changed")

type PolicyRecord struct {
	AccountID string `json:"account_id"`
	Family    int    `json:"family"`
	Revision  int64  `json:"revision"`
	Policy    Policy `json:"policy"`
}
type Store struct{ DB *sql.DB }

// Policy has no paid/default operational values. Missing rows are disabled, revision zero.
func (s Store) Policy(ctx context.Context, account string, family int) (PolicyRecord, error) {
	r := PolicyRecord{AccountID: account, Family: family, Policy: Policy{Family: family}}
	if family != 4 && family != 6 {
		return r, ErrPolicy
	}
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT revision,policy FROM ip_policies WHERE account_id=$1 AND family=$2", account, family).Scan(&r.Revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r.Policy)
	return r, err
}

// SavePolicy is settings only: it does not grant execution or cleanup authority.
func (s Store) SavePolicy(ctx context.Context, account string, expected int64, p Policy) (PolicyRecord, error) {
	r := PolicyRecord{AccountID: account, Family: p.Family, Policy: p}
	if expected < 0 {
		return r, ErrPolicy
	}
	if err := p.Validate(); err != nil {
		return r, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return r, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	if err = LockAccount(ctx, tx, account); err != nil {
		return r, err
	}
	var available bool
	if err = tx.QueryRowContext(ctx, "SELECT enabled AND deleted_at IS NULL AND deletion_requested_at IS NULL FROM accounts WHERE id=$1", account).Scan(&available); err != nil {
		return r, err
	}
	if p.Enabled && !available {
		return r, ErrProtected
	}

	if expected == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO ip_policies(account_id,family,policy) VALUES($1,$2,$3::jsonb) ON CONFLICT DO NOTHING RETURNING revision`, account, p.Family, string(raw)).Scan(&r.Revision)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE ip_policies SET policy=$3::jsonb,revision=revision+1,updated_at=clock_timestamp() WHERE account_id=$1 AND family=$2 AND revision=$4 RETURNING revision`, account, p.Family, string(raw), expected).Scan(&r.Revision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrRevision
	}
	if err != nil {
		return r, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO ip_policy_events(account_id,family,revision,policy) VALUES($1,$2,$3,$4::jsonb)", account, p.Family, r.Revision, string(raw))
	if err != nil {
		return r, err
	}
	return r, tx.Commit()
}

type SQLFence interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Same exclusive account fence as native provider deletion and account purge.
// Callers must acquire it before row locks; no network IO is done inside this transaction.
func LockAccount(ctx context.Context, tx SQLFence, account string) error {
	_, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "account-mutation:"+account)
	return e
}
func CheckObligations(ctx context.Context, tx SQLFence, account, server string) error {
	var blocked bool
	if e := tx.QueryRowContext(ctx, "SELECT ip_account_obligations($1::uuid,$2)", account, server).Scan(&blocked); e != nil {
		return e
	}
	if blocked {
		return ErrProtected
	}
	return nil
}
