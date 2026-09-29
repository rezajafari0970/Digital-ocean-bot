package sanaei

import (
	"context"
	"errors"
	"testing"
)

func TestRuntimeFactoryRejectsMissingDependencies(t *testing.T) {
	_, e := (RuntimeFactory{}).Open(context.Background(), "p")
	if e == nil {
		t.Fatal("expected config error")
	}
}
func TestRetryableStatusClassification(t *testing.T) {
	for _, x := range []int{408, 425, 429, 500, 502, 503} {
		if !retryableStatus(x) {
			t.Fatalf("%d should retry", x)
		}
	}
	for _, x := range []int{400, 404, 409, 422} {
		if retryableStatus(x) {
			t.Fatalf("%d should fail fast", x)
		}
	}
}
func TestRetryableErrorCancellationIsPermanent(t *testing.T) {
	if retryableError(context.Canceled) || retryableError(context.DeadlineExceeded) {
		t.Fatal("context errors must not retry")
	}
	if retryableError(errors.New("validation failed")) {
		t.Fatal("validation must not retry")
	}
}
