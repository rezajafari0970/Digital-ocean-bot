package network

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type passiveRT func(*http.Request) (*http.Response, error)

func (f passiveRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHealthReportingTransportClassifiesProviderResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
		want   ProxyStatus
	}{
		{"ok", 200, nil, StatusHealthy},
		{"auth_is_path_success", 401, nil, StatusHealthy},
		{"permission_is_path_success", 403, nil, StatusHealthy},
		{"rate_limit_is_path_success", 429, nil, StatusHealthy},
		{"provider_5xx_is_path_success", 502, nil, StatusHealthy},
		{"proxy_auth_is_negative", 407, nil, StatusDown},
		{"network_error_is_negative", 0, errors.New("dial timeout"), StatusDown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got HealthResult
			base := passiveRT(func(*http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: http.NoBody}, nil
			})
			t0 := time.Unix(100, 0)
			t1 := t0.Add(25 * time.Millisecond)
			calls := 0
			tr := HealthReportingTransport{Base: base, Report: func(x HealthResult) { got = x }, Now: func() time.Time {
				calls++
				if calls == 1 {
					return t0
				}
				return t1
			}}
			req, _ := http.NewRequest(http.MethodGet, "https://provider.test/v2/account", nil)
			_, _ = tr.RoundTrip(req)
			if got.Status != tc.want {
				t.Fatalf("status=%s want=%s result=%+v", got.Status, tc.want, got)
			}
			if got.Latency != 25*time.Millisecond {
				t.Fatalf("latency=%s", got.Latency)
			}
		})
	}
}
