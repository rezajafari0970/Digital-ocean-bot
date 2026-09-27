package planner

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"testing"
)

func TestPlanCreatesMissing(t *testing.T) {
	d := []DesiredInbound{{Key: "dob:r443", Remark: "dob:r443", Protocol: "vless", Port: 443, Enabled: true, Transport: "tcp", Security: "reality", Managed: true}}
	a, e := Plan(d, nil, Policy{})
	if e != nil {
		t.Fatal(e)
	}
	if len(a) != 1 || a[0].Kind != ActionCreate {
		t.Fatalf("%+v", a)
	}
}
func TestPlanDetectsDrift(t *testing.T) {
	d := []DesiredInbound{{Key: "dob:r443", Remark: "dob:r443", Protocol: "vless", Port: 443, Enabled: true, Transport: "tcp", Security: "reality", Managed: true}}
	cur := []inventory.InboundRecord{{RemoteID: 9, Remark: "dob:r443", Protocol: "vless", Port: 8443, Enabled: true, Transport: "tcp", Security: "reality"}}
	a, e := Plan(d, cur, Policy{})
	if e != nil {
		t.Fatal(e)
	}
	if a[0].Kind != ActionUpdate || a[0].RemoteID != 9 {
		t.Fatalf("%+v", a)
	}
}
func TestPlanDoesNotDeleteByDefault(t *testing.T) {
	cur := []inventory.InboundRecord{{RemoteID: 4, Remark: "manual", Protocol: "vless", Port: 1234}}
	a, e := Plan(nil, cur, Policy{})
	if e != nil {
		t.Fatal(e)
	}
	if a[0].Kind != ActionNoop || a[0].Reason != "unmanaged" {
		t.Fatalf("%+v", a)
	}
}
func TestPlanRejectsPortCollision(t *testing.T) {
	d := []DesiredInbound{{Key: "a", Remark: "a", Protocol: "vless", Port: 443}, {Key: "b", Remark: "b", Protocol: "vless", Port: 443}}
	if _, e := Plan(d, nil, Policy{}); e == nil {
		t.Fatal("expected collision")
	}
}
