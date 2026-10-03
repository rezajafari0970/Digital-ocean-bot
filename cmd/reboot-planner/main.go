package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/rollingreboot"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	s := rollingreboot.Service{DB: a.DB, Enabled: false, MinReadyPerAccount: 2}
	n, err := s.Plan(ctx)
	if err != nil {
		log.Fatal(err)
	}
	cs, err := s.DryRun(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("planned_new=%d pending=%d mode=dry-run\n", n, len(cs))
	for i, c := range cs {
		fmt.Printf("%02d account=%s host=%s ready=%d panel=%s\n", i+1, c.Account, c.Host, c.Ready, c.PanelID)
	}
}
