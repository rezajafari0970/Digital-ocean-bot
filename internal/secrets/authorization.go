package secrets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
)

type AuthorizationCipher struct {
	Ciphertext, Nonce []byte
	KeyVersion        int
}

func (s *Store) SealAuthorization(owner, id string, plain []byte) (AuthorizationCipher, error) {
	if owner == "" || id == "" || len(plain) == 0 || len(plain) > 65536 {
		return AuthorizationCipher{}, errors.New("invalid authorization secret")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return AuthorizationCipher{}, e
	}
	return AuthorizationCipher{s.aead.Seal(nil, nonce, plain, []byte("provider-authorization\x00"+owner+"\x00"+id)), nonce, s.keyVersion}, nil
}
func (s *Store) OpenAuthorization(owner, id string, c AuthorizationCipher) ([]byte, error) {
	key, ok := s.keys[c.KeyVersion]
	if !ok || len(c.Nonce) != key.NonceSize() {
		return nil, ErrSecretDecrypt
	}
	out, e := key.Open(nil, c.Nonce, c.Ciphertext, []byte("provider-authorization\x00"+owner+"\x00"+id))
	if e != nil {
		return nil, ErrSecretDecrypt
	}
	return out, nil
}
func (s *Store) validateAuthorizationKeys(ctx context.Context) error {
	repo, ok := s.repo.(SQLRepository)
	if !ok {
		return nil
	}
	var exists bool
	if e := repo.DB.QueryRowContext(ctx, "SELECT to_regclass('provider_authorizations') IS NOT NULL").Scan(&exists); e != nil {
		return e
	}
	if !exists {
		return nil
	}
	rows, e := repo.DB.QueryContext(ctx, "SELECT owner_id,id::text,ciphertext,nonce,key_version FROM provider_authorizations WHERE ciphertext IS NOT NULL AND expires_at>now()")
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var owner, id string
		var c AuthorizationCipher
		if e = rows.Scan(&owner, &id, &c.Ciphertext, &c.Nonce, &c.KeyVersion); e != nil {
			return e
		}
		plain, e := s.OpenAuthorization(owner, id, c)
		if e != nil {
			return errors.New("pending authorization key cannot be retired")
		}
		for i := range plain {
			plain[i] = 0
		}
	}
	return rows.Err()
}

// PutInTransaction binds an account credential to the caller's atomic account
// creation transaction. It uses the same encryption and AAD as Put.
func (s *Store) PutInTransaction(ctx context.Context, tx *sql.Tx, accountID, id, kind string, plain []byte) error {
	if tx == nil || accountID == "" || id == "" || kind == "" {
		return errors.New("invalid transactional secret")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return e
	}
	ct := s.aead.Seal(nil, nonce, plain, []byte(accountID+"\x00"+id+"\x00"+kind))
	_, e := tx.ExecContext(ctx, `INSERT INTO secrets(id,account_id,kind,ciphertext,nonce,key_version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_key,id) DO UPDATE SET kind=EXCLUDED.kind,ciphertext=EXCLUDED.ciphertext,nonce=EXCLUDED.nonce,key_version=EXCLUDED.key_version,updated_at=now()`, id, accountID, kind, ct, nonce, s.keyVersion)
	return e
}
