package secrets

import (
	"context"
	"database/sql"
	"errors"
)

var ErrSecretNotFound = errors.New("secret not found")

type SQLRepository struct{ DB *sql.DB }

func (r SQLRepository) Put(ctx context.Context, s Record) error {
	if r.DB == nil {
		return ErrSecretNotFound
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO secrets (id,account_id,kind,ciphertext,nonce,key_version) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (owner_key,id) DO UPDATE SET kind=EXCLUDED.kind,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,key_version=EXCLUDED.key_version,updated_at=now()`, s.ID, s.AccountID, s.Kind, s.Ciphertext, s.Nonce, s.KeyVersion)
	return err
}

func (r SQLRepository) Get(ctx context.Context, accountID, id string) (Record, error) {
	if r.DB == nil {
		return Record{}, ErrSecretNotFound
	}
	var s Record
	err := r.DB.QueryRowContext(ctx, `SELECT id,account_id,kind,ciphertext,nonce,key_version FROM secrets WHERE account_id=$1 AND id=$2`, accountID, id).Scan(&s.ID, &s.AccountID, &s.Kind, &s.Ciphertext, &s.Nonce, &s.KeyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrSecretNotFound
	}
	return s, err
}
