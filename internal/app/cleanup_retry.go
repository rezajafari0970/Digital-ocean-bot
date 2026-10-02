package app

import (
	"fmt"
	"time"
)

type cleanupProviderRetryError struct {
	state string
	err   error
}

func (e cleanupProviderRetryError) Error() string {
	return fmt.Sprintf("cleanup provider %s: %v", e.state, e.err)
}
func (e cleanupProviderRetryError) Unwrap() error             { return e.err }
func (e cleanupProviderRetryError) RetryDelay() time.Duration { return 10 * time.Minute }

func slowCleanupProviderError(state string, err error) error {
	if err == nil {
		return nil
	}
	if state == ProviderStateLocked || state == ProviderStateBillingBlocked {
		return cleanupProviderRetryError{state: state, err: err}
	}
	return err
}
