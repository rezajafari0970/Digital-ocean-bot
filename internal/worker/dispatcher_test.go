package worker

import (
	"context"
	"testing"
	"time"
)

func TestDispatcherSlowAccountDoesNotBlockOthers(t *testing.T) {
	d := NewDispatcher(3, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := make(chan struct{})
	done := make(chan struct{})
	if !d.Submit(ctx, "a1", "a", func() { <-slow }) {
		t.Fatal("first")
	}
	if d.Submit(ctx, "a2", "a", func() { t.Error("per-account bound bypassed") }) {
		t.Fatal("bound")
	}
	if d.Submit(ctx, "a1", "b", func() { t.Error("duplicate ran") }) {
		t.Fatal("identity")
	}
	if !d.Submit(ctx, "b1", "b", func() { close(done) }) {
		t.Fatal("independent account blocked")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("head-of-line blocking")
	}
	close(slow)
	d.Wait()
	cancel()
	if d.Submit(ctx, "c1", "c", func() { t.Error("ran after shutdown") }) {
		t.Fatal("shutdown")
	}
}
