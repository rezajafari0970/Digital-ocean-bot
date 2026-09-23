package main

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/observability"
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
	metrics := observability.NewMetrics()
	health := observability.Health{DB: application.DB}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "digital-ocean-bot-api"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		report := health.Readiness(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if report.Status != "ready" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metrics.WritePrometheus(w)
	})
	server := &http.Server{Addr: application.Config.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("api listening on %s", application.Config.HTTPAddr)
	log.Fatal(server.ListenAndServe())
}
