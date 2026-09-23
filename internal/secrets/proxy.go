package secrets

import (
	"context"
	"crypto/rand"
)

func (s *Store) PutProxy(ctx context.Context, proxyID, id, kind string, plaintext []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	aad := []byte("proxy\x00" + proxyID + "\x00" + id + "\x00" + kind)
	ct := s.aead.Seal(nil, nonce, plaintext, aad)
	_, err := s.repo.(SQLRepository).DB.ExecContext(ctx, `INSERT INTO secrets(id,proxy_id,kind,ciphertext,nonce,key_version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(proxy_id,id) WHERE proxy_id IS NOT NULL DO UPDATE SET kind=EXCLUDED.kind,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,key_version=EXCLUDED.key_version,updated_at=now()`, id, proxyID, kind, ct, nonce, s.keyVersion)
	return err
}
