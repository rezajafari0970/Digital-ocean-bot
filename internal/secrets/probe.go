package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
)

func (s *Store) PutProbe(ctx context.Context, probeID, id, kind string, plaintext []byte) error {
	repo, ok := s.repo.(SQLRepository)
	if !ok {
		return ErrSecretNotFound
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	aad := []byte("probe\x00" + probeID + "\x00" + id + "\x00" + kind)
	ct := s.aead.Seal(nil, nonce, plaintext, aad)
	_, e := repo.DB.ExecContext(ctx, `INSERT INTO secrets(id,probe_id,kind,ciphertext,nonce,key_version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(probe_id,id) WHERE probe_id IS NOT NULL DO UPDATE SET kind=EXCLUDED.kind,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,key_version=EXCLUDED.key_version,updated_at=now()`, id, probeID, kind, ct, nonce, s.keyVersion)
	return e
}

func (s *Store) GetProbe(ctx context.Context, probeID, id string) ([]byte, error) {
	repo, ok := s.repo.(SQLRepository)
	if !ok {
		return nil, ErrSecretNotFound
	}
	var kind string
	var ct, nonce []byte
	e := repo.DB.QueryRowContext(ctx, `SELECT kind,ciphertext,nonce FROM secrets WHERE probe_id=$1 AND id=$2`, probeID, id).Scan(&kind, &ct, &nonce)
	if e == sql.ErrNoRows {
		return nil, ErrSecretNotFound
	}
	if e != nil {
		return nil, e
	}
	aad := []byte("probe\x00" + probeID + "\x00" + id + "\x00" + kind)
	pt, e := s.aead.Open(nil, nonce, ct, aad)
	if e != nil {
		return nil, ErrSecretDecrypt
	}
	return pt, nil
}
