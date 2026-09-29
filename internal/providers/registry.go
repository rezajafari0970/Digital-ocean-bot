package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

func normalizeName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (r *Registry) Register(f Factory) error {
	if f == nil {
		return fmt.Errorf("provider factory is nil")
	}
	name := normalizeName(f.Name())
	if name == "" {
		return fmt.Errorf("provider factory name is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.factories[name]; ok {
		return fmt.Errorf("provider factory %q already registered", name)
	}
	r.factories[name] = f
	return nil
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[normalizeName(name)]
	return ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for name := range r.factories {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

func (r *Registry) Open(ctx context.Context, name string, req OpenRequest) (Driver, error) {
	r.mu.RLock()
	f := r.factories[normalizeName(name)]
	r.mu.RUnlock()
	if f == nil {
		return nil, fmt.Errorf("provider %q is not registered", name)
	}
	return f.Open(ctx, req)
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
