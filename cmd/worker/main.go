package main

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/cleanup"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/globalreality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/rollingreboot"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/serverguardian"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/usercapacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residential"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/scheduler"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/serverprotection"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	application, err := app.BootstrapWorker(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	if err := (migrate.Runner{DB: application.DB, Dir: "migrations"}).Up(ctx); err != nil {
		log.Fatal(err)
	}
	// Each process has a distinct liveness record; stale processes age out.
	host, _ := os.Hostname()
	var recoveryProgress, schedulerProgress, lifecycleProgress atomic.Int64
	heartbeat := worker.Heartbeat{DB: application.DB, WorkerID: fmt.Sprintf("%s:%d", host, os.Getpid()), Kind: "production"}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			beatCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := heartbeat.Beat(beatCtx, map[string]any{"role": "worker", "recovery_scan_unix": recoveryProgress.Load(), "scheduler_scan_unix": schedulerProgress.Load(), "lifecycle_scan_unix": lifecycleProgress.Load()})
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("worker heartbeat: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
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
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			jobCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			if err := application.Container.ProcessAccountDeletions(jobCtx); err != nil && ctx.Err() == nil {
				log.Printf("account deletion remains pending")
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			billingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if err := application.Container.RefreshOneBillingAccount(billingCtx); err != nil && ctx.Err() == nil {
				log.Printf("billing observation persistence failed")
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
	// The local protection controller is inert until its Config policy is enabled.
	go (serverprotection.Controller{DB: application.DB, Secrets: application.Container.Secrets,
		UpgradePanels: serverprotection.ParseUpgradePanels(os.Getenv("DOB_GUARDIAN_UPGRADE_PANELS")),
		SSH:           provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}}}).Run(ctx)
	// Existing periodic diagnostics retain their recovery fallback on unmanaged nodes.
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
			ids, err := application.Container.AccountNetworkMaintenanceIDs(ctx)
			if err != nil {
				return
			}
			// Keep advisory-lease holders below the worker connection budget.
			sem := make(chan struct{}, 4)
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
	// when the shared snapshot is older than 60s. The 15s cadence leaves room for network latency before
	// the unchanged 2m freshness/safety boundary. Provider backoff still applies.
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		application.Container.RefreshProviderSnapshots(ctx, 60*time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				application.Container.RefreshProviderSnapshots(ctx, 60*time.Second)
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

	// Durable client mutations are always serialized. The database execution gate
	// is checked atomically by Claim; its production default is OFF + kill-switch ON.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		exec := clientops.Executor{
			Journal:  clientops.Journal{DB: application.DB},
			Runtimes: sanaeiRuntimes,
			Timeout:  30 * time.Second,
		}
		run := func() {
			if _, err := exec.Drain(ctx); err != nil && ctx.Err() == nil {
				if gateErr := exec.Journal.FailCloseGateWithFailure(ctx, err); gateErr != nil {
					log.Printf("client mutation executor fail-close: %v (original: %v)", gateErr, err)
					return
				}
				log.Printf("client mutation executor: %v; execution gate fail-closed", err)
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		executor := cleanup.Service{DB: application.DB, Runtimes: sanaeiRuntimes}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := executor.RunOne(ctx); err != nil {
					log.Printf("panel cleanup paused: %v", err)
				}
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

	// User capacity fast fill: time-budgeted per inbound; the one-second cadence
	// makes users_per_second match its configured wall-clock meaning.
	go func() {
		t := time.NewTicker(1 * time.Second)
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
			sem := make(chan struct{}, 8)
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
					cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
					defer cancel()
					runtime, err := sanaeiRuntimes.Acquire(cctx, p.ID)
					if err == nil {
						err = capacity.ReconcileRuntimeFromPolicy(cctx, p, runtime)
					}
					if err != nil && ctx.Err() == nil {
						log.Printf("user capacity cleanup %s: %v", p.ID, err)
					}
				}()
			}
			for range panels {
				<-done
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

	// Eight disjoint serving lanes keep running-router proofs inside the
	// publication freshness window. Retirement has its own single lane.
	// Client mutation execution remains independently gated at concurrency=1.
	for _, serving := range []bool{true, false} {
		lanes := 1
		if serving {
			lanes = 8
		}
		for shard := 0; shard < lanes; shard++ {
			go func(serving bool, shard, lanes int) {
				t := time.NewTicker(time.Second)
				defer t.Stop()
				syncer := residentialsync.Service{DB: application.DB, Secrets: application.Container.Secrets, Runtimes: sanaeiRuntimes}
				failures := worker.FailureStore{DB: application.DB}
				for {
					p, found, err := syncer.NextDuePanelShard(ctx, serving, shard, lanes)
					if err != nil {
						log.Printf("residential sync discovery: %v", err)
					}
					if err == nil && found {
						panelCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
						err = syncer.ReconcilePanel(panelCtx, p, false)
						cancel()
						if err != nil {
							failures.Fail(ctx, "residential_sync", p.ID, "", err)
							log.Printf("residential sync panel %s: %v", p.ID, err)
						} else {
							failures.Clear(ctx, "residential_sync", p.ID)
						}
						continue
					}
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			}(serving, shard, lanes)
		}
	}

	go (residentialperf.Store{DB: application.DB}).Run(ctx)
	go application.Container.RunObservationRetention(ctx)
	go func() { _ = (residential.Monitor{DB: application.DB, Secrets: application.Container.Secrets}).Run(ctx) }()
	monitor := network.Monitor{DB: application.DB, Secrets: application.Container.Secrets, Interval: 10 * time.Second, Timeout: 8 * time.Second, Policy: network.HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: 5 * time.Second}}
	go func() {
		if err := monitor.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("proxy monitor stopped: %v", err)
		}
	}()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if err := application.Container.ProcessAccountRuleApplications(ctx); err != nil && ctx.Err() == nil {
				log.Printf("account rule application: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
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
			if err := lw.Once(ctx); err != nil {
				if ctx.Err() == nil {
					log.Printf("lifecycle worker: %v", err)
				}
			} else {
				lifecycleProgress.Store(time.Now().Unix())
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	w := worker.Worker{Store: worker.RecoveryStore{DB: application.DB}, Handler: app.RecoveryHandler{Container: application.Container}, Failures: failures, Interval: 10 * time.Second, Batch: 100, Progress: func(t time.Time) { recoveryProgress.Store(t.Unix()) }}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		engine := scheduler.Engine{DB: application.DB, Store: scheduler.SQLStore{DB: application.DB}, Leases: scheduler.LeaseStore{DB: application.DB}, Starter: app.ScheduledStarter{Container: application.Container}}
		for {
			if err := engine.RunDue(ctx, time.Now().UTC()); err != nil {
				if ctx.Err() == nil {
					log.Printf("scheduler: %v", err)
				}
			} else {
				schedulerProgress.Store(time.Now().Unix())
			}
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
