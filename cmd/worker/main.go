package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/globalreality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/rollingreboot"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/serverguardian"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/usercapacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/scheduler"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"os/signal"
	"sync"
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
	if n, err := (rollingreboot.Service{DB: application.DB}).ReconcileDeferred(ctx); err != nil {
		log.Printf("rolling reboot deferred reconcile: %v", err)
	} else if n > 0 {
		log.Printf("rolling reboot obsolete=%d", n)
	}
	// Keep local repair self-healing even without a process restart. This clears
	// stale Vultr probe claims, disabled-account leases and provable orphan links.
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				application.Container.ReconcileLocalState(ctx)
				if n, err := (rollingreboot.Service{DB: application.DB}).ReconcileDeferred(ctx); err != nil {
					log.Printf("rolling reboot deferred reconcile: %v", err)
				} else if n > 0 {
					log.Printf("rolling reboot obsolete=%d", n)
				}
			}
		}
	}()
	go application.Container.RunDailyCatalogSync(ctx)
	// Persist the rolling-reboot plan only. Execution remains disabled until
	// the dry-run queue is reviewed and explicitly enabled.
	if n, err := (rollingreboot.Service{DB: application.DB, Enabled: false, MinReadyPerAccount: 2}).Plan(ctx); err != nil {
		log.Printf("rolling reboot plan: %v", err)
	} else if n > 0 {
		log.Printf("rolling reboot plan queued=%d mode=dry-run", n)
	}
	// Fleet resource guardian starts in observe-only mode. It records x-ui,
	// memory, disk, package-lock and DB health without mutating server state.
	go func() {
		g := serverguardian.Service{
			DB: application.DB, Secrets: application.Container.Secrets,
			SSH:    provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}},
			Repair: true,
		}
		run := func() {
			c, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if err := g.Run(c); err != nil && c.Err() == nil {
				log.Printf("server guardian: %v", err)
			}
		}
		run()
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
	// Sticky proxy identity keeper: preserve each account's current exit IPv4.
	// On failure retry the previous session every 10s for two minutes, then
	// rotate within the preferred country until five minutes, then allow fallback.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		run := func() {
			rows, err := application.DB.QueryContext(ctx, `SELECT a.id::text FROM accounts a JOIN network_profiles np ON np.account_id=a.id WHERE np.mode='proxy_required' AND (a.enabled=true OR (a.deletion_requested_at IS NOT NULL AND EXISTS(SELECT 1 FROM droplets d WHERE d.account_id=a.id AND d.state<>'DELETED')))`)
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
			sem := make(chan struct{}, 64)
			var wg sync.WaitGroup
			for _, id := range ids {
				id := id
				wg.Add(1)
				go func() {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						return
					}
					cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
					defer cancel()
					_ = application.Container.MaintainProxyControlPlane(cctx, id)
				}()
			}
			wg.Wait()
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
	// Panel registry: materialize every completed READY Sanaei deployment into panel_instances.
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		store := panels.SQLStore{DB: application.DB}
		run := func() {
			if _, err := store.ReconcileInstances(ctx); err != nil {
				log.Printf("panel instance reconcile: %v", err)
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

	// Shared fast Sanaei runtime cache for high-frequency panel work.
	sanaeiRuntimes := &sanaei.RuntimeManager{
		Factory: sanaei.RuntimeFactory{DB: application.DB, Secrets: application.Container.Secrets, Timeout: 8 * time.Second},
		TTL:     5 * time.Second,
	}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sanaeiRuntimes.PurgeExpired()
			}
		}
	}()

	// Global Reality policy: every READY Sanaei panel converges to the globally configured ports.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}}
		reconciler := globalreality.Service{DB: application.DB, Secrets: application.Container.Secrets, SSH: ssh, Runtimes: sanaeiRuntimes}
		run := func() {
			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("global reality discovery: %v", err)
				return
			}
			sem := make(chan struct{}, 8)
			var wg sync.WaitGroup
			for _, panel := range panels {
				panel := panel
				wg.Add(1)
				go func() {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						return
					}
					cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
					defer cancel()
					if err := reconciler.ReconcilePanel(cctx, panel, false); err != nil && cctx.Err() == nil {
						log.Printf("global reality panel %s: %v", panel.ID, err)
					}
				}()
			}
			wg.Wait()
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

	// User capacity fast fill: lightweight slim inventory + batch add.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		capacity := usercapacity.Service{DB: application.DB, Secrets: application.Container.Secrets}
		run := func() {
			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("user capacity discovery: %v", err)
				return
			}
			sem := make(chan struct{}, 8)
			var wg sync.WaitGroup
			for _, panel := range panels {
				panel := panel
				wg.Add(1)
				go func() {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						return
					}
					cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
					defer cancel()
					runtime, err := sanaeiRuntimes.Acquire(cctx, panel.ID)
					if err != nil {
						if cctx.Err() == nil {
							log.Printf("user capacity fast runtime %s: %v", panel.ID, err)
						}
						return
					}
					if _, err = capacity.FastFillFromPolicy(cctx, panel, runtime); err != nil && cctx.Err() == nil {
						log.Printf("user capacity fast fill %s: %v", panel.ID, err)
					}
				}()
			}
			wg.Wait()
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

	// User capacity cleanup: expensive full snapshot on a slower cadence.
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		capacity := usercapacity.Service{DB: application.DB, Secrets: application.Container.Secrets}
		run := func() {
			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("user capacity cleanup discovery: %v", err)
				return
			}
			for _, panel := range panels {
				cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				runtime, err := sanaeiRuntimes.Acquire(cctx, panel.ID)
				if err == nil {
					err = capacity.ReconcileRuntimeFromPolicy(cctx, panel, runtime)
				}
				cancel()
				if err != nil && ctx.Err() == nil {
					log.Printf("user capacity cleanup %s: %v", panel.ID, err)
				}
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

	// Residential Ads sync: fan out active residential proxies to every ready Sanaei panel.
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}}
		syncer := residentialsync.Service{DB: application.DB, Secrets: application.Container.Secrets, SSH: ssh}
		failures := worker.FailureStore{DB: application.DB}
		run := func() {
			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("residential sync discovery: %v", err)
				return
			}
			sem := make(chan struct{}, 12)
			done := make(chan struct{}, len(panels))
			for _, panel := range panels {
				p := panel
				go func() {
					defer func() { done <- struct{}{} }()
					select {
					case sem <- struct{}{}:
						defer func() { <-sem }()
					case <-ctx.Done():
						return
					}
					if !failures.Due(ctx, "residential_sync", p.ID) {
						return
					}
					panelCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					if err := syncer.ReconcilePanel(panelCtx, p, false); err != nil {
						failures.Fail(ctx, "residential_sync", p.ID, "", err)
						log.Printf("residential sync panel %s: %v", p.ID, err)
						return
					}
					failures.Clear(ctx, "residential_sync", p.ID)
				}()
			}
			for range panels {
				select {
				case <-done:
				case <-ctx.Done():
					return
				}
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

	monitor := network.Monitor{DB: application.DB, Secrets: application.Container.Secrets, Interval: 10 * time.Second, Timeout: 8 * time.Second, Policy: network.HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: 5 * time.Second}}
	go func() {
		if err := monitor.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("proxy monitor stopped: %v", err)
		}
	}()
	failures := worker.FailureStore{DB: application.DB}
	lw := &worker.LifecycleWorker{Store: droplets.LifecycleStore{DB: application.DB}, Handler: application.Container, Failures: failures, Batch: 100}
	// Lifecycle runs independently from provider/deployment recovery so slow or
	// unknown provider operations cannot starve expiry and deletion processing.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			if err := lw.Once(ctx); err != nil && ctx.Err() == nil {
				log.Printf("lifecycle worker: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	w := worker.Worker{Store: worker.RecoveryStore{DB: application.DB}, Handler: app.RecoveryHandler{Container: application.Container}, Failures: failures, Interval: 10 * time.Second, Batch: 100}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
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
