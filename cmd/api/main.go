package main

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/adminapi"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"log"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	application, err := app.Bootstrap(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer application.Close()
	if err := (migrate.Runner{DB: application.DB, Dir: "migrations"}).Up(ctx); err != nil {
		log.Fatal(err)
	}
	settings, err := application.PanelSettings(ctx)
	if err != nil {
		log.Fatal(err)
	}
	api := adminapi.New(application.DB, application.Container)
	api.WebPath = settings.WebPath
	server := &http.Server{Addr: settings.Addr(), Handler: adminapi.Secure(api.Routes()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("api listening on %s", application.Config.HTTPAddr)
	log.Fatal(server.ListenAndServe())
}
