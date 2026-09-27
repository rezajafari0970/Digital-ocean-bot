package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/desired"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/panelbootstrap"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func main() {
	ctx := context.Background()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}
	defer a.Close()
	const managedKey = "dob:reality-primary:000001"
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: a.Container.DB}}
	adapter := readyworker.ProductionAdapter{
		Eligibility:  readyworker.SQLSource{DB: a.Container.DB},
		Bootstrapper: panelbootstrap.Service{DB: a.Container.DB, Secrets: a.Container.Secrets, SSH: ssh, ManagedKey: managedKey},
		Desired:      desired.Service{DB: a.Container.DB, Secrets: a.Container.Secrets, ManagedKey: managedKey, Port: 443},
	}
	worker := readyworker.Worker{Adapter: adapter, Concurrency: 2, DryRun: true}
	for cycle := 1; cycle <= 3; cycle++ {
		results, err := worker.Run(ctx)
		if err != nil {
			panic(err)
		}
		boot, reconciled, failed := 0, 0, 0
		fmt.Printf("CYCLE=%d ELIGIBLE=%d\n", cycle, len(results))
		for _, r := range results {
			if r.Bootstrapped {
				boot++
			}
			if r.Reconciled {
				reconciled++
			}
			if r.Err != nil {
				failed++
				fmt.Printf("PANEL=%s STATUS=WARMING_OR_FAILED ERR=%v\n", r.PanelID, r.Err)
			}
		}
		fmt.Printf("SUMMARY cycle=%d bootstrapped=%d reconciled=%d failed=%d mutated=0\n", cycle, boot, reconciled, failed)
		if cycle < 3 {
			time.Sleep(2 * time.Second)
		}
	}
}
