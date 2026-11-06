package providers

import (
	"context"
	"testing"
)

type auditFactory struct {
	name     string
	metadata Metadata
	driver   Driver
	opens    int
	crash    bool
}

func (f *auditFactory) Name() string       { return f.name }
func (f *auditFactory) Metadata() Metadata { return f.metadata }
func (f *auditFactory) Open(context.Context, OpenRequest) (Driver, error) {
	f.opens++
	if f.crash {
		panic("private credential")
	}
	return f.driver, nil
}

type auditDriver struct {
	name string
	caps Capabilities
}

func (d *auditDriver) Name() string               { return d.name }
func (d *auditDriver) Capabilities() Capabilities { return d.caps }
func (*auditDriver) Health(context.Context) error { return nil }

func auditPanicMustNotEscape(t *testing.T) {
	t.Helper()
	if recover() != nil {
		t.Fatal("plugin panic escaped its boundary")
	}
}

func TestAuditRegistryRejectsInvalidDriverContracts(t *testing.T) {
	var typedNil *auditDriver
	for _, tc := range []struct {
		name   string
		driver Driver
		crash  bool
	}{
		{"nil", nil, false}, {"typed_nil", typedNil, false},
		{"wrong_identity", &auditDriver{name: "other"}, false},
		{"missing_account", &auditDriver{name: "fixture", caps: Capabilities{Account: true}}, false},
		{"missing_catalog", &auditDriver{name: "fixture", caps: Capabilities{Catalog: true}}, false},
		{"missing_compute", &auditDriver{name: "fixture", caps: Capabilities{Compute: true}}, false},
		{"missing_ssh", &auditDriver{name: "fixture", caps: Capabilities{SSHKeys: true}}, false},
		{"missing_inventory", &auditDriver{name: "fixture", caps: Capabilities{Inventory: true}}, false},
		{"panic", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer auditPanicMustNotEscape(t)
			r := NewRegistry()
			f := &auditFactory{name: "fixture", metadata: Metadata{Name: "fixture"}, driver: tc.driver, crash: tc.crash}
			if err := r.Register(f); err != nil {
				t.Fatal(err)
			}
			d, err := r.Open(context.Background(), "fixture", OpenRequest{})
			if err == nil || d != nil {
				t.Fatal("invalid plugin returned an accepted driver")
			}
		})
	}
}

func TestAuditRegistryRegistrationAndMetadataIsolation(t *testing.T) {
	t.Run("typed_nil", func(t *testing.T) {
		defer auditPanicMustNotEscape(t)
		var f *auditFactory
		if NewRegistry().Register(f) == nil {
			t.Fatal("typed nil factory accepted")
		}
	})
	t.Run("zero_value", func(t *testing.T) {
		defer auditPanicMustNotEscape(t)
		var r Registry
		if err := r.Register(&auditFactory{name: "fixture", metadata: Metadata{Name: "fixture"}}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("metadata_identity", func(t *testing.T) {
		if NewRegistry().Register(&auditFactory{name: "fixture", metadata: Metadata{Name: "other"}}) == nil {
			t.Fatal("conflicting metadata identity accepted")
		}
	})
	t.Run("snapshot", func(t *testing.T) {
		r := NewRegistry()
		f := &auditFactory{name: "fixture", metadata: Metadata{Name: "fixture", CredentialFields: []CredentialField{{Key: "token"}}, Defaults: Defaults{Regions: []string{"region"}, Plans: []string{"plan"}, Images: ImagePolicy{Versions: []string{"version"}}}}}
		if err := r.Register(f); err != nil {
			t.Fatal(err)
		}
		f.metadata.Defaults.Regions[0] = "factory mutation"
		m, _ := r.Metadata("fixture")
		m.Defaults.Plans[0] = "consumer mutation"
		m.Defaults.Images.Versions[0] = "consumer mutation"
		m.CredentialFields[0].Key = "consumer mutation"
		n, _ := r.Metadata("fixture")
		if n.Defaults.Regions[0] != "region" || n.Defaults.Plans[0] != "plan" || n.Defaults.Images.Versions[0] != "version" || n.CredentialFields[0].Key != "token" {
			t.Fatal("plugin metadata mutated across consumers")
		}
	})
}

func TestAuditRegistryCancelledOpenDoesNotCallPlugin(t *testing.T) {
	r := NewRegistry()
	f := &auditFactory{name: "fixture", metadata: Metadata{Name: "fixture"}, driver: &auditDriver{name: "fixture"}}
	if err := r.Register(f); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d, err := r.Open(ctx, "fixture", OpenRequest{}); err == nil || d != nil || f.opens != 0 {
		t.Fatal("cancelled open invoked plugin")
	}
	if d, err := r.Discover(ctx, "fixture", OpenRequest{}, nil); err == nil || d.Complete {
		t.Fatal("cancelled discovery accepted as complete")
	}
}
