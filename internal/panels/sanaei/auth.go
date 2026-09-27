package sanaei

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var ErrInsecureCredentialTransport = errors.New("refusing panel credential over plaintext http")

type BearerAuth struct {
	Secrets SecretReader

	AccountID string
	SecretRef string
}

func (a BearerAuth) Authorize(
	ctx context.Context,
	req *http.Request,
) error {

	if a.Secrets == nil ||
		a.AccountID == "" ||
		a.SecretRef == "" {
		return ErrAPIRequest
	}

	secret, err := a.Secrets.Get(
		ctx,
		a.AccountID,
		a.SecretRef,
	)

	if err != nil {
		return err
	}

	defer wipeAuth(secret)

	token := strings.TrimSpace(
		string(secret),
	)

	if token == "" {
		return ErrAPIRequest
	}

	req.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	return nil
}

type SessionCredential struct {
	Username string
	Password string
}

func ValidateSessionTransport(
	baseURL string,
) error {

	u, err := url.Parse(baseURL)
	if err != nil {
		return err
	}

	// Never send panel username/password over
	// public plaintext HTTP.
	if !strings.EqualFold(
		u.Scheme,
		"https",
	) {
		return ErrInsecureCredentialTransport
	}

	return nil
}

func wipeAuth(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
