package sanaei

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }
func TestLoginPreservesTransportCause(t *testing.T) {
	cause := &net.DNSError{Err: "boom", Name: "panel.invalid"}
	c, e := NewAPIClient("http://panel.invalid", Credentials{Username: "u", Password: "p"}, failingTransport{err: cause})
	if e != nil {
		t.Fatal(e)
	}
	e = c.Login(context.Background())
	if !errors.Is(e, ErrAPIRequest) || !errors.As(e, new(*net.DNSError)) {
		t.Fatalf("cause not preserved: %v", e)
	}
}
