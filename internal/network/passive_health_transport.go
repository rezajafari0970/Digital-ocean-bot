package network

import (
	"errors"
	"net/http"
	"time"
)

// PassiveHealthReporter receives transport-level evidence from real provider
// requests. Any provider HTTP response proves that the transport path worked;
// provider/business failures (including 5xx/429/401/403) must not poison proxy
// health. A 407 is proxy-auth failure and network/TLS/dial errors are negative.
type PassiveHealthReporter func(HealthResult)

type HealthReportingTransport struct {
	Base   http.RoundTripper
	Report PassiveHealthReporter
	Now    func() time.Time
}

func (t HealthReportingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	now := t.Now
	if now == nil {
		now = time.Now
	}
	started := now()
	resp, err := base.RoundTrip(req)
	checked := now()
	if t.Report != nil {
		result := HealthResult{Status: StatusHealthy, CheckedAt: checked, Latency: checked.Sub(started)}
		switch {
		case err != nil:
			result.Status = StatusDown
			result.Error = err.Error()
		case resp != nil && resp.StatusCode == http.StatusProxyAuthRequired:
			result.Status = StatusDown
			result.Error = "proxy authentication required"
		}
		t.Report(result)
	}
	return resp, err
}

func IsPassiveTransportFailure(result HealthResult) bool {
	return result.Status == StatusDown && result.Error != ""
}

var ErrPassiveHealthUnavailable = errors.New("passive proxy health reporter unavailable")
