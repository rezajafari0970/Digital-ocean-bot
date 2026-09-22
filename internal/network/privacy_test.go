package network

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPrivacyTransportStripsLeakHeaders(t *testing.T) {
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if HasForbiddenOutboundHeader(r.Header) {
			t.Fatal("leak header reached network transport")
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("required auth header removed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header), Request: r}, nil
	})
	tr := PrivacyTransport{Base: base}
	req, _ := http.NewRequest("GET", "https://example.invalid", nil)
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	req.Header.Set("Via", "internal")
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Forwarded-For") == "" {
		t.Fatal("original request must not be mutated")
	}
}
