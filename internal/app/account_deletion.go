package app

import (
	"context"
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
	// Also bound callers outside the periodic worker. Earlier deadlines win.
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	id, release, err := c.claimAccountDeletion(ctx)
	if err != nil || id == "" {
		return err
	}
	defer release()
	err = c.finishAccountDeletion(ctx, id)
	if err != nil {
		// A timed-out provider call must still leave a current, bounded status.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		// Error codes shown in the UI contain no provider credentials or URLs.
		detail := "Provider cleanup pending; " + deletionErrorCode(err)
		var state string
		if e := c.DB.QueryRowContext(persist, "SELECT provider_state FROM accounts WHERE id=$1", id).Scan(&state); e == nil {
			switch state {
			case "TOKEN_INVALID":
				detail = "Deletion blocked: provider token is invalid. Update the API credential to verify and delete remaining servers."
			case "LOCKED":
				detail = "Deletion blocked: provider account is locked. Restore provider access before remaining servers can be verified."
			case "PERMISSION_DENIED":
				detail = "Deletion blocked: provider denied access. Restore read/delete permissions."
			case "BILLING_BLOCKED":
				detail = "Deletion blocked: provider billing restriction prevents resource cleanup."
			}
		}
		tx, saveErr := c.DB.BeginTx(persist, nil)
		if saveErr != nil {
			return errors.Join(err, fmt.Errorf("persist deletion status: %w", saveErr))
		}
		defer tx.Rollback()
		_, saveErr = tx.ExecContext(persist, `UPDATE account_deletion_jobs SET last_error=$2,next_attempt_at=CASE WHEN $4 THEN GREATEST(now(),requested_at+interval '2 minutes') ELSE now()+($3*interval '1 second') END WHERE account_id=$1`, id, detail, deletionRetrySeconds(state), errors.Is(err, errDeletionSettling))
		if saveErr == nil {
			_, saveErr = tx.ExecContext(persist, `UPDATE accounts SET runtime_status='DELETE_PENDING',runtime_status_detail=$2,deleted_at=NULL WHERE id=$1 AND deletion_requested_at IS NOT NULL`, id, detail)
		}
		if saveErr == nil {
			saveErr = tx.Commit()
		}
		if saveErr != nil {
			return errors.Join(err, fmt.Errorf("persist deletion status: %w", saveErr))
		}
	}
	return err
}

var errDeletionSettling = errors.New("waiting for in-flight account work to settle")

// Claim fairly, skipping another worker's locked account. Recheck the due time
// under the session lock: a candidate may already have been processed.
func (c Container) claimAccountDeletion(ctx context.Context) (string, func(), error) {
	conn, err := c.DB.Conn(ctx)
	if err != nil {
		return "", nil, err
	}
	unlock := func(id string) {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock(hashtextextended($1,713))", id); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}
	type candidate struct {
		id             string
		due, requested time.Time
	}
	var last candidate
	var after any
	for {
		// Keyset pagination also skips a whole page of locked candidates. The
		// caller's work deadline bounds the scan, not a starvation-prone prefix.
		rows, e := conn.QueryContext(ctx, `SELECT account_id::text,next_attempt_at,requested_at FROM account_deletion_jobs
 WHERE next_attempt_at<=now() AND ($1::timestamptz IS NULL OR (next_attempt_at,requested_at,account_id)>($1::timestamptz,$2::timestamptz,NULLIF($3,'')::uuid))
 ORDER BY next_attempt_at,requested_at,account_id LIMIT 32`, after, last.requested, last.id)
		if e != nil {
			conn.Close()
			return "", nil, e
		}
		var candidates []candidate
		for rows.Next() {
			var v candidate
			if e = rows.Scan(&v.id, &v.due, &v.requested); e != nil {
				rows.Close()
				conn.Close()
				return "", nil, e
			}
			candidates = append(candidates, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil || len(candidates) == 0 {
			conn.Close()
			return "", nil, e
		}
		for _, v := range candidates {
			id := v.id
			var locked bool
			if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,713))", id).Scan(&locked); err != nil {
				// An interrupted response can hide a successfully acquired lock.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
				conn.Close()
				return "", nil, err
			}
			if !locked {
				continue
			}
			res, e := conn.ExecContext(ctx, `UPDATE account_deletion_jobs SET attempts=attempts+1,next_attempt_at=now()+interval '1 minute' WHERE account_id=$1 AND next_attempt_at<=now()`, id)
			if e != nil {
				unlock(id)
				conn.Close()
				return "", nil, e
			}
			n, e := res.RowsAffected()
			if e != nil {
				unlock(id)
				conn.Close()
				return "", nil, e
			}
			if n == 1 {
				return id, func() { unlock(id); conn.Close() }, nil
			}
			unlock(id)
		}
		last = candidates[len(candidates)-1]
		after = last.due
	}
}

func deletionErrorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "verification interrupted or timed out; automatic retry scheduled"
	}
	if errors.Is(err, ErrNetworkNotReady) {
		return "account network not ready"
	}
	if class := providers.Class(err); class != "" && class != providers.ErrorUnknown {
		return string(class)
	}
	switch err.Error() {
	case "managed resources or operations pending":
		return "managed servers or operations are still being deleted"
	case "waiting for in-flight account work to settle":
		return "waiting for in-flight work to settle"
	case "tracked provider resource still exists":
		return "provider still reports managed servers"
	case "SSH cleanup continues next chunk":
		return "managed SSH key cleanup is in progress"
	}
	return "fresh provider verification or resource cleanup incomplete"
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
			return errDeletionSettling
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
	if err = lockAccountPurge(ctx, tx, id); err != nil {
		return err
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
	return commitAccountPurge(ctx, tx, id)
}

func (c Container) cleanupDeletionKeys(ctx context.Context, id string, rt AccountRuntime) error {
	if rt.Driver.Capabilities().InlineSSHKeys {
		return nil
	}
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

func deletionRetrySeconds(state string) int {
	switch state {
	case "LOCKED", "TOKEN_INVALID", "PERMISSION_DENIED", "BILLING_BLOCKED":
		return 1800
	default:
		return 60
	}
}
