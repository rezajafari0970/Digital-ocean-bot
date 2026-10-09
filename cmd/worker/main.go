package main

import (
	"context"
	"errors"
	"flag"
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
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	roleFlag := flag.String("role", "all", "worker modules: all, control, panels")
	stateDir := flag.String("state-dir", "/var/lib/digital-ocean-bot/worker-supervision", "per-role restart state directory")
	flag.Parse()
	role, err := worker.ParseRole(*roleFlag)
	if err != nil {
		log.Fatal(err)
	}
	processOwner, err := worker.AcquireProcessRole(filepath.Join(*stateDir, "ownership"), role)
	if err != nil {
		log.Fatal(err)
	}
	defer processOwner.Close()
	gate := &supervision.RestartGate{Path: filepath.Join(*stateDir, string(role)+".json")}
	defer gate.Close()
	if err := gate.Enter(ctx); err != nil {
		log.Printf("worker restart admission unavailable: %v", err)
		select {
		case <-ctx.Done():
		case <-time.After(30 * time.Second):
		}
		log.Fatal("worker startup refused")
	}
	bootCtx, bootCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer bootCancel()
	forceStartup := time.AfterFunc(3*time.Minute+15*time.Second, func() { log.Fatal("worker startup progress deadline exceeded") })
	defer forceStartup.Stop()
	application, err := app.BootstrapWorkerBudget(bootCtx, role.ConnectionBudget())
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	leaseCtx, leaseCancel := context.WithTimeout(bootCtx, 5*time.Second)
	roleLease, err := worker.AcquireRole(leaseCtx, application.DB, role)
	leaseCancel()
	if err != nil {
		log.Fatal(err)
	}
	defer roleLease.Close()
	// Ownership supervision starts before migrations or any startup repair.
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	ownershipFailure := make(chan error, 1)
	go func() {
		if err := roleLease.Run(watchCtx); err != nil && watchCtx.Err() == nil {
			ownershipFailure <- fmt.Errorf("worker role ownership lost: %w", err)
			bootCancel()
		}
	}()

	if err := (migrate.Runner{DB: application.DB, Dir: "migrations"}).Up(bootCtx); err != nil {
		log.Fatal(err)
	}
	if role.Owns(worker.RoleControl) {
		// Repair only locally provable state links before any scheduler/lifecycle work.
		application.Container.ReconcileLocalState(bootCtx)
		if n, err := (rollingreboot.Service{DB: application.DB}).ReconcileDeferred(bootCtx); err != nil {
			log.Printf("rolling reboot deferred reconcile: %v", err)
		} else if n > 0 {
			log.Printf("rolling reboot obsolete=%d", n)
		}
		// Persist the rolling-reboot plan only. Execution remains disabled until
		// the dry-run queue is reviewed and explicitly enabled.
		if n, err := (rollingreboot.Service{DB: application.DB, Enabled: false, MinReadyPerAccount: 2}).Plan(bootCtx); err != nil {
			log.Printf("rolling reboot plan: %v", err)
		} else if n > 0 {
			log.Printf("rolling reboot plan queued=%d mode=dry-run", n)
		}
	}
	if bootCtx.Err() != nil {
		log.Fatal("worker startup deadline exceeded")
	}
	forceStartup.Stop()
	bootCancel()
	modules := buildModules(application, role, roleLease)
	modules.Failure = ownershipFailure
	started := time.Time{}
	reset := false
	modules.Ready = func() error { started = time.Now(); return supervision.Notify("READY=1") }
	modules.Tick = func(s supervision.Snapshot) error {
		if !s.Healthy {
			return supervision.ErrStalled
		}
		if !reset && !started.IsZero() && time.Since(started) >= 5*time.Minute {
			if err := gate.Healthy(); err != nil {
				return err
			}
			reset = true
		}
		return supervision.Notify("WATCHDOG=1")
	}
	log.Printf("worker started role=%s modules=%v db_budget=%d", role, modules.Names(role), role.ConnectionBudget())
	if err := modules.Run(ctx, role); err != nil && (!errors.Is(err, context.Canceled) || ctx.Err() == nil) {
		log.Fatal(err)
	}
}

