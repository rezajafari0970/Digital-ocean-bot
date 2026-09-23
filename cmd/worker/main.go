package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
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
	lw := &worker.LifecycleWorker{Store: droplets.LifecycleStore{DB: application.DB}, Handler: application.Container, Batch: 100}
	w := worker.Worker{Store: worker.RecoveryStore{DB: application.DB}, Handler: app.RecoveryHandler{Container: application.Container}, Lifecycle: lw, Interval: 10 * time.Second, Batch: 100}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		engine := scheduler.Engine{DB: application.DB, Store: scheduler.SQLStore{DB: application.DB}, Starter: app.ScheduledStarter{Container: application.Container}}
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
