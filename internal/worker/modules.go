package worker

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"sort"
	"sync"
	"time"
)

type Role string

const (
	RoleAll     Role = "all"
	RoleControl Role = "control"
	RolePanels  Role = "panels"
)

func ParseRole(value string) (Role, error) {
	switch Role(value) {
	case RoleAll, RoleControl, RolePanels:
		return Role(value), nil
	default:
		return "", fmt.Errorf("unknown worker role %q", value)
	}
}

// The two split processes share the old total64 budget; API retains24.
func (r Role) ConnectionBudget() int {
	switch r {
	case RoleControl:
		return 24
	case RolePanels:
		return 40
	default:
		return 64
	}
}
func (r Role) HeartbeatKind() string {
	if r == RoleAll {
		return "production"
	}
	return "production-" + string(r)
}
func (r Role) Owns(group Role) bool { return r == RoleAll || group == RoleAll || r == group }

type Module struct {
	Name string
	Role Role
	Run  func(context.Context)
}
type Modules struct {
	modules    []Module
	Supervisor *supervision.Registry
	Policies   map[string]supervision.Policy
	Grace      time.Duration
	Tick       func(supervision.Snapshot) error
	Ready      func() error
	Failure    <-chan error
}

func (m *Modules) Add(role Role, name string, run func(context.Context)) {
	// Programming errors are startup failures, never a silently missing module.
	if _, err := ParseRole(string(role)); err != nil {
		panic(err)
	}
	if name == "" || run == nil {
		panic("invalid worker module")
	}
	for _, v := range m.modules {
		if v.Name == name {
			panic("duplicate worker module: " + name)
		}
	}
	m.modules = append(m.modules, Module{Name: name, Role: role, Run: run})
}
func (m *Modules) Names(role Role) []string {
	names := []string{}
	for _, v := range m.modules {
		if role.Owns(v.Role) {
			names = append(names, v.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Run never restarts a module in-process: remote work may have an unknown
// outcome. An unexpected return exits the role process; durable reconciliation
// and systemd then supervise a new process only after the old one has exited.
// Normal shutdown joins every selected module before resources are closed.
func (m *Modules) Run(ctx context.Context, role Role) error {
	if _, err := ParseRole(string(role)); err != nil {
		return err
	}
	if len(m.Names(role)) == 0 {
		return fmt.Errorf("no modules for role %s", role)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if m.Supervisor != nil {
		defer m.Supervisor.Cancel()
	}
	type invocation struct {
		mod Module
		ctx context.Context
	}
	selected := []invocation{}
	for _, mod := range m.modules {
		if !role.Owns(mod.Role) {
			continue
		}
		c := runCtx
		if m.Supervisor != nil {
			policy, ok := m.Policies[mod.Name]
			if !ok {
				return fmt.Errorf("missing supervision policy %s", mod.Name)
			}
			var err error
			c, err = m.Supervisor.Register(c, mod.Name, policy)
			if err != nil {
				return err
			}
		}
		selected = append(selected, invocation{mod, c})
	}
	var wg sync.WaitGroup
	stopped := make(chan string, 1)
	for _, v := range selected {
		wg.Add(1)
		go func(v invocation) {
			defer wg.Done()
			v.mod.Run(v.ctx)
			if runCtx.Err() == nil {
				select {
				case stopped <- v.mod.Name:
				default:
				}
			}
		}(v)
	}
	joined := make(chan struct{})
	go func() { wg.Wait(); close(joined) }()
	fail := func(err error) error {
		cancel()
		if m.Supervisor != nil {
			m.Supervisor.Cancel()
			grace := m.Grace
			if grace <= 0 {
				grace = 15 * time.Second
			}
			timer := time.NewTimer(grace)
			defer timer.Stop()
			select {
			case <-joined:
			case <-timer.C:
			}
		}
		return err
	}
	ready := false
	if m.Supervisor == nil {
		if m.Ready != nil {
			if err := m.Ready(); err != nil {
				return fail(err)
			}
		}
		ready = true
	}
	var ticks <-chan time.Time
	var ticker *time.Ticker
	if m.Supervisor != nil {
		ticker = time.NewTicker(time.Second)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case err := <-m.Failure:
			if err == nil {
				err = fmt.Errorf("worker ownership monitor stopped")
			}
			return fail(err)
		case name := <-stopped:
			return fail(fmt.Errorf("worker module %s stopped unexpectedly", name))
		case <-ticks:
			if err := m.Supervisor.Check(); err != nil {
				return fail(err)
			}
			snapshot := m.Supervisor.Snapshot()
			if !ready && snapshot.Healthy {
				select {
				case name := <-stopped:
					return fail(fmt.Errorf("worker module %s stopped during startup", name))
				default:
				}
				select {
				case err := <-m.Failure:
					if err == nil {
						err = fmt.Errorf("worker ownership monitor stopped")
					}
					return fail(err)
				default:
				}
				if m.Ready != nil {
					if err := m.Ready(); err != nil {
						return fail(err)
					}
				}
				ready = true
			}
			if ready && m.Tick != nil {
				if err := m.Tick(snapshot); err != nil {
					return fail(err)
				}
			}
		case <-ctx.Done():
			cancel()
			if m.Supervisor == nil {
				<-joined
				return ctx.Err()
			}
			timer := time.NewTimer(75 * time.Second)
			defer timer.Stop()
			select {
			case <-joined:
				return ctx.Err()
			case <-timer.C:
				return fmt.Errorf("%w: shutdown did not join", supervision.ErrStalled)
			}
		}
	}
}