func buildModules(application *app.Application, role worker.Role, roleLease *worker.RoleLease) worker.Modules {
	var modules worker.Modules
	// Shared admission limits include ALL long-held/nested worker SQL paths.
	controlWork := worker.NewWorkBudget(3)
	panelWork := worker.NewWorkBudget(10)

	// Each process has a distinct liveness record; stale processes age out.
	host, _ := os.Hostname()
	var recoveryProgress, schedulerProgress, lifecycleProgress atomic.Int64
	var clientProgress worker.ModuleProgress
	lifecycleLanes := worker.NewDispatcher(6, 1)
	heartbeat := worker.Heartbeat{DB: application.DB, WorkerID: fmt.Sprintf("%s:%d", host, os.Getpid()), Kind: role.HeartbeatKind()}
	modules.Add(worker.RoleAll, "heartbeat", func(ctx context.Context) {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			supervision.Pulse(ctx)
			beatCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			supervised := modules.Supervisor.Snapshot()
			lanes := lifecycleLanes.Snapshot(0)
			for _, m := range supervised.Modules {
				if m.Name == "lifecycle" && m.State == "STALLED" {
					lanes.Stalled = 1
				}
			}
			err := heartbeat.Beat(beatCtx, map[string]any{"supervision": supervised, "role": role, "modules": modules.Names(role), "database_pool": application.DB.Stats(), "control_work": controlWork.Snapshot(), "panel_work": panelWork.Snapshot(), "recovery_scan_unix": recoveryProgress.Load(), "scheduler_scan_unix": schedulerProgress.Load(), "lifecycle_scan_unix": lifecycleProgress.Load(), "client_mutation": clientProgress.Snapshot(), "lifecycle_lanes": lanes})
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("worker heartbeat: %v", err)
			}
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	// Keep local repair self-healing even without a process restart. This clears
	// stale Vultr probe claims, disabled-account leases and provable orphan links.
	modules.Add(worker.RoleControl, "local-repair", func(ctx context.Context) {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = controlWork.Do(ctx, func(ctx context.Context) error { application.Container.ReconcileLocalState(ctx); return ctx.Err() })
				if n, err := (rollingreboot.Service{DB: application.DB}).ReconcileDeferred(ctx); err != nil {
					log.Printf("rolling reboot deferred reconcile: %v", err)
				} else if n > 0 {
					log.Printf("rolling reboot obsolete=%d", n)
				}
			}
		}
	})
	modules.Add(worker.RoleControl, "account-deletion", func(ctx context.Context) {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			supervision.Pulse(ctx)
			jobCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			if err := controlWork.Do(jobCtx, func(ctx context.Context) error { return application.Container.ProcessAccountDeletions(ctx) }); err != nil && ctx.Err() == nil {
				log.Printf("account deletion remains pending")
			}
			cancel()
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	modules.Add(worker.RoleControl, "billing", func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			supervision.Pulse(ctx)
			billingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if err := controlWork.Do(billingCtx, func(ctx context.Context) error { return application.Container.RefreshOneBillingAccount(ctx) }); err != nil && ctx.Err() == nil {
				log.Printf("billing observation persistence failed")
			}
			cancel()
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	modules.Add(worker.RoleControl, "catalog", func(ctx context.Context) {
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			supervision.Pulse(ctx)
			_ = controlWork.Do(ctx, func(ctx context.Context) error { application.Container.SyncCatalogs(ctx); return ctx.Err() })
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
	// The local protection controller is inert until its Config policy is enabled.
	protection := serverprotection.Controller{Admit: panelWork.Do, DB: application.DB, Secrets: application.Container.Secrets,
		UpgradePanels: serverprotection.ParseUpgradePanels(os.Getenv("DOB_GUARDIAN_UPGRADE_PANELS")),
		SSH:           provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}}}
	modules.Add(worker.RolePanels, "server-protection", protection.Run)
	modules.Add(worker.RolePanels, "server-protection-cleanup", protection.RunCleanup)
	// Existing periodic diagnostics retain their recovery fallback on unmanaged nodes.
	modules.Add(worker.RolePanels, "server-guardian", func(ctx context.Context) {
		g := serverguardian.Service{
			DB: application.DB, Secrets: application.Container.Secrets,
			SSH:    provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}},
			Repair: true,
		}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

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
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})
	// Sticky proxy identity keeper: preserve each account's current exit IPv4.
	// On failure retry the previous session every 10s for two minutes, then
	// rotate within the preferred country until five minutes, then allow fallback.
	modules.Add(worker.RoleControl, "network-identity", func(ctx context.Context) {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

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
					_ = controlWork.Do(cctx, func(ctx context.Context) error { return application.Container.MaintainProxyControlPlane(ctx, id) })
				}()
			}
			wg.Wait()
		}
		run()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})

	// Capacity refresh coordinator: one full provider refresh per account only
	// when the shared snapshot is older than 60s. The 15s cadence leaves room for network latency before
	// the unchanged 2m freshness/safety boundary. Provider backoff still applies.
	modules.Add(worker.RoleControl, "provider-capacity", func(ctx context.Context) {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		_ = controlWork.Do(ctx, func(ctx context.Context) error {
			application.Container.RefreshProviderSnapshots(ctx, 60*time.Second)
			return ctx.Err()
		})
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = controlWork.Do(ctx, func(ctx context.Context) error {
					application.Container.RefreshProviderSnapshots(ctx, 60*time.Second)
					return ctx.Err()
				})
			}
		}
	})
	// Panel registry: materialize every completed READY Sanaei deployment into panel_instances.
	modules.Add(worker.RolePanels, "panel-registry", func(ctx context.Context) {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		store := panels.SQLStore{DB: application.DB}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

			if _, err := store.ReconcileInstances(ctx); err != nil {
				log.Printf("panel instance reconcile: %v", err)
			}
		}
		run()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})

	// Shared fast Sanaei runtime cache for high-frequency panel work.
	sanaeiRuntimes := &sanaei.RuntimeManager{
		Factory: sanaei.RuntimeFactory{DB: application.DB, Secrets: application.Container.Secrets, Timeout: 8 * time.Second},
		TTL:     5 * time.Second,
	}
	modules.Add(worker.RolePanels, "sanaei-cache", func(ctx context.Context) {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sanaeiRuntimes.PurgeExpired()
			}
		}
	})

	// Durable client mutations retain global concurrency1 and the advisory lock.
	modules.Add(worker.RolePanels, "client-mutation", func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		exec := clientops.Executor{Journal: clientops.Journal{DB: application.DB}, Runtimes: sanaeiRuntimes, Timeout: 30 * time.Second, Observe: clientProgress.Observe}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

			clientProgress.Start()
			gateCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			gate, err := exec.Journal.Gate(gateCtx)
			cancel()
			if err != nil {
				clientProgress.Finish("FAILED", err)
				return
			}
			if !gate.Enabled || gate.KillSwitch || gate.Concurrency != 1 {
				clientProgress.Finish("GATED", nil)
				return
			}
			var n int
			err = panelWork.Do(ctx, func(ctx context.Context) error {
				var drainErr error
				n, drainErr = exec.Drain(ctx)
				return drainErr
			})
			if err != nil && ctx.Err() == nil {
				finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				gateErr := exec.Journal.FailCloseGateWithFailure(finishCtx, err)
				finishCancel()
				if gateErr != nil {
					log.Printf("client mutation executor fail-close: %v (original: %v)", gateErr, err)
				} else {
					log.Printf("client mutation executor: %v; execution gate fail-closed", err)
				}
			}
			state := "IDLE"
			if n > 0 {
				state = "COMPLETED"
			}
			clientProgress.Finish(state, err)
		}
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	})

	modules.Add(worker.RolePanels, "panel-cleanup", func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		executor := cleanup.Service{DB: application.DB, Runtimes: sanaeiRuntimes}
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := panelWork.Do(ctx, executor.RunOne); err != nil {
					log.Printf("panel cleanup paused: %v", err)
				}
			}
		}
	})

	// Global Reality policy: every READY Sanaei panel converges to the globally configured ports.
	modules.Add(worker.RolePanels, "global-reality", func(ctx context.Context) {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: application.DB}}
		reconciler := globalreality.Service{DB: application.DB, Secrets: application.Container.Secrets, SSH: ssh, Runtimes: sanaeiRuntimes}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

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
					if err := panelWork.Do(cctx, func(ctx context.Context) error { return reconciler.ReconcilePanel(ctx, panel, false) }); err != nil && cctx.Err() == nil {
						log.Printf("global reality panel %s: %v", panel.ID, err)
					}
				}()
			}
			wg.Wait()
		}
		run()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})

	// User capacity fast fill: time-budgeted per inbound; the one-second cadence
	// makes users_per_second match its configured wall-clock meaning.
	modules.Add(worker.RolePanels, "capacity-fill", func(ctx context.Context) {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		capacity := usercapacity.Service{DB: application.DB, Secrets: application.Container.Secrets}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("user capacity discovery: %v", err)
				return
			}
			_ = worker.RunBoundedBatch(ctx, len(panels), 8, panelWork.Do, func(ctx context.Context, index int) error {
				panel := panels[index]
				cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				runtime, err := sanaeiRuntimes.Acquire(cctx, panel.ID)
				if err == nil {
					_, err = capacity.FastFillFromPolicy(cctx, panel, runtime)
				}
				if err != nil && ctx.Err() == nil {
					log.Printf("user capacity fast fill %s: %v", panel.ID, err)
				}
				return err
			})
		}
		run()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})

	// User capacity cleanup: expensive full snapshot on a slower cadence.
	modules.Add(worker.RolePanels, "capacity-cleanup", func(ctx context.Context) {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		source := readyworker.SQLSource{DB: application.DB}
		capacity := usercapacity.Service{DB: application.DB, Secrets: application.Container.Secrets}
		run := func() {
			ctx, finish, watchErr := supervision.Begin(ctx, "work", 0)
			if watchErr != nil {
				return
			}
			defer finish()

			panels, err := source.EligibleReadyPanels(ctx)
			if err != nil {
				log.Printf("user capacity cleanup discovery: %v", err)
				return
			}
			_ = worker.RunBoundedBatch(ctx, len(panels), 8, panelWork.Do, func(ctx context.Context, index int) error {
				panel := panels[index]
				cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				runtime, err := sanaeiRuntimes.Acquire(cctx, panel.ID)
				if err == nil {
					err = capacity.ReconcileRuntimeFromPolicy(cctx, panel, runtime)
				}
				if err != nil && ctx.Err() == nil {
					log.Printf("user capacity cleanup %s: %v", panel.ID, err)
				}
				return err
			})
		}
		run()
		for {
			supervision.Pulse(ctx)
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	})

	// Eight disjoint serving lanes keep running-router proofs inside the
	// publication freshness window. Retirement has its own single lane.
	// Client mutation execution remains independently gated at concurrency=1.
	for _, serving := range []bool{true, false} {
		lanes := 1
		if serving {
			lanes = 8
		}
		for shard := 0; shard < lanes; shard++ {
			serving, shard, lanes := serving, shard, lanes
			modules.Add(worker.RolePanels, fmt.Sprintf("residential-sync-%t-%d", serving, shard), func(ctx context.Context) {
				t := time.NewTicker(time.Second)
				defer t.Stop()
				syncer := residentialsync.Service{DB: application.DB, Secrets: application.Container.Secrets, Runtimes: sanaeiRuntimes}
				failures := worker.FailureStore{DB: application.DB}
				after := ""
				for {
					supervision.Pulse(ctx)
					p, found, err := syncer.NextDuePanelShardAfter(ctx, serving, shard, lanes, after)
					if err != nil {
						log.Printf("residential sync discovery: %v", err)
					}
					if err == nil && found {
						after = p.ID
						claimCtx, claimCancel := context.WithTimeout(ctx, 3*time.Second)
						claimed, claimErr := failures.ReserveOutcome(claimCtx, "residential_sync", p.ID, "", 90*time.Second)
						claimCancel()
						if claimErr != nil {
							log.Printf("residential sync reservation %s: %v", p.ID, claimErr)
							supervision.Idle(ctx)
							select {
							case <-ctx.Done():
								return
							case <-t.C:
							}
							continue
						}
						if !claimed {
							continue
						}
						panelCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
						err = panelWork.Do(panelCtx, func(ctx context.Context) error { return syncer.ReconcilePanel(ctx, p, false) })
						cancel()
						if persistErr := failures.RecordOutcome(ctx, "residential_sync", p.ID, "", err); persistErr != nil {
							log.Printf("residential sync persistence %s: %v", p.ID, persistErr)
						}
						if err != nil {
							log.Printf("residential sync panel %s: %v", p.ID, err)
						}
						continue
					}
					supervision.Idle(ctx)
					select {
					case <-ctx.Done():
						return
					case <-t.C:
					}
				}
			})
		}
	}

	modules.Add(worker.RolePanels, "trial-relay-health", (residentialsync.Service{DB: application.DB, Secrets: application.Container.Secrets, Runtimes: sanaeiRuntimes}).RunRelayHealth)
	modules.Add(worker.RolePanels, "residential-performance", (residentialperf.Store{DB: application.DB}).Run)
	modules.Add(worker.RoleControl, "observation-retention", application.Container.RunObservationRetention)
	modules.Add(worker.RolePanels, "residential-monitor", func(ctx context.Context) {
		if err := (residential.Monitor{DB: application.DB, Secrets: application.Container.Secrets}).Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("residential monitor stopped")
		}
	})
	monitor := network.Monitor{Economy: application.Container.Economy, DB: application.DB, Secrets: application.Container.Secrets, Interval: 10 * time.Second, Timeout: 8 * time.Second, Policy: network.HealthPolicy{FailureThreshold: 2, RecoveryThreshold: 2, MaxHealthyLatency: 5 * time.Second}}
	modules.Add(worker.RoleControl, "network-monitor", func(ctx context.Context) {
		if err := monitor.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("proxy monitor stopped: %v", err)
		}
	})
	modules.Add(worker.RoleControl, "account-rules", func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			supervision.Pulse(ctx)
			if err := controlWork.Do(ctx, func(ctx context.Context) error { return application.Container.ProcessAccountRuleApplications(ctx) }); err != nil && ctx.Err() == nil {
				log.Printf("account rule application: %v", err)
			}
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	failures := worker.FailureStore{DB: application.DB}
	lw := worker.LifecycleWorker{
		Dispatcher: lifecycleLanes,
		Store:      droplets.LifecycleStore{DB: application.DB}, Handler: application.Container, Admit: controlWork.Do, Failures: failures, Batch: 100,
		Concurrency: 6, ItemTimeout: 90 * time.Second, Interval: 10 * time.Second,
		Progress: func(t time.Time) { lifecycleProgress.Store(t.Unix()) },
	}
	// One bounded lane per account; slow provider work cannot occupy all lanes.
	modules.Add(worker.RoleControl, "lifecycle", func(ctx context.Context) {
		if err := lw.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("lifecycle worker stopped: %v", err)
		}
	})

	w := worker.Worker{Store: worker.RecoveryStore{DB: application.DB}, Handler: app.RecoveryHandler{Container: application.Container}, Admit: controlWork.Do, Failures: failures, Interval: 10 * time.Second, Batch: 100, Progress: func(t time.Time) { recoveryProgress.Store(t.Unix()) }}
	modules.Add(worker.RoleControl, "scheduler", func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		engine := scheduler.Engine{DB: application.DB, Store: scheduler.SQLStore{DB: application.DB}, Leases: scheduler.LeaseStore{DB: application.DB}, Starter: app.ScheduledStarter{Container: application.Container}}
		for {
			supervision.Pulse(ctx)
			if err := controlWork.Do(ctx, func(ctx context.Context) error { return engine.RunDue(ctx, time.Now().UTC()) }); err != nil {
				if ctx.Err() == nil {
					log.Printf("scheduler: %v", err)
				}
			} else {
				schedulerProgress.Store(time.Now().Unix())
			}
			supervision.Idle(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
	modules.Add(worker.RoleControl, "recovery", func(ctx context.Context) {
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("recovery worker stopped: %v", err)
		}
	})
	configureSupervision(&modules)
	return modules
}
