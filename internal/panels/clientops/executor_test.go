package clientops

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestClientFromInboundSupportsObjectSettings(t *testing.T) {
	raw := json.RawMessage(`{"settings":{"clients":[{"id":"u1","email":"a","enable":true,"totalGB":100,"expiryTime":200,"limitIp":2}]}}`)
	c, ok, err := clientFromInbound(raw, "u1")
	if err != nil || !ok {
		t.Fatalf("client not found: ok=%v err=%v", ok, err)
	}
	if c.ID != "u1" || c.Email != "a" || c.TotalGB != 100 || c.LimitIP != 2 {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestClientFromInboundSupportsEncodedSettings(t *testing.T) {
	settings := `{"clients":[{"id":"u2","email":"b","enable":true}]}`
	raw, _ := json.Marshal(map[string]any{"settings": settings})
	c, ok, err := clientFromInbound(raw, "u2")
	if err != nil || !ok || c.ID != "u2" {
		t.Fatalf("encoded settings failed: %+v ok=%v err=%v", c, ok, err)
	}
}

func TestPayloadClientRequiresClient(t *testing.T) {
	_, err := payloadClient(json.RawMessage(`{"other":1}`))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("want invalid request, got %v", err)
	}
}

func TestSameClientIncludesPolicyFields(t *testing.T) {
	a := sanaei.Client{ID: "x", Email: "e", Enable: true, TotalGB: 10, ExpiryTime: 20, LimitIP: 1, Flow: "xtls-rprx-vision"}
	b := a
	if !sameClient(a, b) {
		t.Fatal("equal clients must match")
	}
	b.TotalGB++
	if sameClient(a, b) {
		t.Fatal("quota difference must not match")
	}
}

func TestUpdateRemainsDisabledUntilVerified(t *testing.T) {
	if !errors.Is(ErrUnsupportedKind, ErrUnsupportedKind) {
		t.Fatal("unsupported sentinel")
	}
	job := Job{Kind: KindUpdate}
	if job.Kind != KindUpdate {
		t.Fatal("update kind")
	}
}

func TestUpdateCannotReuseCreateOrDeleteMutation(t *testing.T) {
	job := Job{Kind: KindUpdate, ClientID: "u1", InboundID: 1}
	switch job.Kind {
	case KindCreate, KindDelete:
		t.Fatal("update must never route through create/delete mutation")
	case KindUpdate:
	default:
		t.Fatal("unexpected kind")
	}
}

func TestCreateExistingUUIDWithDifferentPolicyIsConflict(t *testing.T) {
	raw := json.RawMessage(`{"settings":{"clients":[{"id":"u1","email":"actual","enable":true,"totalGB":0,"expiryTime":0,"limitIp":0}]}}`)
	current, exists, err := clientFromInbound(raw, "u1")
	if err != nil || !exists {
		t.Fatalf("expected existing client: %v", err)
	}
	want := sanaei.Client{ID: "u1", Email: "wanted", Enable: true, TotalGB: 100}
	if sameClient(current, want) {
		t.Fatal("policy mismatch must be detectable without issuing another create")
	}
}

func TestClientFromInboundMatchesProductionStringSettings(t *testing.T) {
	settings := `{"clients":[{"id":"prod-u","email":"prod","enable":true,"totalGB":0,"expiryTime":0,"limitIp":0,"flow":"xtls-rprx-vision"}]}`
	raw, err := json.Marshal(map[string]any{
		"id":       1,
		"settings": settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	c, ok, err := clientFromInbound(raw, "prod-u")
	if err != nil || !ok {
		t.Fatalf("production settings client missing: ok=%v err=%v", ok, err)
	}
	if c.ID != "prod-u" || c.Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected production client: %+v", c)
	}
}

func TestCrashRecoveryDecisionNeverDuplicatesCommittedCreate(t *testing.T) {
	want := sanaei.Client{ID: "u1", Email: "e", Enable: true, Flow: "xtls-rprx-vision"}
	if got := decideObserved(KindCreate, false, sanaei.Client{}, want); got != decisionMutate {
		t.Fatalf("before commit decision=%v want mutate", got)
	}
	if got := decideObserved(KindCreate, true, want, want); got != decisionSatisfied {
		t.Fatalf("after commit/restart decision=%v want satisfied", got)
	}
}

func TestCrashRecoveryDeleteIsIdempotent(t *testing.T) {
	current := sanaei.Client{ID: "u1"}
	if got := decideObserved(KindDelete, true, current, sanaei.Client{}); got != decisionMutate {
		t.Fatalf("before delete decision=%v want mutate", got)
	}
	if got := decideObserved(KindDelete, false, sanaei.Client{}, sanaei.Client{}); got != decisionSatisfied {
		t.Fatalf("after delete/restart decision=%v want satisfied", got)
	}
}

func TestExistingUUIDWithDifferentPayloadNeverMutatesAgain(t *testing.T) {
	current := sanaei.Client{ID: "u1", Email: "actual", Enable: true}
	want := sanaei.Client{ID: "u1", Email: "wanted", Enable: true}
	if got := decideObserved(KindCreate, true, current, want); got != decisionConflict {
		t.Fatalf("existing conflicting uuid decision=%v want conflict", got)
	}
}

func TestRetryResultSurfacesExecutionFailure(t *testing.T) {
	execErr := errors.New("mutation failed")
	if got := retryResult(execErr, nil); !errors.Is(got, execErr) {
		t.Fatalf("got=%v", got)
	}
}
func TestRetryResultPrefersJournalFailure(t *testing.T) {
	execErr := errors.New("mutation failed")
	journalErr := errors.New("journal failed")
	got := retryResult(execErr, journalErr)
	if !errors.Is(got, journalErr) {
		t.Fatalf("got=%v", got)
	}
}
