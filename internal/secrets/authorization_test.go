package secrets

import (
	"bytes"
	"testing"
)

func TestAuthorizationSecretsOwnerBindingAndRotation(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	s, e := NewStore(&memRepo{}, key, 1)
	if e != nil {
		t.Fatal(e)
	}
	plain := []byte("fixture-consumer-secret")
	c, e := s.SealAuthorization("owner", "request", plain)
	if e != nil || bytes.Contains(c.Ciphertext, plain) {
		t.Fatal("unencrypted authorization", e)
	}
	for _, pair := range [][2]string{{"other", "request"}, {"owner", "other"}} {
		if _, e := s.OpenAuthorization(pair[0], pair[1], c); e == nil {
			t.Fatal("authorization not owner/request bound")
		}
	}
	rotated, e := NewStoreWithKeys(&memRepo{}, bytes.Repeat([]byte{2}, 32), 2, map[int][]byte{1: key})
	if e != nil {
		t.Fatal(e)
	}
	got, e := rotated.OpenAuthorization("owner", "request", c)
	if e != nil || !bytes.Equal(got, plain) {
		t.Fatal("retained key lost", e)
	}
	missing, _ := NewStore(&memRepo{}, bytes.Repeat([]byte{2}, 32), 2)
	if _, e := missing.OpenAuthorization("owner", "request", c); e == nil {
		t.Fatal("unknown key accepted")
	}
	c.Ciphertext[0] ^= 1
	if _, e := s.OpenAuthorization("owner", "request", c); e == nil {
		t.Fatal("tampered credential accepted")
	}
}
