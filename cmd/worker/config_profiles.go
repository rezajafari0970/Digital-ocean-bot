package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"strings"
	"time"
)

func addConfigProfileModules(modules *worker.Modules, a *app.Application, rt *sanaei.RuntimeManager, budget *worker.WorkBudget) {
	create := worker.NewWorkClass(budget, 2)
	verify := worker.NewWorkClass(budget, 4)
	for _, name := range []string{"profiles-create-0", "profiles-create-1", "profiles-verify-0", "profiles-verify-1", "profiles-verify-2", "profiles-verify-3"} {
		modules.Add(worker.RolePanels, name, func(ctx context.Context) {
			service := configplane.Service{DB: a.DB, Secrets: a.Container.Secrets, Runtimes: rt, SSH: provisioning.SSHClient{OutputLimit: 16384, Timeout: 420 * time.Second, HostKeys: provisioning.SQLHostKeyPins{DB: a.DB}}}
			ticker := time.NewTicker(time.Second)
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
				duration := 7 * time.Minute
				if strings.Contains(name, "verify") {
					duration = 45 * time.Second
				}
				bounded, cancel := context.WithTimeout(work, duration)
				if strings.Contains(name, "verify") {
					e = verify.Do(bounded, service.RunVerifyOne)
				} else {
					e = create.Do(bounded, service.RunOne)
				}
				cancel()
				finish()
				if e != nil && ctx.Err() == nil {
					log.Printf("config profile reconcile: %v", e)
				}
			}
		})
	}
}
