package main

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/adminapi"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	application, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer application.Close()
	if err := (migrate.Runner{DB: application.DB, Dir: "migrations"}).Up(ctx); err != nil {
		return err
	}
	settings, err := application.PanelSettings(ctx)
	if err != nil {
		return err
	}
	api := adminapi.New(application.DB, application.Container)
	api.WebPath = settings.WebPath
	go api.WarmOutputCache(ctx)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				api.WarmOutputCache(ctx)
			}
		}
	}()
	server := &http.Server{Addr: settings.Addr(), Handler: adminapi.Secure(api.Routes()), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("api listening on %s", application.Config.HTTPAddr)
	listenErr := make(chan error, 1)
	go func() { listenErr <- server.ListenAndServe() }()
	select {
	case err := <-listenErr:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			log.Printf("api shutdown deadline reached")
		}
	}
	// Application.Close flushes final socket counters after HTTP work drains.
	return nil

}
