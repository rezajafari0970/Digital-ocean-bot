package network

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransportObserverTreatsHTTPResponsesAsPathSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer s.Close()
	var got TransportObservation
	c := s.Client()
	c.Transport = ObserveTransport(c.Transport, func(x TransportObservation) { got = x })
	resp, err := c.Get(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Err != nil {
		t.Fatalf("HTTP response must be transport success: %v", got.Err)
	}
	if got.Latency <= 0 {
		t.Fatal("latency not observed")
	}
}

type failingRoundTripper struct{ err error }

func (f failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestTransportObserverReportsRoundTripFailure(t *testing.T) {
	root := errors.New("dial timeout")
	var got TransportObservation
	rt := ObserveTransport(failingRoundTripper{err: root}, func(x TransportObservation) { got = x })
	req, _ := http.NewRequest(http.MethodGet, "https://provider.invalid", nil)
	_, err := rt.RoundTrip(req)
	if !errors.Is(err, root) || !errors.Is(got.Err, root) {
		t.Fatalf("err=%v observed=%v", err, got.Err)
	}
}
