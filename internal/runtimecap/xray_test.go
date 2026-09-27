package runtimecap

import (
	"context"
	"errors"
	"testing"
)

func TestResolveXray(t *testing.T) {
	r := XrayResolver{Run: func(context.Context, string) (string, error) { return "/opt/xray\n", nil }}
	p, e := r.Resolve(context.Background())
	if e != nil || p != "/opt/xray" {
		t.Fatalf("%q %v", p, e)
	}
}
func TestResolveMissing(t *testing.T) {
	r := XrayResolver{Run: func(context.Context, string) (string, error) { return "", nil }}
	_, e := r.Resolve(context.Background())
	if !errors.Is(e, ErrXrayNotFound) {
		t.Fatalf("%v", e)
	}
}
