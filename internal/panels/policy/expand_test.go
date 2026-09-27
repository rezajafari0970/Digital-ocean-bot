package policy

import "testing"

func TestExpandStableKeys(t *testing.T) {
	p := InboundPolicy{ID: "reality-primary", PanelID: "p", Enabled: true, DesiredCount: 2, Protocol: "vless", Transport: "tcp", Security: "reality", DynamicPortStart: 10000, DynamicPortEnd: 10010}
	x, e := Expand(p, nil)
	if e != nil {
		t.Fatal(e)
	}
	if x[0].Key != "dob:reality-primary:000001" || x[1].Port != 10001 {
		t.Fatalf("%+v", x)
	}
}
