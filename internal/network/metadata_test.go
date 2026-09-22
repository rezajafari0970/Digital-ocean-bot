package network

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMetadataTransportDoesNotLeakInternalIdentity(t *testing.T) {
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Account-ID") != "" || r.Header.Get("X-Cell-ID") != "" || r.Header.Get("X-Internal-Job-ID") != "" {
			t.Fatal("internal identity leaked")
		}
		if r.Header.Get("X-Request-ID") == "" {
			t.Fatal("request id missing")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header), Request: r}, nil
	})
	req, _ := http.NewRequest("GET", "https://example.invalid", nil)
	req.Header.Set("X-Account-ID", "account-a")
	req.Header.Set("X-Cell-ID", "cell-a")
	req.Header.Set("X-Internal-Job-ID", "job-a")
	if _, err := (MetadataTransport{Base: base}).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
}
