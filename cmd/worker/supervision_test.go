package main

import (
	"testing"
	"time"
)

func TestProtectionLanesHaveIndependentExistingBudgets(t *testing.T) {
	for _, name := range []string{"server-protection", "server-protection-cleanup"} {
		p, ok := modulePolicy(name)
		if !ok || !p.Valid() || p.Loop != 2*time.Minute || p.Work != 45*time.Second || p.Idle != 5*time.Second {
			t.Fatalf("missing or changed protection policy: %s %+v", name, p)
		}
	}
}
