package digitalocean

import (
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

func TestCapacityClassification(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		capacity bool
		class    resilience.ErrorClass
	}{
		{"region unavailable", HTTPError{Status: 422, Code: "unprocessable_entity", Message: "region is not available"}, true, resilience.Permanent},
		{"size unavailable", HTTPError{Status: 422, Message: "size is unavailable in this region"}, true, resilience.Permanent},
		{"unauthorized", HTTPError{Status: 401, Message: "Unable to authenticate you"}, false, resilience.Permanent},
		{"rate limited", HTTPError{Status: 429}, false, resilience.RateLimited},
		{"server error", HTTPError{Status: 503}, false, resilience.Retryable},
		{"transport", errors.Join(ErrProviderRequest, errors.New("timeout")), false, resilience.Retryable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsCapacityError(tc.err); got != tc.capacity {
				t.Fatalf("capacity=%v want %v", got, tc.capacity)
			}
			if got := ClassifyError(tc.err); got != tc.class {
				t.Fatalf("class=%v want %v", got, tc.class)
			}
		})
	}
}
