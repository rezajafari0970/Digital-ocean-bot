package secrets

import (
	"context"
	"database/sql"
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

func (s *Store) DeleteProxy(ctx context.Context, proxyID, id string) error {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return ErrSecretNotFound
	}
	_, err := repo.DB.ExecContext(ctx, `DELETE FROM secrets WHERE proxy_id=$1 AND id=$2`, proxyID, id)
	return err
}
