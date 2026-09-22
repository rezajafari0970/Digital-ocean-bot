package app

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

type Application struct {
	Config    Config
	DB        *sql.DB
	Container Container
}

func Bootstrap(ctx context.Context) (*Application, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
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
	container := Container{DB: db, Secrets: secretStore, Accounts: Repository{DB: db}}
	return &Application{Config: cfg, DB: db, Container: container}, nil
}

func (a *Application) Close() error {
	if a == nil || a.DB == nil {
		return nil
	}
	return a.DB.Close()
}
