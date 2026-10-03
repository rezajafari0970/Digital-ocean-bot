package main

import (
	"context"
	"log"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	application, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	runtimes := &sanaei.RuntimeManager{
		Factory: sanaei.RuntimeFactory{DB: application.DB, Secrets: application.Container.Secrets, Timeout: 8 * time.Second},
		TTL:     5 * time.Second,
	}
	exec := clientops.Executor{
		Journal:  clientops.Journal{DB: application.DB},
		Runtimes: runtimes,
		Timeout:  30 * time.Second,
	}
	ok, err := exec.RunOne(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		log.Print("no eligible client mutation job")
	}
}
