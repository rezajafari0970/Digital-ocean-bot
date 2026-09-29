package credentials

import (
	"context"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func TestLocalX25519GeneratorCompatibility(t *testing.T) {
	k, err := (LocalX25519Generator{}).Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer Wipe(k.Private)
	priv, err := base64.RawURLEncoding.DecodeString(string(k.Private))
	if err != nil || len(priv) != curve25519.ScalarSize {
		t.Fatalf("private format len=%d err=%v", len(priv), err)
	}
	pub, err := base64.RawURLEncoding.DecodeString(k.Public)
	if err != nil || len(pub) != curve25519.PointSize {
		t.Fatalf("public format len=%d err=%v", len(pub), err)
	}
	want, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(pub) {
		t.Fatal("public key does not match private key")
	}
}
