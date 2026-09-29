package workflow

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"testing"
	"time"
)

type waiterCompute struct {
	calls  int
	server providers.Server
	err    error
}

func (w *waiterCompute) CreateServer(context.Context, providers.CreateServerRequest) (providers.CreateServerResult, error) {
	return providers.CreateServerResult{}, nil
}
func (w *waiterCompute) GetServer(context.Context, string) (providers.Server, error) {
	w.calls++
	return w.server, w.err
}
func (w *waiterCompute) ListServers(context.Context) ([]providers.Server, error) { return nil, nil }
func (w *waiterCompute) DeleteServer(context.Context, string) error              { return nil }
func (w *waiterCompute) FindServerByIdentity(context.Context, string) ([]providers.Server, error) {
	return nil, nil
}
func TestServerWaiterReady(t *testing.T) {
	p := &waiterCompute{server: providers.Server{ID: "abc", Ready: true, PrimaryIPv4: "203.0.113.9"}}
	got, err := (ServerWaiter{Provider: p, Timeout: time.Second}).Wait(context.Background(), "abc")
	if err != nil || got.ProviderID != "abc" || got.Host != "203.0.113.9" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
func TestServerWaiterPermanentError(t *testing.T) {
	p := &waiterCompute{err: &providers.Error{Class: providers.ErrorAuthentication}}
	_, err := (ServerWaiter{Provider: p, Timeout: time.Second}).Wait(context.Background(), "abc")
	if providers.Class(err) != providers.ErrorAuthentication {
		t.Fatalf("err=%v", err)
	}
	if p.calls != 1 {
		t.Fatalf("calls=%d", p.calls)
	}
}
