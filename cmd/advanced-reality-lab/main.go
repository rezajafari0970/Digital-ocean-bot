// advanced-reality-lab operates only on a private snapshot directory. No DB,
// panel API, SSH, production configuration or service manager is contacted.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/advancedreality"
)

func read(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("config_read_failed")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, advancedreality.MaxConfigBytes+1))
	if e != nil || len(b) > advancedreality.MaxConfigBytes {
		return nil, fmt.Errorf("config_read_failed")
	}
	return b, nil
}
func run() error {
	action := flag.String("action", "status", "init, inspect, stage, status, rollback, remove, export, inventory")
	dir := flag.String("lab", "", "absolute private lab directory; init requires it not to exist")
	base := flag.String("baseline", "", "baseline JSON file")
	candidate := flag.String("candidate", "", "candidate JSON file")
	core := flag.String("core", "", "exact server core version")
	expected := flag.String("expected-sha256", "", "current lab hash from prior receipt")
	key := flag.String("request-key", "", "8..64 character idempotency key")
	output := flag.String("output", "", "new private export file; never stdout")
	flag.Parse()
	l := advancedreality.Lab{Dir: *dir}
	var result any
	var e error
	switch *action {
	case "init":
		var b []byte
		b, e = read(*base)
		if e == nil {
			result, e = l.Init(b, *core)
		}
	case "inspect":
		var b, c []byte
		b, e = read(*base)
		if e == nil {
			c, e = read(*candidate)
		}
		if e == nil {
			result, e = advancedreality.Inspect(b, c, *core)
		}
	case "stage":
		var b []byte
		b, e = read(*candidate)
		if e == nil {
			result, e = l.Stage(b, *expected, *key)
		}
	case "status":
		result, e = l.Status()
	case "rollback":
		result, e = l.Rollback(*expected, *key)
	case "remove":
		e = l.Remove(*expected)
		result = map[string]bool{"removed": e == nil, "production_changed": false}
	case "export":
		var raw []byte
		raw, e = l.CurrentConfig()
		if e == nil {
			var f *os.File
			f, e = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e == nil {
				_, e = f.Write(raw)
				if e == nil {
					e = f.Sync()
				}
				ce := f.Close()
				if e == nil {
					e = ce
				}
			}
			if e != nil {
				e = fmt.Errorf("private_export_failed")
			}
		}
		result = map[string]bool{"exported": e == nil, "production_changed": false}
	case "inventory":
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			return fmt.Errorf("database_environment_required")
		}
		var db *sql.DB
		db, e = sql.Open("postgres", dsn)
		if e != nil {
			return fmt.Errorf("inventory_unavailable")
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		result, e = advancedreality.Inventory(ctx, db)
		if e != nil {
			e = fmt.Errorf("inventory_unavailable")
		}
	default:
		e = fmt.Errorf("unknown_action")
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	if e := run(); e != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": e.Error()})
		os.Exit(1)
	}
}
