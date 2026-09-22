package network

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

func randomRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type MetadataTransport struct{ Base http.RoundTripper }

func (t MetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Del("X-Account-ID")
	clone.Header.Del("X-Cell-ID")
	clone.Header.Del("X-Internal-Job-ID")
	id, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	clone.Header.Set("X-Request-ID", id)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
