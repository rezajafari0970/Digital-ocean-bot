package clientops

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDrainStopsAtGateAndFailure(t *testing.T) {
	n, err := drain(context.Background(), func(context.Context) (bool, error) { return true, nil }, 3, time.Second)
	if n != 3 || err != nil {
		t.Fatal(n, err)
	}
	calls := 0
	n, err = drain(context.Background(), func(context.Context) (bool, error) { calls++; return false, nil }, 3, time.Second)
	if n != 0 || calls != 1 || err != nil {
		t.Fatal(n, calls, err)
	}
	failure := errors.New("journal unavailable")
	calls = 0
	n, err = drain(context.Background(), func(context.Context) (bool, error) { calls++; return true, failure }, 3, time.Second)
	if n != 1 || calls != 1 || !errors.Is(err, failure) {
		t.Fatal(n, calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err = drain(ctx, func(context.Context) (bool, error) { t.Fatal("cancelled job executed"); return true, nil }, 3, time.Second)
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
}
