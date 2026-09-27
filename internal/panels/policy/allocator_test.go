package policy

import (
	"errors"
	"reflect"
	"testing"
)

func TestAllocatePreferredThenDynamic(t *testing.T) {
	p := InboundPolicy{DesiredCount: 5, PreferredPorts: []int{443, 8443, 2052, 2087}, DynamicPortStart: 10000, DynamicPortEnd: 10010, ReservedPorts: []int{2052}}
	got, e := AllocatePorts(p, []int{8443, 10000})
	if e != nil {
		t.Fatal(e)
	}
	want := []int{443, 2087, 10001, 10002, 10003}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}
func TestAllocateDeterministic(t *testing.T) {
	p := InboundPolicy{DesiredCount: 3, DynamicPortStart: 12000, DynamicPortEnd: 12010}
	a, _ := AllocatePorts(p, nil)
	b, _ := AllocatePorts(p, nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%v %v", a, b)
	}
}
func TestAllocateFailsClosed(t *testing.T) {
	p := InboundPolicy{DesiredCount: 3, DynamicPortStart: 10000, DynamicPortEnd: 10001}
	_, e := AllocatePorts(p, nil)
	if !errors.Is(e, ErrInsufficientPorts) {
		t.Fatalf("%v", e)
	}
}
