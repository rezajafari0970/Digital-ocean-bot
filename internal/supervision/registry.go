// Package supervision watches real loop, task and stage progress without SQL.
package supervision

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrStalled = errors.New("worker progress deadline exceeded")
var ErrProtocol = errors.New("invalid supervision contract")

type Policy struct {
	Loop, Work, Idle time.Duration
	// AsyncLoop has an independent discovery loop; child activity cannot renew it.
	AsyncLoop bool
}

func (p Policy) Valid() bool {
	return p.Loop > 0 && p.Work > 0 && p.Idle >= 0 && p.Loop <= 24*time.Hour && p.Work <= 24*time.Hour && p.Idle <= 24*time.Hour
}

type node struct {
	id, parent uint64
	phase      string
	budget     time.Duration
	deadline   time.Time
	children   int
}
type module struct {
	started   bool
	name      string
	policy    Policy
	cancel    context.CancelFunc
	due       time.Time
	idle      bool
	waiting   int
	nodes     map[uint64]*node
	completed uint64
	fault     string
}
type Registry struct {
	mu       sync.Mutex
	now      func() time.Time
	last     time.Time
	sequence uint64
	modules  map[string]*module
}
type binding struct {
	r      *Registry
	name   string
	parent uint64
}
type contextKey struct{}

func New(now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{now: now, modules: map[string]*module{}}
}
func (r *Registry) Register(ctx context.Context, name string, p Policy) (context.Context, error) {
	if name == "" || !p.Valid() {
		return nil, ErrProtocol
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.modules[name]; ok {
		return nil, ErrProtocol
	}
	c, cancel := context.WithCancel(ctx)
	r.modules[name] = &module{name: name, policy: p, cancel: cancel, due: r.now().Add(p.Loop), nodes: map[uint64]*node{}}
	return context.WithValue(c, contextKey{}, binding{r: r, name: name}), nil
}
func (r *Registry) asyncLoop(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.modules[name].policy.AsyncLoop
}
func lookup(ctx context.Context) (binding, bool) {
	b, ok := ctx.Value(contextKey{}).(binding)
	return b, ok
}
func Pulse(ctx context.Context) {
	b, ok := lookup(ctx)
	if !ok || (b.parent != 0 && b.r.asyncLoop(b.name)) {
		return
	}
	b.r.mu.Lock()
	defer b.r.mu.Unlock()
	m := b.r.modules[b.name]
	m.started = true
	m.due = b.r.now().Add(m.policy.Loop)
	m.idle = false
}
func Idle(ctx context.Context) {
	b, ok := lookup(ctx)
	if !ok || (b.parent != 0 && b.r.asyncLoop(b.name)) {
		return
	}
	b.r.mu.Lock()
	defer b.r.mu.Unlock()
	m := b.r.modules[b.name]
	m.started = true
	m.due = b.r.now().Add(m.policy.Idle + m.policy.Loop)
	m.idle = true
}

// Waiting is visible, but no execution budget is charged before admission.
func Waiting(ctx context.Context) func() {
	b, ok := lookup(ctx)
	if !ok {
		return func() {}
	}
	b.r.mu.Lock()
	m := b.r.modules[b.name]
	m.started = true
	m.waiting++
	if p := m.nodes[b.parent]; p != nil {
		p.children++
	}
	b.r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.r.mu.Lock()
			defer b.r.mu.Unlock()
			m := b.r.modules[b.name]
			m.waiting--
			if p := m.nodes[b.parent]; p != nil {
				p.children--
				if p.children == 0 {
					p.deadline = b.r.now().Add(p.budget)
				}
			}
			m.started = true
			if !m.policy.AsyncLoop {
				m.due = b.r.now().Add(m.policy.Loop)
			}
		})
	}
}

