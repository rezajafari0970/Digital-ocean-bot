package app

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

type stickyRTFunc func(*http.Request) (*http.Response, error)

func (f stickyRTFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGeoObserverOutageDoesNotImplyProxyFailure(t *testing.T) {
	g := &network.Gateway{Client: &http.Client{Transport: stickyRTFunc(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Host, "ipwho.is") {
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header), Request: r}, nil
		}
		if strings.Contains(r.URL.Host, "api.ipify.org") || strings.Contains(r.URL.Host, "api64.ipify.org") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{\"ip\":\"203.0.113.44\"}")), Header: make(http.Header), Request: r}, nil
		}
		return nil, errors.New("unexpected endpoint")
	})}}
	_, err := observeStickyGeoReliable(t.Context(), g)
	if !errors.Is(err, ErrProxyObservationUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
