package sanaei

import (
	"context"
	"net/http"
	"testing"
)

type fakeSecret []byte

func (f fakeSecret) Get(
	context.Context,
	string,
	string,
) ([]byte, error) {

	return append(
		[]byte(nil),
		f...,
	), nil
}

func TestBearerAuthReadsSecretAndSetsHeader(
	t *testing.T,
) {

	req, err := http.NewRequest(
		http.MethodGet,
		"https://panel.example/api",
		nil,
	)

	if err != nil {
		t.Fatal(err)
	}

	auth := BearerAuth{
		Secrets:   fakeSecret("abc123"),
		AccountID: "account",
		SecretRef: "secret-ref",
	}

	if err := auth.Authorize(
		context.Background(),
		req,
	); err != nil {
		t.Fatal(err)
	}

	if got := req.Header.Get(
		"Authorization",
	); got != "Bearer abc123" {

		t.Fatalf(
			"unexpected authorization header",
		)
	}
}

func TestSessionCredentialsRefusePlainHTTP(
	t *testing.T,
) {

	err := ValidateSessionTransport(
		"http://127.0.0.1:2053/random/",
	)

	if !errorsIs(
		err,
		ErrInsecureCredentialTransport,
	) {
		t.Fatalf(
			"expected insecure transport rejection, got %v",
			err,
		)
	}

	err = ValidateSessionTransport(
		"https://panel.example/random/",
	)

	if err != nil {
		t.Fatal(err)
	}
}

func errorsIs(
	got error,
	want error,
) bool {

	return got == want
}
