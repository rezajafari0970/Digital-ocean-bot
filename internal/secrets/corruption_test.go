package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
)

func secretReadWithoutPanic(t *testing.T, read func() ([]byte, error)) ([]byte, error) {
	t.Helper()
	var out []byte
	var err error
	func() {
		defer func() {
			if recover() != nil {
				t.Error("malformed encrypted record panicked")
				err = ErrSecretDecrypt
			}
		}()
		out, err = read()
	}()
	return out, err
}

func TestSecretRecordCorruptionNeverPanics(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{0, 1, 11, 13, 128} {
		t.Run(fmt.Sprintf("nonce-%d", n), func(t *testing.T) {
			repo := &memRepo{}
			store, err := NewStore(repo, bytes.Repeat([]byte{1}, 32), 1)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Put(ctx, "owner", "key", "private", []byte("fixture-value")); err != nil {
				t.Fatal(err)
			}
			r := repo.records["owner:key"]
			r.Nonce = make([]byte, n)
			repo.records["owner:key"] = r
			out, err := secretReadWithoutPanic(t, func() ([]byte, error) { return store.Get(ctx, "owner", "key") })
			if !errors.Is(err, ErrSecretDecrypt) || len(out) != 0 {
				t.Fatal("malformed nonce returned data")
			}
		})
	}
	t.Run("repository-owner-substitution", func(t *testing.T) {
		repo := &memRepo{}
		store, err := NewStore(repo, bytes.Repeat([]byte{1}, 32), 1)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.Put(ctx, "foreign", "key", "private", []byte("foreign-fixture")); err != nil {
			t.Fatal(err)
		}
		repo.records["owner:key"] = repo.records["foreign:key"]
		if out, e := store.Get(ctx, "owner", "key"); !errors.Is(e, ErrSecretDecrypt) || len(out) != 0 {
			t.Fatal("repository returned another owner's credential")
		}
	})
}
