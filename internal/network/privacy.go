package network

import (
	"errors"
	"net/http"
	"strings"
)

var ErrForbiddenOutboundHeader = errors.New("forbidden outbound header")

var forbiddenOutboundHeaders = map[string]struct{}{
	"forwarded":         {},
	"via":               {},
	"x-forwarded-for":   {},
	"x-forwarded-host":  {},
	"x-forwarded-proto": {},
	"x-real-ip":         {},
	"x-debug":           {},
}

type PrivacyTransport struct {
	Base http.RoundTripper
}

func (t PrivacyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for name := range clone.Header {
		if _, blocked := forbiddenOutboundHeaders[strings.ToLower(name)]; blocked {
			clone.Header.Del(name)
		}
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func HasForbiddenOutboundHeader(h http.Header) bool {
	for name := range h {
		if _, blocked := forbiddenOutboundHeaders[strings.ToLower(name)]; blocked {
			return true
		}
	}
	return false
}
