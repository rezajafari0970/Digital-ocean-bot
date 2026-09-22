package network

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
)

var ErrAccountContextMismatch = errors.New("account network context mismatch")

type AccountContext struct {
	AccountID string
	Profile   Profile
	Client    *http.Client
	Jar       http.CookieJar
	SessionID string
}

func AccountNamespace(accountID string) string {
	sum := sha256.Sum256([]byte(accountID))
	return hex.EncodeToString(sum[:16])
}

func (c *AccountContext) Validate(accountID string) error {
	if c == nil || c.AccountID == "" || c.AccountID != accountID {
		return ErrAccountContextMismatch
	}
	if c.Profile.AccountID != accountID {
		return ErrAccountContextMismatch
	}
	return nil
}
