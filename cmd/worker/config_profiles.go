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
	limit := int(budget.Snapshot()["limit"])
	create := worker.NewWorkClass(budget, min(4, limit))
	// Proof workers share the SQL-derived panel budget. Queue wait must not
	// consume an admitted operation's runtime deadline.
	verify := worker.NewWorkClass(budget, min(8, limit))
	remove := worker.NewWorkClass(budget, min(2, limit))
	for _, name := range []string{"profiles-create-0", "profiles-create-1", "profiles-create-2", "profiles-create-3", "profiles-delete-0", "profiles-delete-1", "profiles-verify-0", "profiles-verify-1", "profiles-verify-2", "profiles-verify-3", "profiles-verify-4", "profiles-verify-5", "profiles-verify-6", "profiles-verify-7"} {
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
				duration := 2 * time.Minute
				if strings.Contains(name, "verify") {
					duration = 75 * time.Second
				}
				if strings.Contains(name, "verify") {
					e = runConfigProfileWork(work, verify.Do, duration, service.RunVerifyOne)
				} else if strings.Contains(name, "delete") {
					e = runConfigProfileWork(work, remove.Do, duration, service.RunDeleteOne)
				} else {
					e = runConfigProfileWork(work, create.Do, duration, service.RunOne)
				}
				finish()
				if e != nil && ctx.Err() == nil {
					log.Printf("config profile reconcile: %v", e)
				}
			}
		})
	}
}

// Waiting retains no database session. Give the admitted handler its full
// deadline while preserving worker shutdown cancellation and supervision context.
func runConfigProfileWork(ctx context.Context, admit func(context.Context, func(context.Context) error) error, duration time.Duration, run func(context.Context) error) error {
	return admit(ctx, func(admitted context.Context) error {
		bounded, cancel := context.WithTimeout(admitted, duration)
		defer cancel()
		return run(bounded)
	})
}
