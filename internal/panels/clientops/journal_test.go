package clientops

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestValidation(t *testing.T) {
	good := Request{AccountID: "a", PanelID: "p", InboundID: 1, ClientID: "c", Kind: KindCreate, IdempotencyKey: "key", Payload: json.RawMessage(`{"id":"c"}`)}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	bad := good
	bad.InboundID = 0
	if bad.Validate() == nil {
		t.Fatal("zero inbound id must be rejected")
	}
}

func TestClaimOnlyAdmitsEligiblePendingJobs(t *testing.T) {
	for _, want := range []string{"m.state='PENDING'", "a.provider_state='ACTIVE'", "d.state IN ('READY','EXPIRING')", "dep.state='PANEL_COMPLETE"} {
		if !strings.Contains(claimSQL, want) {
			t.Fatalf("claim missing guard %q", want)
		}
	}
	for _, forbidden := range []string{"'RETIRING'", "'DELETING'", "'DELETED'"} {
		if strings.Contains(claimSQL, forbidden) {
			t.Fatalf("claim must not admit lifecycle %s", forbidden)
		}
	}
}

func TestSameRequestUsesSemanticJSON(t *testing.T) {
	j := Job{AccountID: "a", PanelID: "p", InboundID: 1, ClientID: "c", Kind: KindUpdate, IdempotencyKey: "k", Payload: json.RawMessage(`{"a":1,"b":2}`)}
	r := Request{AccountID: "a", PanelID: "p", InboundID: 1, ClientID: "c", Kind: KindUpdate, IdempotencyKey: "k", Payload: json.RawMessage(`{"b":2,"a":1}`)}
	if !sameRequest(j, r) {
		t.Fatal("equivalent JSON payloads must be idempotent")
	}
	r.ClientID = "other"
	if sameRequest(j, r) {
		t.Fatal("different target must conflict")
	}
}
