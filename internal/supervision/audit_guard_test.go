package supervision

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
	"testing"
	"time"
)

func TestAuditSupervisedPanicReleasesTaskOwnership(t *testing.T) {
	now := time.Now()
	registry := New(func() time.Time { return now })
	ctx, err := registry.Register(context.Background(), "fixture", Policy{Loop: time.Hour, Work: time.Second, Idle: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Cancel()
	Pulse(ctx)
	err = Work(ctx, func(context.Context) error { panic("private callback value") })
	if !errors.Is(err, resilience.ErrTaskPanic) {
		t.Fatal("panic escaped the supervised task boundary")
	}
	now = now.Add(2 * time.Second)
	if err := registry.Check(); err != nil {
		t.Fatal("failed task retained supervisor ownership", err)
	}
}
