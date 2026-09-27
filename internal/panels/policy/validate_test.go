package policy

import "testing"

func TestValidateRejectsDuplicatePortsAcrossLists(t *testing.T) {
	p := InboundPolicy{ID: "p", PanelID: "x", Protocol: "vless", DynamicPortStart: 10000, DynamicPortEnd: 20000, PreferredPorts: []int{443}, ReservedPorts: []int{443}}
	if Validate(p) == nil {
		t.Fatal("expected invalid")
	}
}
