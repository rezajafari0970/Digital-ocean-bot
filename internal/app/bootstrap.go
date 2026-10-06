package app

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/digitalocean"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/upcloud"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"time"
)

type Application struct {
	Config    Config
	DB        *sql.DB
	Container Container
}

func Bootstrap(ctx context.Context) (*Application, error) {
	return bootstrap(ctx, 24)
}

// Worker leases hold connections while bounded network work runs. Keep a
// separate, larger budget so lease holders can still execute SQL. With one
// API and one worker this reserves at most 88 of the default 100 connections.
func BootstrapWorker(ctx context.Context) (*Application, error) {
	return bootstrap(ctx, 64)
}

func bootstrap(ctx context.Context, maxOpen int) (*Application, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	// API and worker have separate pools. Leave headroom within PostgreSQL's
	// connection budget for migrations, operators and bounded advisory leases.
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(8)
	db.SetConnMaxIdleTime(2 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	key, err := secrets.LoadMasterKey()
	if err != nil {
		db.Close()
		return nil, err
	}
	defer wipe(key)
	secretStore, err := secrets.NewStore(secrets.SQLRepository{DB: db}, key, cfg.MasterKeyVersion)
	if err != nil {
		db.Close()
		return nil, err
	}
	registry := providers.NewRegistry()
	if err := registry.Register(digitalocean.Factory{}); err != nil {
		db.Close()
		return nil, err
	}
	if err := registry.Register(vultr.Factory{}); err != nil {
		db.Close()
		return nil, err
	}
	if err := registry.Register(upcloud.Factory{}); err != nil {
		db.Close()
		return nil, err
	}
	container := Container{DB: db, Secrets: secretStore, Accounts: Repository{DB: db}, Providers: registry}
	return &Application{Config: cfg, DB: db, Container: container}, nil
}

func (a *Application) Close() error {
	if a == nil || a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
