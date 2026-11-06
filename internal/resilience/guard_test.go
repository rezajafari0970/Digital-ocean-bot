package resilience

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAuditTaskGuardPreservesOutcomesAndRedactsPanic(t *testing.T) {
	sentinel := errors.New("ordinary failure")
	if err := Guard(context.Background(), func(context.Context) error { return sentinel }); err != sentinel {
		t.Fatal("ordinary error changed")
	}
	if err := Guard(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{"secret-value-that-must-never-be-stored", nil} {
		err := Guard(context.Background(), func(context.Context) error { panic(value) })
		if !errors.Is(err, ErrTaskPanic) || strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "guard_test.go") {
			t.Fatal("panic diagnosis unsafe or missing source location")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := Guard(ctx, func(context.Context) error { called = true; return nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled task entered callback")
	}
}

func TestAuditRetrierRejectsCancelledDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := (Retrier{}).Do(ctx, func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("cancelled retry dispatched %d times", calls)
	}
}
