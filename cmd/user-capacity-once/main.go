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
	flag.StringVar(&panelID, "panel", "", "panel uuid")
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
	f := sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 60 * time.Second}
	rt, err := f.Open(ctx, panelID)
	if err != nil {
		log.Fatal(err)
	}
	s := usercapacity.Service{DB: a.DB, Secrets: a.Container.Secrets}
	if err = s.ReconcileRuntimeFromPolicy(ctx, *target, rt); err != nil {
		log.Fatal(err)
	}
	fmt.Println("USER_CAPACITY_RECONCILE_OK", panelID)
}
