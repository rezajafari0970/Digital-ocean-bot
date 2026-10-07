package sanaei

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrRuntimeBusy = errors.New("sanaei runtime busy before mutation")

var ErrRuntimeCircuitOpen = errors.New("sanaei runtime circuit open")

type runtimeEntry struct {
	runtime *PanelRuntime
	opened  time.Time
	opening bool
	wait    chan struct{}
	err     error
}

type circuitState struct {
	failures int
	until    time.Time
}

type RuntimeManager struct {
	Factory RuntimeFactory
	TTL     time.Duration

	mu            sync.Mutex
	entries       map[string]*runtimeEntry
	circuits      map[string]circuitState
	mutationGates map[string]*sync.Mutex
}

func (m *RuntimeManager) ttl() time.Duration {
	if m.TTL <= 0 {
		return 5 * time.Second
	}
	return m.TTL
}
func circuitDelay(failures int) time.Duration {
	switch {
	case failures >= 5:
		return 2 * time.Minute
	case failures >= 3:
		return 30 * time.Second
	default:
		return 0
	}
}
func (m *RuntimeManager) observe(panelID string, success, transient bool) {
	if panelID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if success {
		delete(m.circuits, panelID)
		return
	}
	if !transient {
		return
	}
	if m.circuits == nil {
		m.circuits = map[string]circuitState{}
	}
	c := m.circuits[panelID]
	c.failures++
	if delay := circuitDelay(c.failures); delay > 0 {
		c.until = time.Now().Add(delay)
	}
	m.circuits[panelID] = c
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
		if m.circuits == nil {
			m.circuits = map[string]circuitState{}
		}
		if c := m.circuits[panelID]; !c.until.IsZero() && time.Now().Before(c.until) {
			remain := time.Until(c.until).Round(time.Second)
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: retry in %s", ErrRuntimeCircuitOpen, remain)
		}
		if e := m.entries[panelID]; e != nil {
			if e.opening {
				ch := e.wait
				m.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("%w: %w", ErrRuntimeBusy, ctx.Err())
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
		if m.mutationGates == nil {
			m.mutationGates = map[string]*sync.Mutex{}
		}
		gate := m.mutationGates[panelID]
		if gate == nil {
			gate = &sync.Mutex{}
			m.mutationGates[panelID] = gate
		}
		if r != nil {
			r.mutationMu = gate
			if r.Session != nil && r.Session.Exec != nil {
				r.Session.Exec.Observe = func(success, transient bool) { m.observe(panelID, success, transient) }
			}
		}
		e.runtime = r
		e.err = err
		e.opened = time.Now()
		e.opening = false
		close(e.wait)
		if err != nil {
			delete(m.entries, panelID)
			// SQL, secret-store and local invariant failures must never
			// acquire panel-transport provenance through the circuit breaker.
			if errors.Is(err, ErrAPIRequest) {
				c := m.circuits[panelID]
				c.failures++
				if delay := circuitDelay(c.failures); delay > 0 {
					c.until = time.Now().Add(delay)
				}
				m.circuits[panelID] = c
			}
		} else {
			delete(m.circuits, panelID)
		}
		m.mu.Unlock()
		return r, err
	}
}
func (m *RuntimeManager) ResetCircuit(panelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.circuits, panelID)
}

func (m *RuntimeManager) Invalidate(panelID string) {
	var session *PanelSession
	m.mu.Lock()
	if e := m.entries[panelID]; e != nil && e.runtime != nil {
		session = e.runtime.Session
	}
	delete(m.entries, panelID)
	m.mu.Unlock()
	if session != nil {
		session.Invalidate()
	}
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
	for id, c := range m.circuits {
		if !c.until.IsZero() && now.After(c.until.Add(10*time.Minute)) {
			delete(m.circuits, id)
		}
	}
}
func (m *RuntimeManager) Size() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.entries) }
