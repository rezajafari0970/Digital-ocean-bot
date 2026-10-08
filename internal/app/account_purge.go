package app

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/worker"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrAccountPurgeConflict = errors.New("account purge preconditions changed")

// PurgeSavedAccount is an explicit local-only action. It never claims provider
// deletion. Ordinary Delete retains the durable provider-cleanup workflow.
func PurgeSavedAccount(ctx context.Context, db *sql.DB, id, expectedState string, remaining int) error {
	if !canonicalPurgeAccountID(id) {
		return ErrAccountPurgeConflict
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='5s'"); err != nil {
		return err
	}
	// Same fence as the worker prevents local purge racing cloud verification.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,713))", id); err != nil {
		return err
	}
	if err = lockAccountPurge(ctx, tx, id); err != nil {
		return err
	}
	var state string
	var safe bool
	var live int
	err = tx.QueryRowContext(ctx, `SELECT provider_state,
 NOT enabled AND deletion_requested_at<now()-interval '2 minutes' AND runtime_status='DELETE_PENDING',
 (SELECT count(*) FROM (SELECT provider_resource_id FROM droplets WHERE account_id=a.id AND state<>'DELETED' UNION SELECT provider_resource_id FROM resources WHERE account_id=a.id AND managed AND state<>'deleted') remaining)
 FROM accounts a WHERE id=$1 FOR UPDATE`, id).Scan(&state, &safe, &live)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} // Lost response can safely be replayed.
	if err != nil {
		return err
	}
	if !safe || live != remaining || state != expectedState || !localPurgeAllowed(state) {
		return ErrAccountPurgeConflict
	}
	return commitAccountPurge(ctx, tx, id)
}

func localPurgeAllowed(state string) bool {
	switch state {
	case "LOCKED", "TOKEN_INVALID", "PERMISSION_DENIED", "BILLING_BLOCKED":
		return true
	}
	return false
}
func lockAccountPurge(ctx context.Context, tx *sql.Tx, id string) error {
	if !canonicalPurgeAccountID(id) {
		return ErrAccountPurgeConflict
	}
	if err := worker.FenceRecoveryAccountPurge(ctx, tx, id); err != nil {
		return errors.Join(ErrAccountPurgeConflict, err)
	}
	for _, prefix := range []string{"deployment-admission:", "account-mutation:", "account-proxy:", "account-route:"} {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+id); err != nil {
			return err
		}
	}
	// Serialize with panel structural mutation, including routing and cleanup.
	rows, err := tx.QueryContext(ctx, "SELECT id::text FROM panel_instances WHERE account_id=$1 ORDER BY id", id)
	if err != nil {
		return err
	}
	var panels []string
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		panels = append(panels, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range panels {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "panel-config:"+p); err != nil {
			return err
		}
	}
	return nil
}

// Children without cascading FKs must be deleted before their parents. Scoped
// gates are closed before SET NULL can accidentally widen their scope to fleet.
func commitAccountPurge(ctx context.Context, tx *sql.Tx, id string) error {
	return commitAccountPurgeWithArtifacts(ctx, tx, id, func(canonicalID string) error {
		return removeAccountBrowserSession("/var/lib/digital-ocean-bot/browser-sessions", canonicalID)
	})
}

