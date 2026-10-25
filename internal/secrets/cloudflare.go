package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
)

// Cloudflare tokens have a separate owner namespace and never leave the controller.
func (s *Store) PutCloudflareTx(ctx context.Context, tx ProxySecretExecutor, account string, plaintext []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	ct := s.aead.Seal(nil, nonce, plaintext, []byte("cloudflare\x00"+account))
	_, e := tx.ExecContext(ctx, `INSERT INTO cloudflare_secrets(account_id,ciphertext,nonce,key_version) VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET ciphertext=excluded.ciphertext,nonce=excluded.nonce,key_version=excluded.key_version`, account, ct, nonce, s.keyVersion)
	return e
}
func (s *Store) GetCloudflare(ctx context.Context, account string) ([]byte, error) {
	repo, ok := s.repo.(SQLRepository)
	if !ok || repo.DB == nil {
		return nil, ErrSecretNotFound
	}
	var ct, nonce []byte
	var version int
	e := repo.DB.QueryRowContext(ctx, "SELECT ciphertext,nonce,key_version FROM cloudflare_secrets WHERE account_id=$1", account).Scan(&ct, &nonce, &version)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	if e != nil {
		return nil, e
	}
	out, e := s.openRecord(version, nonce, ct, []byte("cloudflare\x00"+account))
	if e != nil {
		return nil, ErrSecretDecrypt
	}
	return out, nil
}
