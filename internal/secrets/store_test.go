package secrets

import (
	"context"
	"errors"
	"testing"
)

type memRepo struct{ records map[string]Record }

func (m *memRepo) Put(_ context.Context, r Record) error {
	if m.records == nil {
		m.records = map[string]Record{}
	}
	m.records[r.AccountID+":"+r.ID] = r
	return nil
}
func (m *memRepo) Get(_ context.Context, accountID, id string) (Record, error) {
	r, ok := m.records[accountID+":"+id]
	if !ok {
		return Record{}, errors.New("not found")
	}
	return r, nil
}

func TestSecretsAreEncryptedAndTenantBound(t *testing.T) {
	repo := &memRepo{}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	s, err := NewStore(repo, key, 1)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("super-secret-token")
	if err := s.Put(context.Background(), "a", "token", "digitalocean", plain); err != nil {
		t.Fatal(err)
	}
	r := repo.records["a:token"]
	if string(r.Ciphertext) == string(plain) {
		t.Fatal("plaintext stored")
	}
	got, err := s.Get(context.Background(), "a", "token")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatal("wrong plaintext")
	}
	if _, err := s.Get(context.Background(), "b", "token"); err == nil {
		t.Fatal("cross-account secret access must fail")
	}
}
