package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type fakeRunLease struct {
	mu   sync.Mutex
	held bool
}

func (l *fakeRunLease) Acquire(context.Context, string) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return nil, ErrDeploymentBusy
	}
	l.held = true
	return func() {
		l.mu.Lock()
		l.held = false
		l.mu.Unlock()
	}, nil
}

func TestRunLeaseRejectsConcurrentOwner(t *testing.T) {
	l := &fakeRunLease{}
	release, err := l.Acquire(context.Background(), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Acquire(context.Background(), "d1"); !errors.Is(err, ErrDeploymentBusy) {
		t.Fatalf("got %v want ErrDeploymentBusy", err)
	}
	release()
	if release2, err := l.Acquire(context.Background(), "d1"); err != nil {
		t.Fatal(err)
	} else {
		release2()
	}
}
