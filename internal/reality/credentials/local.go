package credentials

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/curve25519"
)

// LocalX25519Generator creates X25519 keys locally so runtime reconciliation
// never needs SSH/Xray merely to generate Reality credentials.
type LocalX25519Generator struct{}

func (LocalX25519Generator) Generate(context.Context) (KeyPair, error) {
	private := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(private); err != nil {
		return KeyPair{}, err
	}
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		Wipe(private)
		return KeyPair{}, err
	}

	enc := base64.RawURLEncoding
	privateText := []byte(enc.EncodeToString(private))
	publicText := enc.EncodeToString(public)
	Wipe(private)
	return KeyPair{Private: privateText, Public: publicText}, nil
}
