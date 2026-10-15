package secrets

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrInvalidMasterKey = errors.New("invalid master key")
	ErrSecretDecrypt    = errors.New("secret decrypt failed")
)

type Record struct {
	ID, AccountID, Kind string
	Ciphertext, Nonce   []byte
	KeyVersion          int
}
type Repository interface {
	Put(context.Context, Record) error
	Get(context.Context, string, string) (Record, error)
}
type Store struct {
	repo       Repository
	aead       cipher.AEAD
	keyVersion int
	keys       map[int]cipher.AEAD
}

// NewStore is the single-key compatibility entry point. It refuses a SQL store
// whose retained cleanup archives need another key; use NewStoreWithKeys.
func NewStore(repo Repository, masterKey []byte, keyVersion int) (*Store, error) {
	return NewStoreWithKeys(repo, masterKey, keyVersion, nil)
}
func NewStoreWithKeys(repo Repository, masterKey []byte, keyVersion int, retained map[int][]byte) (*Store, error) {
	if keyVersion < 1 || len(masterKey) != 32 {
		return nil, ErrInvalidMasterKey
	}
	all := map[int][]byte{}
	for version, key := range retained {
		all[version] = key
	}
	if old, ok := all[keyVersion]; ok && !bytes.Equal(old, masterKey) {
		return nil, ErrInvalidMasterKey
	}
	all[keyVersion] = masterKey
	ring := map[int]cipher.AEAD{}
	for version, key := range all {
		if version < 1 || len(key) != 32 {
			return nil, ErrInvalidMasterKey
		}
		block, e := aes.NewCipher(key)
		if e != nil {
			return nil, e
		}
		aead, e := cipher.NewGCM(block)
		if e != nil {
			return nil, e
		}
		ring[version] = aead
	}
	s := &Store{repo: repo, aead: ring[keyVersion], keyVersion: keyVersion, keys: ring}
	if e := s.ValidateIPCleanupKeys(context.Background()); e != nil {
		return nil, e
	}
	return s, nil
}

func (s *Store) Put(ctx context.Context, accountID, id, kind string, plaintext []byte) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	aad := []byte(accountID + "\x00" + id + "\x00" + kind)
	ct := s.aead.Seal(nil, nonce, plaintext, aad)
	return s.repo.Put(ctx, Record{ID: id, AccountID: accountID, Kind: kind, Ciphertext: ct, Nonce: nonce, KeyVersion: s.keyVersion})
}

func (s *Store) Get(ctx context.Context, accountID, id string) ([]byte, error) {
	r, err := s.repo.Get(ctx, accountID, id)
	if err != nil {
		return nil, err
	}
	aad := []byte(r.AccountID + "\x00" + r.ID + "\x00" + r.Kind)
	aead, ok := s.keys[r.KeyVersion]
	if !ok {
		return nil, ErrSecretDecrypt
	}
	pt, err := aead.Open(nil, r.Nonce, r.Ciphertext, aad)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return pt, nil
}
