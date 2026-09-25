package main

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
)

func main() {
	ctx := context.Background()
	a, e := app.Bootstrap(ctx)
	if e != nil {
		panic(e)
	}
	defer a.Close()
	e = (app.RecoveryHandler{Container: a.Container}).RecoverDeployment(ctx, worker.RecoveryItem{ID: "14f02b13-98d8-4e3e-b224-843d09385184", AccountID: "188a2610-2482-4be6-88f2-36c880877705", Kind: "deployment"})
	fmt.Printf("RESULT=%v\n", e)
}
