package network

import (
	"context"
	"errors"
	"testing"
)

func TestResolveIPv4RejectsIPv6Literal(t *testing.T) {
	_, err := ResolveIPv4(context.Background(), "2001:db8::1")
	if !errors.Is(err, ErrIPv4Required) {
		t.Fatalf("err=%v", err)
	}
}
func TestResolveIPv4AcceptsIPv4Literal(t *testing.T) {
	got, err := ResolveIPv4(context.Background(), "203.0.113.7")
	if err != nil || got != "203.0.113.7" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
