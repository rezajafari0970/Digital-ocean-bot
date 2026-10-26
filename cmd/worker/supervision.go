package main

import (
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"strings"
	"time"
)

// Idle is the actual scheduling cadence. Work starts only after admission;
// existing native timeouts precede these conservative cancellation ceilings.
func modulePolicy(name string) (supervision.Policy, bool) {
	p := supervision.Policy{Loop: time.Minute, Work: 3 * time.Minute}
	switch name {
	case "cloudflare-resources":
		p.Work = time.Minute
		p.Idle = time.Second
	case "heartbeat":
		p.Loop = 30 * time.Second
		p.Work = 10 * time.Second
		p.Idle = 10 * time.Second
	case "local-repair":
		p.Idle = 5 * time.Minute
	case "account-deletion":
		p.Work = time.Minute
		p.Idle = 10 * time.Second
	case "billing":
		p.Work = 30 * time.Second
		p.Idle = 5 * time.Second
	case "catalog":
		p.Work = 20 * time.Minute
		p.Idle = 24 * time.Hour
	case "server-protection", "server-protection-cleanup":
		p.AsyncLoop = true
		p.Loop = 2 * time.Minute
		p.Work = 45 * time.Second
		p.Idle = 5 * time.Second
	case "server-guardian":
		p.Work = 150 * time.Second
		p.Idle = 5 * time.Minute
	case "network-identity":
		p.Work = time.Minute
		p.Idle = 10 * time.Second
	case "provider-capacity":
		p.AsyncLoop = true
		p.Work = 55 * time.Second
		p.Idle = 15 * time.Second
	case "panel-registry":
		p.Work = 30 * time.Second
		p.Idle = 5 * time.Second
	case "sanaei-cache":
		p.Idle = 5 * time.Second
	case "trojan-tcp-0", "trojan-tcp-1", "trojan-verify-0", "trojan-verify-1":
		p.Work = 8 * time.Minute
		p.Idle = 3 * time.Second
	case "profiles-create-0", "profiles-create-1", "profiles-create-2", "profiles-create-3", "profiles-create-4", "profiles-create-5", "profiles-create-6", "profiles-create-7", "profiles-delete-0", "profiles-delete-1", "profiles-delete-2", "profiles-delete-3", "profiles-delete-4", "profiles-delete-5", "profiles-delete-6", "profiles-delete-7":
		p.Work = 8 * time.Minute
		p.Idle = time.Second
	case "profiles-verify-0", "profiles-verify-1", "profiles-verify-2", "profiles-verify-3", "profiles-verify-4", "profiles-verify-5", "profiles-verify-6", "profiles-verify-7":
		p.Work = 150 * time.Second
		p.Idle = time.Second
	case "transport-apply":
		p.Work = 5 * time.Minute
		p.Idle = 3 * time.Second
	case "client-mutation":
		p.Work = 2 * time.Minute
		p.Idle = time.Second
	case "panel-cleanup":
		p.Work = 10 * time.Minute
		p.Idle = time.Second
	case "global-reality", "global-reality-bootstrap":
		p.Work = 30 * time.Second
		p.Idle = 10 * time.Second
	case "capacity-fill":
		p.Work = 30 * time.Second
		p.Idle = time.Second
	case "capacity-cleanup":
		p.Work = 75 * time.Second
		p.Idle = time.Minute
	case "residential-performance":
		p.Work = time.Minute
		p.Idle = 5 * time.Second
	case "residential-protocol-health":
		p.Work = 90 * time.Second
		p.Idle = 5 * time.Second
	case "trial-relay-health":
		p.Loop = 2 * time.Minute
		p.Work = 10 * time.Second
		p.Idle = 5 * time.Second
	case "residential-monitor":
		p.Loop = 2 * time.Minute
		p.Work = 15 * time.Second
		p.Idle = 10 * time.Second
	case "network-monitor":
		p.Work = 15 * time.Second
		p.Idle = 10 * time.Second
	case "observation-retention":
		p.Idle = 5 * time.Minute
	case "account-rules":
		p.Idle = 5 * time.Second
	case "lifecycle":
		p.AsyncLoop = true
		p.Work = 110 * time.Second
		p.Idle = 10 * time.Second
	case "scheduler":
		p.Work = 15 * time.Minute
		p.Idle = 5 * time.Second
	case "recovery":
		p.AsyncLoop = true
		p.Work = 3 * time.Minute
		p.Idle = 10 * time.Second
	default:
		if !strings.HasPrefix(name, "residential-sync-") {
			return p, false
		}
		p.Work = 70 * time.Second
		p.Idle = time.Second
	}
	return p, true
}
func configureSupervision(m *worker.Modules) {
	m.Supervisor = supervision.New(nil)
	m.Policies = map[string]supervision.Policy{}
	for _, name := range m.Names(worker.RoleAll) {
		p, ok := modulePolicy(name)
		if !ok || !p.Valid() {
			panic(fmt.Sprintf("missing module policy: %s", name))
		}
		m.Policies[name] = p
	}
	m.Grace = 15 * time.Second
}
