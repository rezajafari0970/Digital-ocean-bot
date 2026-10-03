package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

type sshKeyInventory interface {
	ListSSHKeys(context.Context) ([]providers.SSHKey, error)
}

// ProcessAccountDeletions is bounded and worker-only. Requests persist until
// provider absence is established; transport/auth failures never erase credentials.
func (c Container) ProcessAccountDeletions(ctx context.Context) error {
	var id string
	err := c.DB.QueryRowContext(ctx, `SELECT account_id::text FROM account_deletion_jobs WHERE next_attempt_at<=now() ORDER BY requested_at LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	conn, err := c.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,713))", id).Scan(&locked); err != nil || !locked {
		return err
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(unlock, "SELECT pg_advisory_unlock(hashtextextended($1,713))", id); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	_, err = c.DB.ExecContext(ctx, `UPDATE account_deletion_jobs SET attempts=attempts+1,next_attempt_at=now()+interval '1 minute' WHERE account_id=$1`, id)
	if err != nil {
		return err
	}
	err = c.finishAccountDeletion(ctx, id)
	if err != nil {
		// Error codes shown in the UI contain no provider credentials or URLs.
		detail := "Provider cleanup pending; " + deletionErrorCode(err)
		_, _ = c.DB.ExecContext(ctx, `UPDATE account_deletion_jobs SET last_error=$2 WHERE account_id=$1`, id, detail)
		_, _ = c.DB.ExecContext(ctx, `UPDATE accounts SET runtime_status='DELETE_PENDING',runtime_status_detail=$2,deleted_at=NULL WHERE id=$1 AND deletion_requested_at IS NOT NULL`, id, detail)
	}
	return err
}
func deletionErrorCode(err error) string {
	if errors.Is(err, ErrNetworkNotReady) {
		return "account network not ready"
	}
	if class := providers.Class(err); class != "" {
		return string(class)
	}
	return "verification or resource cleanup incomplete"
}
func (c Container) finishAccountDeletion(ctx context.Context, id string) error {
	var pending, ever bool
	err := c.DB.QueryRowContext(ctx, `SELECT
 EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state<>'DELETED')
 OR EXISTS(SELECT 1 FROM resources WHERE account_id=$1 AND managed AND state<>'deleted')
 OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND state NOT IN ('succeeded','failed'))
 OR EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND state NOT IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','READY')),
 EXISTS(SELECT 1 FROM droplets WHERE account_id=$1) OR EXISTS(SELECT 1 FROM resources WHERE account_id=$1) OR EXISTS(SELECT 1 FROM deployments WHERE account_id=$1) OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1)`, id).Scan(&pending, &ever)
	if err != nil {
		return err
	}
	if pending {
		return errors.New("managed resources or operations pending")
	}
	if ever {
		var settled bool
		if err = c.DB.QueryRowContext(ctx, "SELECT requested_at<now()-interval '2 minutes' FROM account_deletion_jobs WHERE account_id=$1", id).Scan(&settled); err != nil {
			return err
		}
		if !settled {
			return errors.New("waiting for in-flight account work to settle")
		}

		rt, err := c.CleanupRuntime(ctx, id)
		if err != nil {
			return err
		}
		compute, err := computeDriver(rt)
		if err != nil {
			return err
		}
		servers, err := compute.ListServers(ctx)
		if err != nil {
			return err
		}
		if err = c.recoverDeletingInventory(ctx, id, servers); err != nil {
			return err
		}
		ids := make([]string, 0, len(servers))
		for _, server := range servers {
			if server.ID == "" {
				return errors.New("invalid provider inventory")
			}
			ids = append(ids, server.ID)
		}
		var exists bool
		err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=ANY($2::text[])) OR EXISTS(SELECT 1 FROM resources WHERE account_id=$1 AND managed AND provider_resource_id=ANY($2::text[])) OR EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND resource_id=ANY($2::text[])) OR EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND provider_id=ANY($2::text[]))`, id, pq.Array(ids)).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("tracked provider resource still exists")
		}
		if err = c.cleanupDeletionKeys(ctx, id, rt); err != nil {
			return err
		}
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout='5s'"); err != nil {
		return err
	}
	for _, prefix := range []string{"deployment-admission:", "account-mutation:", "account-proxy:", "account-route:"} {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", prefix+id); err != nil {
			return err
		}
	}
	// A last fresh local read prevents a concurrent lifecycle/recovery commit
	// from being erased. Account disablement is retained until this transaction.
	var safe bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN account_deletion_jobs j ON j.account_id=a.id WHERE a.id=$1 AND NOT a.enabled AND a.deletion_requested_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state<>'DELETED')
 AND NOT EXISTS(SELECT 1 FROM resources WHERE account_id=$1 AND managed AND state<>'deleted')
 AND NOT EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND state NOT IN ('succeeded','failed'))
 AND NOT EXISTS(SELECT 1 FROM deployments WHERE account_id=$1 AND state NOT IN ('FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE','READY')) AND NOT EXISTS(SELECT 1 FROM account_deletion_keys WHERE account_id=$1 AND state<>'SUCCEEDED')`, id).Scan(&safe)
	if err != nil {
		return err
	}
	if !safe {
		return errors.New("account deletion precondition changed")
	}
	// All account-related non-cascading tables and unlinked worker diagnostics are
	// removed in the same transaction. Remaining children use verified FK cascades.
	for _, q := range []string{
		`DELETE FROM worker_item_failures WHERE account_id=$1 OR item_id IN (SELECT id::text FROM panel_instances WHERE account_id=$1 UNION SELECT id::text FROM droplets WHERE account_id=$1 UNION SELECT id::text FROM deployments WHERE account_id=$1 UNION SELECT id::text FROM operations WHERE account_id=$1)`,
		"DELETE FROM audit_events WHERE account_id=$1",
		"DELETE FROM secrets WHERE account_id=$1",
		"DELETE FROM lifecycle_events WHERE account_id=$1",
		"DELETE FROM provider_snapshots WHERE account_id=$1",
		"DELETE FROM resources WHERE account_id=$1",
		"DELETE FROM droplets WHERE account_id=$1",
		"DELETE FROM accounts WHERE id=$1",
	} {
		if _, err = tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (c Container) cleanupDeletionKeys(ctx context.Context, id string, rt AccountRuntime) error {
	reader, ok := rt.Driver.(sshKeyInventory)
	if !ok {
		return errors.New("SSH inventory unavailable")
	}
	deleter, ok := rt.Driver.(providers.SSHKeyDriver)
	if !ok {
		return errors.New("SSH delete unavailable")
	}
	keys, err := reader.ListSSHKeys(ctx)
	if err != nil {
		return err
	}
	rows, err := c.DB.QueryContext(ctx, "SELECT id::text,COALESCE(profile_snapshot->>'ssh_provider_key_id','') FROM deployments WHERE account_id=$1", id)
	if err != nil {
		return err
	}
	owned := map[string]bool{}
	recorded := map[string]bool{}
	for rows.Next() {
		var dep, keyID string
		if err = rows.Scan(&dep, &keyID); err != nil {
			rows.Close()
			return err
		}
		owned["dob-"+dep] = true
		if keyID != "" {
			recorded[keyID] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, key := range keys {
		if key.ID == "" {
			return errors.New("invalid SSH inventory")
		}
		present[key.ID] = true
		if owned[key.Name] && !recorded[key.ID] {
			return errors.New("unrecorded provider SSH key requires ownership reconciliation")
		}
		if recorded[key.ID] {
			if _, err = c.DB.ExecContext(ctx, `INSERT INTO account_deletion_keys(account_id,provider_key_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, key.ID); err != nil {
				return err
			}
		}
	}
	rows, err = c.DB.QueryContext(ctx, `SELECT provider_key_id FROM account_deletion_keys WHERE account_id=$1 AND state<>'SUCCEEDED' ORDER BY provider_key_id LIMIT 10`, id)
	if err != nil {
		return err
	}
	work := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		work = append(work, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range work {
		if present[key] {
			if err = rt.Gate.AllowMutation(); err != nil {
				return err
			}
			if err = rt.CheckMutationGeneration(ctx); err != nil {
				return err
			}
			if _, err = c.DB.ExecContext(ctx, `UPDATE account_deletion_keys SET state='RUNNING',attempts=attempts+1,updated_at=now() WHERE account_id=$1 AND provider_key_id=$2`, id, key); err != nil {
				return err
			}
			deleteErr := deleter.DeleteSSHKey(ctx, key)
			fresh, e := reader.ListSSHKeys(ctx)
			if e != nil {
				return fmt.Errorf("SSH delete outcome unknown: %w", e)
			}
			for _, k := range fresh {
				if k.ID == key {
					if deleteErr != nil {
						return deleteErr
					}
					return errors.New("SSH key still present")
				}
			}
		}
		if _, err = c.DB.ExecContext(ctx, `UPDATE account_deletion_keys SET state='SUCCEEDED',updated_at=now() WHERE account_id=$1 AND provider_key_id=$2`, id, key); err != nil {
			return err
		}
	}
	var remaining bool
	err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_deletion_keys WHERE account_id=$1 AND state<>'SUCCEEDED')`, id).Scan(&remaining)
	if err != nil {
		return err
	}
	if remaining {
		return errors.New("SSH cleanup continues next chunk")
	}
	return nil
}

// A create response can be lost before a droplet row is saved. Provider identity
// recovery supplies the authoritative ID; persist it for the existing DELETE worker.
func (c Container) adoptDeletedAccountServer(ctx context.Context, account, id string) error {
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "deployment-admission:"+account); err != nil {
		return err
	}
	var allowed bool
	if err = tx.QueryRowContext(ctx, "SELECT NOT enabled AND deletion_requested_at IS NOT NULL FROM accounts WHERE id=$1 FOR UPDATE", account).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return errors.New("account not deleting")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO droplets(id,account_id,provider_resource_id,state) SELECT gen_random_uuid(),$1,$2,'RETIRING' WHERE NOT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=$2)`, account, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE droplets SET state='RETIRING',updated_at=now() WHERE account_id=$1 AND provider_resource_id=$2 AND state NOT IN ('RETIRING','DELETING')`, account, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Resolve a lost CREATE response by the same persisted deployment identity used
// by normal recovery. Names alone never authorize a deletion.
func (c Container) recoverDeletingInventory(ctx context.Context, account string, servers []providers.Server) error {
	rows, err := c.DB.QueryContext(ctx, "SELECT id::text,COALESCE(provider_id,'') FROM deployments WHERE account_id=$1", account)
	if err != nil {
		return err
	}
	deps := map[string]string{}
	for rows.Next() {
		var id, pid string
		if err = rows.Scan(&id, &pid); err != nil {
			rows.Close()
			return err
		}
		deps["dob-deployment-"+id] = pid
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	matched := map[string][]string{}
	for _, server := range servers {
		for _, tag := range server.Tags {
			if _, ok := deps[tag]; ok {
				matched[tag] = append(matched[tag], server.ID)
			}
		}
	}
	for tag, ids := range matched {
		if len(ids) != 1 {
			return errors.New("ambiguous deployment identity in provider inventory")
		}
		if deps[tag] != "" && deps[tag] != ids[0] {
			return errors.New("provider identity conflicts with recorded server")
		}
		if deps[tag] == "" {
			var recorded bool
			if err = c.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND provider_resource_id=$2)", account, ids[0]).Scan(&recorded); err != nil {
				return err
			}
			if recorded {
				return errors.New("tracked recovered resource still appears in provider inventory")
			}
			if err = c.adoptDeletedAccountServer(ctx, account, ids[0]); err != nil {
				return err
			}
			return errors.New("recovered provider server queued for deletion")
		}
	}
	return nil
}
