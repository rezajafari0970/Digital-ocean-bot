package secrets

import (
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
}

func NewStore(repo Repository, masterKey []byte, keyVersion int) (*Store, error) {
	if len(masterKey) != 32 {
		return nil, ErrInvalidMasterKey
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Store{repo: repo, aead: aead, keyVersion: keyVersion}, nil
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
	pt, err := s.aead.Open(nil, r.Nonce, r.Ciphertext, aad)
	if err != nil {
		return nil, ErrSecretDecrypt
	}
	return pt, nil
}
