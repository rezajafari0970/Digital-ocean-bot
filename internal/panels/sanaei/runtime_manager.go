package sanaei

import (
	"context"
	"errors"
	"sync"
	"time"
)

type runtimeEntry struct {
	runtime *PanelRuntime
	opened  time.Time
	opening bool
	wait    chan struct{}
	err     error
}

type RuntimeManager struct {
	Factory RuntimeFactory
	TTL     time.Duration
	mu      sync.Mutex
	entries map[string]*runtimeEntry
}

func (m *RuntimeManager) ttl() time.Duration {
	if m.TTL <= 0 {
		return 5 * time.Second
	}
	return m.TTL
}

func (m *RuntimeManager) Acquire(ctx context.Context, panelID string) (*PanelRuntime, error) {
	if panelID == "" {
		return nil, errors.New("sanaei runtime panel id")
	}
	for {
		m.mu.Lock()
		if m.entries == nil {
			m.entries = map[string]*runtimeEntry{}
		}
		if e := m.entries[panelID]; e != nil {
			if e.opening {
				ch := e.wait
				m.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ch:
					continue
				}
			}
			if e.runtime != nil && time.Since(e.opened) < m.ttl() {
				r := e.runtime
				m.mu.Unlock()
				return r, nil
			}
		}
		e := &runtimeEntry{opening: true, wait: make(chan struct{})}
		m.entries[panelID] = e
		m.mu.Unlock()

		r, err := m.Factory.Open(ctx, panelID)

		m.mu.Lock()
		e.runtime = r
		e.err = err
		e.opened = time.Now()
		e.opening = false
		close(e.wait)
		if err != nil {
			delete(m.entries, panelID)
		}
		m.mu.Unlock()
		return r, err
	}
}

func (m *RuntimeManager) Invalidate(panelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[panelID]; e != nil && e.runtime != nil && e.runtime.Session != nil {
		e.runtime.Session.Invalidate()
	}
	delete(m.entries, panelID)
}

func (m *RuntimeManager) PurgeExpired() {
	now := time.Now()
	ttl := m.ttl()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.entries {
		if !e.opening && now.Sub(e.opened) >= ttl {
			delete(m.entries, id)
		}
	}
}

func (m *RuntimeManager) Size() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.entries) }
