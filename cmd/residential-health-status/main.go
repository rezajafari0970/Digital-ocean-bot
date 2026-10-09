// residential-health-status reads evidence without generating network probes or changing routing.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	panel := flag.String("panel", "", "panel UUID for a read-only diagnostic snapshot")
	flag.Parse()
	if err := run(*panel); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(panel string) error {
	if !residentialperf.UUID.MatchString(panel) {
		return errors.New("panel UUID required")
	}
	parent, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("service bootstrap unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	view, err := (residentialperf.Store{DB: db}).HealthSnapshot(ctx, panel)
	if err != nil {
		return errors.New("current panel context or diagnostic evidence unavailable")
	}
	return json.NewEncoder(os.Stdout).Encode(view)
}
