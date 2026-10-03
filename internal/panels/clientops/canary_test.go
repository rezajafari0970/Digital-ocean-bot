package clientops

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestGuardedCanaryAcceptsOnlyFreshCanaryCreate(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	j := Job{
		ClientID: id, Kind: KindCreate, State: StatePending, Attempts: 0,
		IdempotencyKey: "guarded-canary-create-" + id,
		Payload:        CanaryPayload(id),
	}
	if err := ValidateCanary(j); err != nil {
		t.Fatalf("valid canary rejected: %v", err)
	}
	j.Attempts = 1
	if !errors.Is(ValidateCanary(j), ErrCanaryGuard) {
		t.Fatal("retried canary must be rejected")
	}
}

func TestGuardedCanaryRejectsOrdinaryClient(t *testing.T) {
	id := "22222222-2222-4222-8222-222222222222"
	payload, _ := json.Marshal(map[string]any{"Client": map[string]any{
		"id": id, "email": "ordinary-user", "enable": true,
	}})
	j := Job{ClientID: id, Kind: KindCreate, State: StatePending,
		IdempotencyKey: "guarded-canary-create-" + id, Payload: payload}
	if !errors.Is(ValidateCanary(j), ErrCanaryGuard) {
		t.Fatal("ordinary client must never pass canary guard")
	}
}
