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
		p.Work = 15 * time.Minute
		p.Idle = 15 * time.Second
	case "panel-registry":
		p.Work = 30 * time.Second
		p.Idle = 5 * time.Second
	case "sanaei-cache":
		p.Idle = 5 * time.Second
	case "client-mutation":
		p.Work = 2 * time.Minute
		p.Idle = time.Second
	case "panel-cleanup":
		p.Work = 10 * time.Minute
		p.Idle = time.Second
	case "global-reality":
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
		p.Work = time.Minute
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
