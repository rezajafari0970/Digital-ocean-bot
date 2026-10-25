// Synthetic fixture for the disposable backup/restore drill; never a live DB.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/migrate"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

const owner = "00000000-0000-4000-8000-000000000011"
const profile = "00000000-0000-4000-8000-000000000012"
const deployment = "00000000-0000-4000-8000-000000000013"

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "seed" && os.Args[1] != "verify") {
		return errors.New("seed or verify required")
	}
	dsn := os.Getenv("RESTORE_DRILL_DSN")
	u, err := url.Parse(dsn)
	if err != nil {
		return errors.New("private drill DSN required")
	}
	socket := u.Query().Get("host")
	if os.Getenv("DOB_DISPOSABLE_RESTORE_DRILL") != "1" || u.Host != "" || u.User == nil || u.User.Username() != "dob_drill" ||
		(u.Path != "/dob_restore_source" && u.Path != "/dob_restore_target") ||
		!filepath.IsAbs(socket) || filepath.Base(socket) != "socket" || !strings.HasPrefix(filepath.Base(filepath.Dir(socket)), "dob-restore-drill-") {
		return errors.New("disposable private socket database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = (migrate.Runner{DB: db, Dir: "migrations"}).Up(ctx); err != nil {
		return err
	}
	keyPath := os.Getenv("RESTORE_DRILL_KEY_FILE")
	if filepath.Dir(filepath.Dir(keyPath)) != filepath.Dir(socket) || filepath.Base(keyPath) != "master.key" {
		return errors.New("private fixture key path required")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		return errors.New("fixture encryption identity unavailable")
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	store, err := secrets.NewStore(secrets.SQLRepository{DB: db}, key, 1)
	if err != nil {
		return err
	}
	if os.Args[1] == "seed" {
		for _, query := range []string{
			"INSERT INTO accounts(id,provider,name,secret_ref) VALUES('" + owner + "','vultr','restore-fixture','fixture-provider')",
			"INSERT INTO deployment_profiles(id,account_id,name,config) VALUES('" + profile + "','" + owner + "','restore-fixture','{}')",
			"INSERT INTO deployments(id,account_id,profile_id,state,current_step,profile_snapshot) VALUES('" + deployment + "','" + owner + "','" + profile + "','PLANNED','create','{\"ssh_key_secret_ref\":\"fixture-ssh\",\"ssh_key_intent\":{\"state\":\"unknown\"},\"future_field\":\"preserve\"}')",
			"INSERT INTO deployment_events(deployment_id,step,state,message) VALUES('" + deployment + "','create','PLANNED','fixture-before-backup')",
		} {
			if _, err = db.ExecContext(ctx, query); err != nil {
				return err
			}
		}
		if err = store.Put(ctx, owner, "fixture-provider", "provider", []byte("synthetic-provider-value")); err != nil {
			return err
		}
		if err = store.Put(ctx, owner, "fixture-ssh", "ssh_private_key", []byte("synthetic-private-key-value")); err != nil {
			return err
		}
	}
	plain, err := store.Get(ctx, owner, "fixture-ssh")
	if err != nil {
		return err
	}
	if !bytes.Equal(plain, []byte("synthetic-private-key-value")) {
		return errors.New("restored encrypted identity changed")
	}
	for i := range plain {
		plain[i] = 0
	}
	var raw []byte
	if err = db.QueryRowContext(ctx, `SELECT jsonb_build_object(
 'account',(SELECT to_jsonb(a) FROM accounts a WHERE id=$1),
 'deployment',(SELECT to_jsonb(d) FROM deployments d WHERE id=$2),
 'secrets',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM secrets s WHERE account_id=$1),
 'migrations',(SELECT jsonb_agg(jsonb_build_object('version',version,'checksum',checksum) ORDER BY version) FROM schema_migrations))`, owner, deployment).Scan(&raw); err != nil {
		return err
	}
	var normalized any
	if err = json.Unmarshal(raw, &normalized); err != nil {
		return err
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	var count int
	var version, state string
	if err = db.QueryRowContext(ctx, "SELECT count(*),max(version) FROM schema_migrations").Scan(&count, &version); err != nil {
		return err
	}
	if err = db.QueryRowContext(ctx, "SELECT profile_snapshot->'ssh_key_intent'->>'state' FROM deployments WHERE id=$1", deployment).Scan(&state); err != nil {
		return err
	}
	if state != "unknown" {
		return errors.New("unknown intent changed during restore")
	}
	sequence := false
	if os.Args[1] == "verify" {
		var previous, next int64
		if err = db.QueryRowContext(ctx, "SELECT max(id) FROM deployment_events").Scan(&previous); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, "INSERT INTO deployment_events(deployment_id,step,state,message) VALUES($1,'create','PLANNED','fixture-after-restore') RETURNING id", deployment).Scan(&next); err != nil {
			return err
		}
		if next <= previous {
			return errors.New("restored event sequence regressed")
		}
		sequence = true
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"digest": hex.EncodeToString(sum[:]), "migrations": count, "schema": version, "encrypted_identity_verified": true, "unknown_intent_preserved": true, "sequence_verified": sequence})
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
