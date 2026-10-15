package ipam

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ipwire"
	"testing"
)

func TestWireBindingRejectsEveryUnapprovedField(t *testing.T) {
	spec := ipwire.Spec{Provider: "digitalocean", Family: 4, Product: "reserved", Action: "ALLOCATE", ServerID: "123", Region: "nyc3"}
	prepared, e := ipwire.Prepare(spec)
	if e != nil {
		t.Fatal(e)
	}
	w := WireRequest{Spec: spec, Digest: prepared.Digest()}
	a := Assignment{ServerID: "123", Region: "nyc3", NetworkImpact: prepared.Impact()}
	p := Plan{Input: PlanInput{Policy: Policy{Family: 4, Product: "reserved"}, WireRequests: map[string]WireRequest{"123": w}}, Assignments: []Assignment{a}}
	v := ExecutionEvidence{WireRequest: &w, Provider: "digitalocean"}
	if _, e := boundWire(p, a, v, true); e != nil {
		t.Fatal(e)
	}
	changes := []func(*WireRequest){func(x *WireRequest) { x.Digest = "bad" }, func(x *WireRequest) { x.Spec.ServerID = "124" }, func(x *WireRequest) { x.Spec.Region = "ams3" }, func(x *WireRequest) { x.Spec.Product = "public" }, func(x *WireRequest) { x.Spec.Action = "RELEASE" }, func(x *WireRequest) { x.Spec.Family = 6 }, func(x *WireRequest) { x.Spec.InterfaceMode = "legacy" }, func(x *WireRequest) { x.Spec.ReleasePolicy = "keep" }, func(x *WireRequest) { x.Spec.PrefixLength = 64 }, func(x *WireRequest) { x.Spec.Provider = "linode" }}
	for i, change := range changes {
		bad := w
		change(&bad)
		copy := p
		copy.Input.WireRequests = map[string]WireRequest{"123": bad}
		if _, e := boundWire(copy, a, v, true); e == nil {
			t.Fatal("mutation accepted", i)
		}
	}
	p.Input.WireRequests["extra"] = w
	if validateWireKeys(p) == nil {
		t.Fatal("extra request accepted")
	}
	delete(p.Input.WireRequests, "extra")
	v.WireRequest = nil
	if _, e := boundWire(p, a, v, true); e == nil {
		t.Fatal("no evidence accepted")
	}
	p.Input.WireRequests = nil
	if _, e := boundWire(p, a, v, true); e == nil {
		t.Fatal("legacy send accepted")
	}
	if _, e := boundWire(p, a, v, false); e != nil {
		t.Fatal("legacy offline core broken", e)
	}
}
