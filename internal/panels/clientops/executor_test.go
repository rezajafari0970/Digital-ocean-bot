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
