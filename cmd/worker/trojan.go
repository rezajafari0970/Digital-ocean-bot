package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"strings"
	"time"
)

func addTrojanModules(modules *worker.Modules, a *app.Application, rt *sanaei.RuntimeManager, budget *worker.WorkBudget) {
	addConfigProfileModules(modules, a, rt, budget)
	// Two dedicated bounded slots keep ACME waits out of existing Reality,
	// capacity, route-proof and output admission lanes.
	creation := worker.NewWorkClass(budget, 2)
	verification := worker.NewWorkClass(budget, 2)
	for _, name := range []string{"trojan-tcp-0", "trojan-tcp-1", "trojan-verify-0", "trojan-verify-1"} {
		modules.Add(worker.RolePanels, name, func(ctx context.Context) {
			service := trojantcp.Service{DB: a.DB, Secrets: a.Container.Secrets, Runtimes: rt, SSH: provisioning.SSHClient{OutputLimit: 16384, Timeout: 420 * time.Second, HostKeys: provisioning.SQLHostKeyPins{DB: a.DB}}}
			interval := 3 * time.Second
			if strings.HasPrefix(name, "trojan-verify-") {
				interval = 500 * time.Millisecond
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				supervision.Pulse(ctx)
				supervision.Idle(ctx)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				work, finish, e := supervision.Begin(ctx, "work", 0)
				if e != nil {
					continue
				}
				bounded, cancel := context.WithTimeout(work, 7*time.Minute)
				if strings.HasPrefix(name, "trojan-verify-") {
					verifyCtx, stop := context.WithTimeout(bounded, 45*time.Second)
					e = verification.Do(verifyCtx, service.RunVerifyOne)
					stop()
				} else {
					e = creation.Do(bounded, service.RunOne)
				}
				cancel()
				finish()
				if e != nil && ctx.Err() == nil {
					log.Printf("trojan reconcile: %v", e)
				}
			}
		})
	}
}
