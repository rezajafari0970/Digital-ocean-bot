package providers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

var ErrDriverContract = errors.New("provider driver contract invalid")

type registeredFactory struct {
	factory      Factory
	metadata     Metadata
	requirements RuntimeRequirements
}
type Registry struct {
	mu        sync.RWMutex
	factories map[string]registeredFactory
}

func NewRegistry() *Registry        { return &Registry{factories: map[string]registeredFactory{}} }
func normalizeName(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func nilPlugin(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
func cloneMetadata(m Metadata) Metadata {
	m.CredentialFields = append([]CredentialField(nil), m.CredentialFields...)
	m.Defaults.Regions = append([]string(nil), m.Defaults.Regions...)
	m.Defaults.Plans = append([]string(nil), m.Defaults.Plans...)
	m.Defaults.Images.Versions = append([]string(nil), m.Defaults.Images.Versions...)
	return m
}
func (r *Registry) Register(f Factory) (err error) {
	// Do not print a plugin panic value: it may contain credentials.
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: factory registration failed", ErrDriverContract)
		}
	}()
	if r == nil || nilPlugin(f) {
		return fmt.Errorf("%w: factory missing", ErrDriverContract)
	}
	name := normalizeName(f.Name())
	if name == "" {
		return fmt.Errorf("%w: factory name empty", ErrDriverContract)
	}
	metadata := cloneMetadata(f.Metadata())
	if metadata.Name != "" && normalizeName(metadata.Name) != name {
		return fmt.Errorf("%w: metadata identity mismatch", ErrDriverContract)
	}
	metadata.Name = name
	entry := registeredFactory{factory: f, metadata: metadata}
	if f, ok := f.(RuntimeRequirementProvider); ok {
		entry.requirements = f.RuntimeRequirements()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[string]registeredFactory)
	}
	if _, ok := r.factories[name]; ok {
		return fmt.Errorf("provider factory %q already registered", name)
	}
	r.factories[name] = entry
	return nil
}
func (r *Registry) lookup(name string) (registeredFactory, bool) {
	if r == nil {
		return registeredFactory{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[normalizeName(name)]
	return f, ok
}
func (r *Registry) Has(name string) bool { _, ok := r.lookup(name); return ok }
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for name := range r.factories {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
func (r *Registry) Metadata(name string) (Metadata, bool) {
	f, ok := r.lookup(name)
	return cloneMetadata(f.metadata), ok
}
func (r *Registry) MetadataAll() []Metadata {
	names := r.Names()
	out := make([]Metadata, 0, len(names))
	for _, name := range names {
		if m, ok := r.Metadata(name); ok {
			out = append(out, m)
		}
	}
	return out
}
func (r *Registry) Requirements(name string) (RuntimeRequirements, bool) {
	f, ok := r.lookup(name)
	return f.requirements, ok
}
func (r *Registry) Open(ctx context.Context, name string, req OpenRequest) (driver Driver, err error) {
	defer func() {
		if recover() != nil {
			driver = nil
			err = fmt.Errorf("%w: factory open failed", ErrDriverContract)
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	f, ok := r.lookup(name)
	if !ok {
		return nil, fmt.Errorf("provider %q is not registered", name)
	}
	driver, err = f.factory.Open(ctx, req)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if nilPlugin(driver) || normalizeName(driver.Name()) != normalizeName(name) {
		return nil, fmt.Errorf("%w: driver identity mismatch", ErrDriverContract)
	}
	cap := driver.Capabilities()
	_, account := driver.(AccountReader)
	_, catalog := driver.(CatalogReader)
	_, compute := driver.(ComputeDriver)
	_, ssh := driver.(SSHKeyDriver)
	_, inventory := driver.(InventoryReader)
	if cap.Account && !account || cap.Catalog && !catalog || cap.Compute && !compute || cap.SSHKeys && !ssh || cap.Inventory && !inventory || cap.InlineSSHKeys && !compute {
		return nil, fmt.Errorf("%w: declared capability missing", ErrDriverContract)
	}
	return driver, nil
}
