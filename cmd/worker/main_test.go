package main

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"reflect"
	"sort"
	"testing"
)

func TestProductionModulePartitionPreservesPanelSerialization(t *testing.T) {
	modules := buildModules(&app.Application{}, worker.RoleAll, nil)
	control := []string{"account-deletion", "account-rules", "billing", "catalog", "heartbeat", "lifecycle", "local-repair", "network-identity", "network-monitor", "observation-retention", "provider-capacity", "recovery", "scheduler"}
	panels := []string{"capacity-cleanup", "capacity-fill", "client-mutation", "global-reality", "heartbeat", "panel-cleanup", "panel-registry", "residential-monitor", "residential-performance", "sanaei-cache", "server-guardian", "server-protection", "residential-sync-false-0", "residential-sync-true-0", "residential-sync-true-1", "residential-sync-true-2", "residential-sync-true-3", "residential-sync-true-4", "residential-sync-true-5", "residential-sync-true-6", "residential-sync-true-7"}
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
