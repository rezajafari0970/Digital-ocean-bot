package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/usercapacity"
)

func main() {
	var panelID string
	var dryRun, trace bool
	flag.StringVar(&panelID, "panel", "", "panel uuid")
	flag.BoolVar(&dryRun, "dry-run", false, "show owned shrink candidates without mutation")
	flag.BoolVar(&trace, "trace-shrink", false, "show shrink decision without mutation")
	flag.Parse()
	if panelID == "" {
		log.Fatal("panel required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	panels, err := (readyworker.SQLSource{DB: a.DB}).EligibleReadyPanels(ctx)
	if err != nil {
		log.Fatal(err)
	}
	var target *readyworker.Panel
	for i := range panels {
		if panels[i].ID == panelID {
			target = &panels[i]
			break
		}
	}
	if target == nil {
		log.Fatal("panel not eligible")
	}
	runtimes := &sanaei.RuntimeManager{
		Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 60 * time.Second},
		TTL:     5 * time.Second,
	}
	rt, err := runtimes.Acquire(ctx, panelID)
	if err != nil {
		log.Fatal(err)
	}
	s := usercapacity.Service{DB: a.DB, Secrets: a.Container.Secrets}
	if trace {
		var target, rate int
		if e := a.DB.QueryRowContext(ctx, `SELECT target_users_per_inbound,users_per_second FROM global_config_policies WHERE policy_key='reality'`).Scan(&target, &rate); e != nil {
			log.Fatal(e)
		}
		decision, e := s.ShrinkDecisionDryRun(ctx, panelID, 1, rt, target, rate)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Printf("SHRINK_TRACE gate=%v limit=%d effective_target=%d pending=%v candidates=%v\n", decision.Gate, decision.Limit, decision.EffectiveTarget, decision.Pending, decision.Candidates)
		return
	}
	if dryRun {
		ids, e := s.ShrinkDryRunRuntime(ctx, panelID, 1, rt, 1, 1)
		if e != nil {
			log.Fatal(e)
		}
		fmt.Printf("SHRINK_DRY_RUN candidates=%d ids=%v\n", len(ids), ids)
		return
	}
	if err = s.ReconcileRuntimeFromPolicy(ctx, *target, rt); err != nil {
		log.Fatal(err)
	}
	fmt.Println("USER_CAPACITY_RECONCILE_OK", panelID)
}
