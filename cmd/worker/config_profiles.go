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
	// Fleet creation can use the available parent budget without increasing
	// SQL occupancy; independent proof and removal classes still share that ceiling.
	create := worker.NewWorkClass(budget, min(8, limit))
	// Proof workers share the SQL-derived panel budget. Queue wait must not
	// consume an admitted operation's runtime deadline.
	verify := worker.NewWorkClass(budget, min(8, limit))
	// Independent panels can drain a fleet deletion without raising the shared
	// SQL/SSH ceiling or allowing concurrent writers on a single panel.
	remove := worker.NewWorkClass(budget, min(8, limit))
	for _, name := range []string{"profiles-create-0", "profiles-create-1", "profiles-create-2", "profiles-create-3", "profiles-create-4", "profiles-create-5", "profiles-create-6", "profiles-create-7", "profiles-delete-0", "profiles-delete-1", "profiles-delete-2", "profiles-delete-3", "profiles-delete-4", "profiles-delete-5", "profiles-delete-6", "profiles-delete-7", "profiles-verify-0", "profiles-verify-1", "profiles-verify-2", "profiles-verify-3", "profiles-verify-4", "profiles-verify-5", "profiles-verify-6", "profiles-verify-7"} {
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
					duration = 2 * time.Minute
				}
				if strings.Contains(name, "verify") {
					run := service.RunVerifyOne
					// Two of eight consumers recover failed classes promptly. Six retain
					// deadline order so a failing provider cannot starve healthy refreshes.
					if name == "profiles-verify-0" || name == "profiles-verify-1" {
						run = service.RunVerifyRecoveryOne
					}
					e = runConfigProfileWork(work, verify.Do, duration, run)
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
