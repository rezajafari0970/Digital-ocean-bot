package vultr

import (
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestAccountStateHTTPClassification(t *testing.T) {
	tests := []struct {
		status int
		msg    string
		want   providers.ErrorClass
	}{
		{401, "invalid api key", providers.ErrorAuthentication},
		{403, "account is locked", providers.ErrorAccountLocked},
		{403, "account suspended", providers.ErrorAccountLocked},
		{403, "forbidden", providers.ErrorPermissionDenied},
		{403, "outstanding balance on billing profile", providers.ErrorPermissionDenied},
		{429, "rate limit exceeded", providers.ErrorRateLimited},
		{503, "temporarily unavailable", providers.ErrorTransport},
	}
	for _, tc := range tests {
		err := normalizeError("health", HTTPError{Status: tc.status, Message: tc.msg})
		if got := providers.Class(err); got != tc.want {
			t.Fatalf("status=%d msg=%q got=%s want=%s", tc.status, tc.msg, got, tc.want)
		}
	}
}
