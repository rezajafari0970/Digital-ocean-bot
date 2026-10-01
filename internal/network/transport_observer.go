package network

import (
	"net/http"
	"time"
)

type TransportObservation struct {
	StartedAt         time.Time
	Latency           time.Duration
	Err               error
	ProxyAuthRequired bool
}

type TransportObserver func(TransportObservation)

type observingRoundTripper struct {
	base    http.RoundTripper
	observe TransportObserver
}

func (o observingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	resp, err := o.base.RoundTrip(req)
	if o.observe != nil {
		o.observe(TransportObservation{StartedAt: started, Latency: time.Since(started), Err: err, ProxyAuthRequired: resp != nil && resp.StatusCode == http.StatusProxyAuthRequired})
	}
	return resp, err
}

func ObserveTransport(base http.RoundTripper, observe TransportObserver) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if observe == nil {
		return base
	}
	return observingRoundTripper{base: base, observe: observe}
}
