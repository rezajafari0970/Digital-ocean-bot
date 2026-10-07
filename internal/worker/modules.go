package worker

import (
	"context"
	"fmt"
	"sort"
	"sync"
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
type Modules struct{ modules []Module }

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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	stopped := make(chan string, 1)
	for _, mod := range m.modules {
		if !role.Owns(mod.Role) {
			continue
		}
		wg.Add(1)
		go func(mod Module) {
			defer wg.Done()
			// Panics deliberately terminate this process; continuing after an unknown
			// invariant failure would manufacture recovery without durable proof.
			mod.Run(ctx)
			if ctx.Err() == nil {
				select {
				case stopped <- mod.Name:
				default:
				}
			}
		}(mod)
	}
	select {
	case name := <-stopped:
		return fmt.Errorf("worker module %s stopped unexpectedly", name)
	case <-ctx.Done():
		wg.Wait()
		return ctx.Err()
	}
}
