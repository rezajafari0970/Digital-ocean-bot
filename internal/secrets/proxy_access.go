package secrets

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) GetProxy(ctx context.Context, proxyID, id string) ([]byte, error) {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return nil, ErrSecretNotFound
	}
	var kind string
	var ct, nonce []byte
	var version int
	err := repo.DB.QueryRowContext(ctx, `SELECT kind,ciphertext,nonce,key_version FROM secrets WHERE proxy_id=$1 AND id=$2`, proxyID, id).Scan(&kind, &ct, &nonce, &version)
	if err == sql.ErrNoRows {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	aad := []byte("proxy\x00" + proxyID + "\x00" + id + "\x00" + kind)
	pt, err := s.openRecord(version, nonce, ct, aad)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return pt, nil
}

// GetProxyAtVersion binds plaintext to the immutable definition snapshot before
// any network I/O. A publication CAS alone cannot prevent credential disclosure
// if a newer password were sent to the snapshot's older endpoint.
var ErrProxySecretVersionChanged = errors.New("proxy credential snapshot changed")

func (s *Store) GetProxyAtVersion(ctx context.Context, proxyID, id, expected string) ([]byte, error) {
	if s == nil || expected == "" {
		return nil, ErrProxySecretVersionChanged
	}
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return nil, ErrSecretNotFound
	}
	return s.getProxyAtVersionQuery(ctx, repo.DB, proxyID, id, expected)
}

// GetProxyAtVersionTx reuses the caller's locked definition transaction. This
// prevents preview requests from needing a second pooled SQL connection.
func (s *Store) GetProxyAtVersionTx(ctx context.Context, tx *sql.Tx, proxyID, id, expected string) ([]byte, error) {
	if s == nil || tx == nil || expected == "" {
		return nil, ErrProxySecretVersionChanged
	}
	return s.getProxyAtVersionQuery(ctx, tx, proxyID, id, expected)
}

func (s *Store) getProxyAtVersionQuery(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, proxyID, id, expected string) ([]byte, error) {
	var kind string
	var ct, nonce []byte
	var version int
	err := query.QueryRowContext(ctx, `SELECT kind,ciphertext,nonce,key_version FROM secrets
 WHERE proxy_id=$1 AND id=$2 AND xmin::text||':'||updated_at::text=$3`, proxyID, id, expected).Scan(&kind, &ct, &nonce, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProxySecretVersionChanged
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := s.openRecord(version, nonce, ct, []byte("proxy\x00"+proxyID+"\x00"+id+"\x00"+kind))
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return plaintext, nil
}

func (s *Store) DeleteProxy(ctx context.Context, proxyID, id string) error {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return ErrSecretNotFound
	}
	_, err := repo.DB.ExecContext(ctx, `DELETE FROM secrets WHERE proxy_id=$1 AND id=$2`, proxyID, id)
	return err
}