// Begin creates an independently checked child. Only its direct ancestor's
// deadline is suspended. Siblings cannot hide each other. No goroutine is
// abandoned or restarted by this package.
func Begin(ctx context.Context, phase string, budget time.Duration) (context.Context, func(), error) {
	b, ok := lookup(ctx)
	if !ok {
		return ctx, func() {}, nil
	}
	if err := ctx.Err(); err != nil {
		return ctx, func() {}, err
	}
	if phase != "work" && phase != "precheck" && phase != "execute" && phase != "verify" && phase != "create" && phase != "wait_resource" && phase != "provision" && phase != "database" && phase != "panel" && phase != "clients" && phase != "traffic" {
		return ctx, func() {}, ErrProtocol
	}
	r := b.r
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.modules[b.name]
	if budget == 0 {
		budget = m.policy.Work
	}
	if budget <= 0 || budget > 24*time.Hour || len(m.nodes) >= 1024 {
		return ctx, func() {}, ErrProtocol
	}
	if b.parent != 0 {
		p, ok := m.nodes[b.parent]
		if !ok {
			return ctx, func() {}, ErrProtocol
		}
		p.children++
	}
	r.sequence++
	id := r.sequence
	m.started = true
	m.nodes[id] = &node{id: id, parent: b.parent, phase: phase, budget: budget, deadline: r.now().Add(budget)}
	m.idle = false
	if !m.policy.AsyncLoop {
		m.due = r.now().Add(m.policy.Loop)
	}
	var once sync.Once
	finish := func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			n := m.nodes[id]
			if n == nil || n.children != 0 {
				m.fault = "stage_order"
				m.cancel()
				return
			}
			delete(m.nodes, id)
			m.completed++
			if !m.policy.AsyncLoop {
				m.due = r.now().Add(m.policy.Loop)
			}
			if n.parent != 0 {
				if p := m.nodes[n.parent]; p != nil {
					p.children--
					if p.children == 0 {
						p.deadline = r.now().Add(p.budget)
					}
				} else {
					m.fault = "stage_parent"
					m.cancel()
				}
			}
		})
	}
	return context.WithValue(ctx, contextKey{}, binding{r: r, name: b.name, parent: id}), finish, nil
}
func Work(ctx context.Context, fn func(context.Context) error) error {
	c, done, err := Begin(ctx, "work", 0)
	if err != nil {
		return err
	}
	defer done()
	return fn(c)
}
func reason(m *module, now time.Time) string {
	if m.fault != "" {
		return m.fault
	}
	for _, n := range m.nodes {
		if n.children == 0 && !now.Before(n.deadline) {
			return "task_" + n.phase
		}
	}
	if (m.policy.AsyncLoop || (len(m.nodes) == 0 && m.waiting == 0)) && !now.Before(m.due) {
		return "loop"
	}
	return ""
}

// Check latches a fault and cancels its module context. The process owner must
// bounded-join and exit; retry is exclusively delegated to a new OS process.
func (r *Registry) Check() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	regressed := !r.last.IsZero() && now.Before(r.last)
	r.last = now
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		m := r.modules[name]
		why := reason(m, now)
		if regressed {
			why = "clock_regression"
		}
		if why != "" {
			m.fault = why
			m.cancel()
			return fmt.Errorf("%w: module=%s phase=%s", ErrStalled, name, why)
		}
	}
	return nil
}

type ModuleSnapshot struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Phase     string `json:"phase,omitempty"`
	Active    int    `json:"active"`
	Waiting   int    `json:"waiting"`
	Completed uint64 `json:"completed"`
}
type Snapshot struct {
	Version int              `json:"version"`
	Healthy bool             `json:"healthy"`
	Modules []ModuleSnapshot `json:"modules"`
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Snapshot{Version: 1, Healthy: len(r.modules) > 0, Modules: []ModuleSnapshot{}}
	now := r.now()
	for _, m := range r.modules {
		x := ModuleSnapshot{Name: m.name, State: "RUNNING", Active: len(m.nodes), Waiting: m.waiting, Completed: m.completed}
		if !m.started {
			x.State = "STARTING"
			out.Healthy = false
		}
		if m.idle {
			x.State = "IDLE"
		}
		if m.waiting > 0 && len(m.nodes) == 0 {
			x.State = "WAITING"
		}
		if why := reason(m, now); why != "" {
			x.State = "STALLED"
			x.Phase = why
			out.Healthy = false
		}
		out.Modules = append(out.Modules, x)
	}
	sort.Slice(out.Modules, func(i, j int) bool { return out.Modules[i].Name < out.Modules[j].Name })
	return out
}
func (r *Registry) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range r.modules {
		m.cancel()
	}
}
