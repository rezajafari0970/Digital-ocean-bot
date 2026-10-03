package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
)

// Residential credentials never resolve through account proxy storage.
func (s *Store) PutResidentialTx(ctx context.Context, tx ProxySecretExecutor, endpoint, id, kind string, plaintext []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	aad := []byte("residential\x00" + endpoint + "\x00" + id + "\x00" + kind)
	ct := s.aead.Seal(nil, nonce, plaintext, aad)
	_, err := tx.ExecContext(ctx, `INSERT INTO residential_proxy_secrets(residential_id,id,kind,ciphertext,nonce,key_version,aad_scope)
 VALUES($1,$2,$3,$4,$5,$6,'residential') ON CONFLICT(residential_id,id) DO UPDATE SET kind=excluded.kind,
 ciphertext=excluded.ciphertext,nonce=excluded.nonce,key_version=excluded.key_version,aad_scope='residential',updated_at=now()`, endpoint, id, kind, ct, nonce, s.keyVersion)
	return err
}
func (s *Store) GetResidential(ctx context.Context, endpoint, id string) ([]byte, error) {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return nil, ErrSecretNotFound
	}
	var kind, scope string
	var ct, nonce []byte
	err := repo.DB.QueryRowContext(ctx, `SELECT kind,aad_scope,ciphertext,nonce FROM residential_proxy_secrets WHERE residential_id=$1 AND id=$2`, endpoint, id).Scan(&kind, &scope, &ct, &nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, err
	}
	aad := []byte(scope + "\x00" + endpoint + "\x00" + id + "\x00" + kind)
	pt, err := s.aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return pt, nil
}
