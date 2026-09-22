package network

import (
	"context"
	"testing"
)

type memoryHealthStore struct {
	saved bool
	state HealthState
}

func (m *memoryHealthStore) Save(_ context.Context, _ string, s HealthState) error {
	m.saved = true
	m.state = s
	return nil
}

func TestHealthSchedulerRejectsUnhealthyProxyBeforeNetwork(t *testing.T) {
	store := &memoryHealthStore{}
	s := HealthScheduler{Store: store}
	target := HealthTarget{Proxy: Proxy{ID: "p", Type: ProxyHTTP, Host: "127.0.0.1", Port: 1, Status: StatusDown}}
	if _, err := s.Check(context.Background(), "a", target); err == nil {
		t.Fatal("down proxy must not create gateway")
	}
	if store.saved {
		t.Fatal("invalid gateway must not persist fake health")
	}
}
