package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/scheduler"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	application, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	if err := (migrate.Runner{DB: application.DB, Dir: "migrations"}).Up(ctx); err != nil {
		log.Fatal(err)
	}
	// Repair only locally provable state links before any scheduler/lifecycle work.
	application.Container.ReconcileLocalState(ctx)
	go application.Container.RunDailyCatalogSync(ctx)
	// Sticky proxy identity keeper: preserve each account's current exit IP while
	// healthy. On failure, retry the preferred country every 30s; after five
	// minutes the resolver may accept a healthy unique fallback country.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		run := func() {
			rows, err := application.DB.QueryContext(ctx, `SELECT a.id::text FROM accounts a JOIN network_profiles np ON np.account_id=a.id WHERE a.enabled=true AND np.mode='proxy_required'`)
			if err != nil {
				return
			}
			var ids []string
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			for _, id := range ids {
				_ = application.Container.MaintainStickyIdentity(ctx, id)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
	// Browser audit queue is independent from the sticky health loop.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		application.Container.RunBrowserAuditQueue(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				application.Container.RunBrowserAuditQueue(ctx)
			}
		}
	}()

	// Capacity refresh coordinator: one full provider refresh per account only
	// when the shared snapshot is older than 90s. Runs before the 2m safety gate.
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		application.Container.RefreshProviderSnapshots(ctx, 90*time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				application.Container.RefreshProviderSnapshots(ctx, 90*time.Second)
			}
		}
	}()
	monitor := network.Monitor{DB: application.DB, Secrets: application.Container.Secrets, Interval: 30 * time.Second, Timeout: 12 * time.Second, Policy: network.HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: 5 * time.Second}}
	go func() {
		if err := monitor.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("proxy monitor stopped: %v", err)
		}
	}()
	failures := worker.FailureStore{DB: application.DB}
	lw := &worker.LifecycleWorker{Store: droplets.LifecycleStore{DB: application.DB}, Handler: application.Container, Failures: failures, Batch: 100}
	w := worker.Worker{Store: worker.RecoveryStore{DB: application.DB}, Handler: app.RecoveryHandler{Container: application.Container}, Lifecycle: lw, Failures: failures, Interval: 10 * time.Second, Batch: 100}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		engine := scheduler.Engine{DB: application.DB, Store: scheduler.SQLStore{DB: application.DB}, Leases: scheduler.LeaseStore{DB: application.DB}, Starter: app.ScheduledStarter{Container: application.Container}}
		for {
			_ = engine.RunDue(ctx, time.Now().UTC())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	log.Printf("worker started")
	if err := w.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