// The dependency is private: API callers cannot choose a path or cleanup action.
func commitAccountPurgeWithArtifacts(ctx context.Context, tx *sql.Tx, id string, removeArtifacts func(string) error) error {
	if !canonicalPurgeAccountID(id) {
		return ErrAccountPurgeConflict
	}
	if err := worker.FenceRecoveryAccountPurge(ctx, tx, id); err != nil {
		return errors.Join(ErrAccountPurgeConflict, err)
	}
	// Lock the parent before deleting telemetry. Late meter flushes take KEY SHARE
	// on this same row, so neither can recreate data behind the other's commit.
	var canonicalID string
	if err := tx.QueryRowContext(ctx, "SELECT id::text FROM accounts WHERE id=$1 FOR UPDATE", id).Scan(&canonicalID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tx.Commit()
		}
		return err
	}
	id = canonicalID
	queries := []string{
		`DELETE FROM worker_recovery_checkpoints WHERE account_id=$1`,
		`DELETE FROM proxy_traffic_hourly WHERE role='account' AND (lower(owner_id)=$1::uuid::text OR lower(owner_id)='replace-'||$1::uuid::text)`,
		`UPDATE proxy_economy_policy SET enabled=false,canary_account_id=NULL,updated_at=now() WHERE canary_account_id=$1`,
		`UPDATE server_protection_control SET panel_ids=ARRAY(SELECT x FROM unnest(panel_ids) x WHERE x NOT IN(SELECT id FROM panel_instances WHERE account_id=$1)),revision=revision+1,updated_at=now() WHERE panel_ids && ARRAY(SELECT id FROM panel_instances WHERE account_id=$1)`,
		`UPDATE residential_routing_control SET panel_ids=ARRAY(SELECT x FROM unnest(panel_ids) x WHERE x NOT IN(SELECT id FROM panel_instances WHERE account_id=$1)) WHERE panel_ids && ARRAY(SELECT id FROM panel_instances WHERE account_id=$1)`,
		`UPDATE bulk_client_execution_gate SET enabled=false,kill_switch=true,panel_id=NULL,inbound_id=NULL,remaining_batches=0,updated_at=now() WHERE panel_id IN(SELECT id FROM panel_instances WHERE account_id=$1)`,

		`UPDATE client_mutation_execution_gate SET enabled=false,kill_switch=true,panel_id=NULL,inbound_id=NULL,updated_at=now() WHERE panel_id IN(SELECT id FROM panel_instances WHERE account_id=$1)`,
		`UPDATE bulk_user_shrink_gate SET enabled=false,panel_id=NULL,inbound_id=NULL,updated_at=now() WHERE panel_id IN(SELECT id FROM panel_instances WHERE account_id=$1)`,
		`DELETE FROM bulk_lifecycle_scopes WHERE panel_id IN(SELECT id FROM panel_instances WHERE account_id=$1)`,
		`DELETE FROM bulk_scale_runs WHERE generation_id IN(SELECT g.id FROM bulk_user_generations g JOIN panel_instances p ON p.id=g.panel_id WHERE p.account_id=$1)`,
		`DELETE FROM bulk_user_ownership WHERE generation_id IN(SELECT g.id FROM bulk_user_generations g JOIN panel_instances p ON p.id=g.panel_id WHERE p.account_id=$1)`,
		`DELETE FROM worker_item_failures WHERE account_id=$1 OR (account_id IS NULL AND (item_id=$1::uuid::text OR item_id IN (SELECT id::text FROM panel_instances WHERE account_id=$1 UNION SELECT id::text FROM droplets WHERE account_id=$1 UNION SELECT id::text FROM deployments WHERE account_id=$1 UNION SELECT id::text FROM operations WHERE account_id=$1)))`,
		`DELETE FROM audit_events WHERE account_id=$1 OR (account_id IS NULL AND resource_type IN ('account','deployment','droplet','panel','operation','server') AND (resource_id=$1::uuid::text OR resource_id IN(SELECT id::text FROM panel_instances WHERE account_id=$1 UNION SELECT id::text FROM droplets WHERE account_id=$1 UNION SELECT id::text FROM deployments WHERE account_id=$1 UNION SELECT id::text FROM operations WHERE account_id=$1)))`,
		`DELETE FROM secrets WHERE account_id=$1`,
		`DELETE FROM lifecycle_events WHERE account_id=$1`,
		`DELETE FROM provider_snapshots WHERE account_id=$1`,
		`DELETE FROM resources WHERE account_id=$1`,
		`DELETE FROM deployments WHERE account_id=$1`,
		`DELETE FROM droplets WHERE account_id=$1`,
		`DELETE FROM accounts WHERE id=$1`,
	}
	for i, q := range queries {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("account purge step %d: %w", i+1, err)
		}
	}
	// Retired browser credentials must be gone before reporting PURGED. A failure
	// rolls back the DB rows. Filesystem removal itself is not transactional: if
	// the later DB commit fails, the account remains but this cache may be gone.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := removeArtifacts(id); err != nil {
		return fmt.Errorf("account browser artifacts: %w", err)
	}
	return tx.Commit()
}

// Canonical IDs are required before all string-derived recovery/advisory keys.
func canonicalPurgeAccountID(id string) bool {
	if len(id) != 36 || id != strings.ToLower(id) || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil && len(raw) == 16
}

// Linux production: anchor an O_NOFOLLOW directory descriptor, then give os.Root
// its own descriptor through procfs. Renaming/replacing the path cannot redirect
// a subsequent removal outside this opened root. Missing procfs fails closed.
func openAccountBrowserRoot(root string) (*os.Root, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("invalid browser session root")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	return os.OpenRoot("/proc/self/fd/" + strconv.Itoa(fd))
}
func removeAccountBrowserSession(root, id string) error {
	if !canonicalPurgeAccountID(id) {
		return errors.New("invalid canonical account ID")
	}
	anchored, err := openAccountBrowserRoot(root)
	if err != nil || anchored == nil {
		return err
	}
	defer anchored.Close()
	return anchored.RemoveAll(id)
}
