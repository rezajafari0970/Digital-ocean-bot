package digitalocean

import (
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
	"strconv"
	"strings"
	"time"
)

type HTTPError struct {
	Status     int
	RetryAfter time.Duration
}

func (e HTTPError) Error() string { return fmt.Sprintf("digitalocean api status %d", e.Status) }
func ClassifyError(err error) resilience.ErrorClass {
	var h HTTPError
	if errors.As(err, &h) {
		if h.Status == 429 {
			return resilience.RateLimited
		}
		if h.Status == 408 || h.Status == 409 || h.Status >= 500 {
			return resilience.Retryable
		}
		if h.Status >= 400 && h.Status < 500 {
			return resilience.Permanent
		}
	}
	if errors.Is(err, ErrProviderRequest) {
		return resilience.Retryable
	}
	return resilience.Unknown
}
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	seconds, err := strconv.Atoi(v)
	if err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
