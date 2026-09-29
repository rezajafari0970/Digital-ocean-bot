package providers

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
)

type testDriver struct{}

func (testDriver) Name() string                 { return "test" }
func (testDriver) Capabilities() Capabilities   { return Capabilities{Account: true, Compute: true} }
func (testDriver) Health(context.Context) error { return nil }

type testFactory struct {
	name  string
	opens int
	mu    sync.Mutex
}

func (f *testFactory) Name() string { return f.name }
func (f *testFactory) Metadata() Metadata {
	return Metadata{Name: normalizeName(f.name), DisplayName: f.name}
}
func (f *testFactory) Open(context.Context, OpenRequest) (Driver, error) {
	f.mu.Lock()
	f.opens++
	f.mu.Unlock()
	return testDriver{}, nil
}

func TestRegistryNormalizesAndOpens(t *testing.T) {
	r := NewRegistry()
	f := &testFactory{name: " DigitalOcean "}
	if err := r.Register(f); err != nil {
		t.Fatal(err)
	}
	if !r.Has("digitalocean") || !r.Has(" DIGITALOCEAN ") {
		t.Fatal("normalized lookup failed")
	}
	d, err := r.Open(context.Background(), "DIGITALOCEAN", OpenRequest{AccountID: "a", HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "test" {
		t.Fatalf("driver=%s", d.Name())
	}
	if f.opens != 1 {
		t.Fatalf("opens=%d", f.opens)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&testFactory{name: "vultr"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&testFactory{name: " VULTR "}); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestRegistryNamesSorted(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"vultr", "digitalocean", "hetzner"} {
		if err := r.Register(&testFactory{name: n}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.Names()
	want := []string{"digitalocean", "hetzner", "vultr"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names=%v", got)
		}
	}
}

func TestProviderErrorTaxonomy(t *testing.T) {
	root := errors.New("connection reset")
	err := &Error{Class: ErrorTransport, Operation: "catalog", Cause: root}
	if Class(err) != ErrorTransport {
		t.Fatalf("class=%s", Class(err))
	}
	if !IsClass(err, ErrorTransport) || !IsRetryable(err) {
		t.Fatal("transport classification failed")
	}
	if !errors.Is(err, root) {
		t.Fatal("unwrap failed")
	}
	if IsRetryable(&Error{Class: ErrorAuthentication}) {
		t.Fatal("authentication must not be retryable")
	}
	if Class(errors.New("plain")) != ErrorUnknown {
		t.Fatal("plain errors must be unknown")
	}
}

func TestCapabilityInterfacesRemainIndependent(t *testing.T) {
	c := Capabilities{Compute: true}
	if !c.Compute || c.Catalog || c.SSHKeys || c.Inventory || c.Account {
		t.Fatalf("capabilities=%+v", c)
	}
}
