package main

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"os"
)

const accountID = "188a2610-2482-4be6-88f2-36c880877705"
const serverID = "602967125"

func main() {
	ctx := context.Background()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		panic(err)
	}
	defer a.Close()
	rt, err := a.Container.Runtime(ctx, accountID)
	if err != nil {
		panic(err)
	}
	compute, ok := rt.Driver.(providers.ComputeDriver)
	if !ok {
		panic("provider compute capability unavailable")
	}
	srv, err := compute.GetServer(ctx, serverID)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Found test server: id=%s name=%s region=%s state=%s\n", srv.ID, srv.Name, srv.RegionID, srv.State)
	if srv.Name != "dob-debug-invalid" {
		fmt.Println("ABORT: server name does not match test resource")
		os.Exit(2)
	}
	if err := compute.DeleteServer(ctx, serverID); err != nil {
		panic(err)
	}
	fmt.Printf("Delete request accepted for test server %s\n", serverID)
}
