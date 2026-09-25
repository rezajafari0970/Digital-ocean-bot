package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
)

const (
	accountID = "188a2610-2482-4be6-88f2-36c880877705"
	dropletID = 602967125
)

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

	// Safety check: never delete a different droplet by mistake.
	d, err := rt.Provider.GetDroplet(ctx, dropletID)
	if err != nil {
		panic(err)
	}

	fmt.Printf(
		"Found test droplet: id=%d name=%s region=%s status=%s\n",
		d.ID,
		d.Name,
		d.Region.Slug,
		d.Status,
	)

	if d.Name != "dob-debug-invalid" {
		fmt.Println("ABORT: droplet name does not match test resource")
		os.Exit(2)
	}

	if err := rt.Provider.DeleteDroplet(ctx, dropletID); err != nil {
		panic(err)
	}

	fmt.Printf("Delete request accepted for test droplet %d\n", dropletID)
}
