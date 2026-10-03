package clientops

import (
	"database/sql"
	"strings"
	"testing"
)

func TestExecutionGateFailClosed(t *testing.T) {
	job := Job{PanelID: "p1", InboundID: 1}
	cases := []ExecutionGate{
		{},
		{Enabled: true, KillSwitch: true, Concurrency: 1},
		{Enabled: true, KillSwitch: false, Concurrency: 0},
		{Enabled: true, KillSwitch: false, Concurrency: 2},
	}
	for i, g := range cases {
		if g.Allows(job) {
			t.Fatalf("case %d unexpectedly allowed", i)
		}
	}
}

func TestExecutionGateScope(t *testing.T) {
	job := Job{PanelID: "p1", InboundID: 7}
	g := ExecutionGate{Enabled: true, Concurrency: 1}
	if !g.Allows(job) {
		t.Fatal("global enabled gate should allow")
	}
	g.PanelID = sql.NullString{String: "p2", Valid: true}
	if g.Allows(job) {
		t.Fatal("wrong panel scope allowed")
	}
	g.PanelID = sql.NullString{String: "p1", Valid: true}
	g.InboundID = sql.NullInt64{Int64: 8, Valid: true}
	if g.Allows(job) {
		t.Fatal("wrong inbound scope allowed")
	}
	g.InboundID = sql.NullInt64{Int64: 7, Valid: true}
	if !g.Allows(job) {
		t.Fatal("exact scope rejected")
	}
}

func TestClaimSQLAtomicallyEnforcesExecutionGate(t *testing.T) {
	for _, want := range []string{
		"JOIN client_mutation_execution_gate g",
		"g.enabled=true",
		"g.kill_switch=false",
		"g.concurrency=1",
		"g.panel_id IS NULL OR g.panel_id=m.panel_id",
		"g.inbound_id IS NULL OR g.inbound_id=m.inbound_id",
	} {
		if !strings.Contains(claimSQL, want) {
			t.Fatalf("claim SQL missing %q", want)
		}
	}
}
