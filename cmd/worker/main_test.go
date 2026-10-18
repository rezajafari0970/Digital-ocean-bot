package main

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"reflect"
	"sort"
	"testing"
)

func TestProductionModulePartitionPreservesPanelSerialization(t *testing.T) {
	t.Setenv("DOB_RESIDENTIAL_PROOF_CONCURRENCY", "2")
	modules := buildModules(&app.Application{}, worker.RoleAll, nil)
	control := []string{"account-deletion", "account-rules", "billing", "catalog", "heartbeat", "lifecycle", "local-repair", "network-identity", "network-monitor", "observation-retention", "provider-capacity", "recovery", "scheduler"}
	panels := []string{"profiles-create-0", "profiles-create-1", "profiles-verify-0", "profiles-verify-1", "profiles-verify-2", "profiles-verify-3", "profiles-verify-4", "profiles-verify-5", "profiles-verify-6", "profiles-verify-7", "trojan-tcp-0", "trojan-tcp-1", "trojan-verify-0", "trojan-verify-1", "transport-apply", "trial-relay-health", "capacity-cleanup", "capacity-fill", "client-mutation", "global-reality", "global-reality-bootstrap", "heartbeat", "panel-cleanup", "panel-registry", "residential-monitor", "residential-performance", "sanaei-cache", "server-guardian", "server-protection", "server-protection-cleanup", "residential-sync-false-0", "residential-sync-true-0", "residential-sync-true-1"}
	sort.Strings(control)
	sort.Strings(panels)
	if got := modules.Names(worker.RoleControl); !reflect.DeepEqual(got, control) {
		t.Fatalf("control modules: %v", got)
	}
	if got := modules.Names(worker.RolePanels); !reflect.DeepEqual(got, panels) {
		t.Fatalf("panel modules: %v", got)
	}
	union := map[string]bool{}
	for _, n := range control {
		union[n] = true
	}
	for _, n := range panels {
		union[n] = true
	}
	if len(modules.Names(worker.RoleAll)) != len(union) {
		t.Fatal("all-role module coverage changed")
	}
}
