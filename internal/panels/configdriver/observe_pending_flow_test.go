package configdriver

import (
	"encoding/json"
	"testing"
)

func TestPendingFlowCannotConfirmIdentityOrMembershipDrift(t *testing.T) {
	r := Builtins()
	s := Spec{Protocol: "vless", Network: "tcp", Security: "reality", Port: 24443, SNI: "www.microsoft.com"}
	c, e := NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	c.Clients[0].PolicySet = true
	c.Clients[0].Publish = true
	want, e := r.Build(s, "192.0.2.1", "owned", c)
	if e != nil {
		t.Fatal(e)
	}
	native := normalized(want).(map[string]any)
	native["id"] = float64(7)
	client := native["settings"].(map[string]any)["clients"].([]any)[0].(map[string]any)
	client["flow"] = "xtls-rprx-vision"
	raw := func() []json.RawMessage { b, _ := json.Marshal(native); return []json.RawMessage{b} }
	if _, e = Observe(raw(), want, 7, c); e == nil {
		t.Fatal("unapplied Flow accepted")
	}
	if id, e := ObservePendingFlow(raw(), want, 7, c); e != nil || id != 7 {
		t.Fatal("owned pending Flow rejected", e)
	}
	client["email"] = "another-owner"
	if _, e = ObservePendingFlow(raw(), want, 7, c); e == nil {
		t.Fatal("foreign identity accepted")
	}
	client["email"] = c.Clients[0].Email
	client["totalGB"] = float64(123)
	if _, e = ObservePendingFlow(raw(), want, 7, c); e == nil {
		t.Fatal("foreign limit accepted")
	}
}
