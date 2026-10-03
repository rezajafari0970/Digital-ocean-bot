package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func lifeWait(ctx context.Context, j clientops.Journal, description string, f func() (bool, error)) error {
	for {
		ok, err := f()
		if err != nil {
			return fmt.Errorf("%s: %w", description, err)
		}
		if ok {
			return nil
		}
		gate, err := j.Gate(ctx)
		if err != nil {
			return err
		}
		if !gate.Enabled || gate.KillSwitch {
			return fmt.Errorf("%s: worker gate closed; read jobs before recovery", description)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func lifeCounts(ctx context.Context, db *sql.DB, id string) (active, deleted, pending int, err error) {
	err = db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE state='ACTIVE'),count(*) FILTER(WHERE state='DELETED'),count(*) FILTER(WHERE state IN ('PLANNED','DELETE_PENDING')) FROM bulk_user_ownership WHERE generation_id=$1`, id).Scan(&active, &deleted, &pending)
	if err != nil {
		return
	}
	var jobs int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE payload->>'GenerationID'=$1 AND state IN ('PENDING','RUNNING','FAILED')`, id).Scan(&jobs)
	pending += jobs
	return
}
func runLifecycleCanary(panel string, inbound int64, recoverID, quotaHost string) (retErr error) {
	if quotaHost != "" {
		if err := quotaCallbackIP(quotaHost); err != nil {
			return err
		}
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 12*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	j := clientops.Journal{DB: a.DB}
	gate, err := j.Gate(ctx)
	if err != nil {
		return err
	}
	if gate.Enabled || !gate.KillSwitch {
		return fmt.Errorf("main gate must be closed")
	}
	var unsafe bool
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled OR NOT kill_switch FROM bulk_client_execution_gate WHERE singleton`).Scan(&unsafe); err != nil {
		return err
	}
	if unsafe {
		return fmt.Errorf("bulk gate must be closed")
	}
	var globalEnabled bool
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled FROM global_config_policies WHERE policy_key='reality'`).Scan(&globalEnabled); err != nil {
		return err
	}
	if globalEnabled {
		return fmt.Errorf("fleet policy must be disabled for isolated acceptance")
	}
	r := scaleRun{Panel: panel, Inbound: inbound}
	if recoverID != "" {
		var raw []byte
		err = a.DB.QueryRowContext(ctx, `SELECT s.generation_id::text,g.panel_id::text,g.inbound_id,g.marker,s.baseline,s.phase FROM bulk_scale_runs s JOIN bulk_user_generations g ON g.id=s.generation_id WHERE s.generation_id=$1 AND s.evidence->>'kind'='LIFECYCLE'`, recoverID).Scan(&r.ID, &r.Panel, &r.Inbound, &r.Marker, &raw, &r.Phase)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &r.Baseline); err != nil {
			return err
		}
		if r.Phase == "SUCCEEDED" {
			fmt.Println("LIFECYCLE_ALREADY_CLOSED", r.ID)
			return nil
		}
	}
	if r.Panel == "" || r.Inbound <= 0 {
		return fmt.Errorf("exact panel/inbound required")
	}
	conn, err := a.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,942))`, fmt.Sprintf("%s:%d", r.Panel, r.Inbound)).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("scope utility already running")
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,942))`, fmt.Sprintf("%s:%d", r.Panel, r.Inbound))
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		if e := j.FailCloseGate(c); e != nil {
			fmt.Println("LIFECYCLE_GATE_CLOSE_ERROR", e)
		}
		if r.ID != "" {
			_, _ = a.DB.ExecContext(c, `UPDATE bulk_lifecycle_scopes SET enabled=false,allow_create=false,updated_at=now() WHERE generation_id=$1`, r.ID)
			if retErr != nil {
				_, _ = a.DB.ExecContext(c, `UPDATE bulk_scale_runs SET phase='FAILED',last_error=$2,updated_at=now() WHERE generation_id=$1 AND phase<>'SUCCEEDED'`, r.ID, retErr.Error())
			}
		}
	}()
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: time.Second}
	rt, err := manager.Acquire(ctx, r.Panel)
	if err != nil {
		return err
	}
	if r.ID == "" {
		var safe bool
		err = a.DB.QueryRowContext(ctx, `SELECT d.expires_at>now()+interval '15 minutes' FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=$1`, r.Panel).Scan(&safe)
		if err != nil {
			return err
		}
		if !safe {
			return fmt.Errorf("insufficient server lifetime")
		}
		var blocked int
		err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING','FAILED')`, r.Panel, r.Inbound).Scan(&blocked)
		if err != nil {
			return err
		}
		if blocked != 0 {
			return fmt.Errorf("scope has unresolved mutations")
		}
		if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_lifecycle_scopes WHERE panel_id=$1 AND inbound_id=$2 AND enabled`, r.Panel, r.Inbound).Scan(&blocked); err != nil {
			return err
		}
		if blocked != 0 {
			return fmt.Errorf("scope already enabled; use existing run recovery")
		}
		r.Baseline, err = snapshot(ctx, rt, r.Inbound)
		if err != nil {
			return err
		}
		if len(r.Baseline) < 1 || len(r.Baseline) > 100 {
			return fmt.Errorf("bounded baseline required")
		}
		observed, _, e := clientops.LifecycleInventory(ctx, rt, r.Inbound)
		if e != nil {
			return e
		}
		active := 0
		for _, c := range observed {
			reason, e := c.InactiveReason(time.Now())
			if e != nil {
				return e
			}
			if reason == "" {
				active++
			}
		}
		r.ID, err = sanaei.UUIDv4()
		if err != nil {
			return err
		}
		r.Marker = "lifecycle" + strings.ReplaceAll(r.ID, "-", "")
		r.Target = active + 3
		raw, _ := json.Marshal(r.Baseline)
		tx, e := a.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_user_generations(id,panel_id,inbound_id,purpose,marker) VALUES($1,$2,$3,'CANARY',$4)`, r.ID, r.Panel, r.Inbound, r.Marker); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_scale_runs(generation_id,target_users,baseline,phase,expires_at,evidence) VALUES($1,$2,$3,'CREATING',now()+interval '12 minutes',jsonb_build_object('kind','LIFECYCLE','baseline_active',$4::int))`, r.ID, len(r.Baseline)+3, raw, active); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_lifecycle_scopes(panel_id,inbound_id,generation_id,enabled,use_global_policy,allow_create,target_users,quota_bytes,lifetime_seconds,device_limit,users_per_second,max_batch_size,remaining_operations,expires_at) VALUES($1,$2,$3,true,false,true,$4,104857600,180,2,10,10,40,now()+interval '12 minutes') ON CONFLICT(panel_id,inbound_id) DO UPDATE SET generation_id=excluded.generation_id,enabled=true,use_global_policy=false,allow_create=true,target_users=excluded.target_users,quota_bytes=excluded.quota_bytes,lifetime_seconds=excluded.lifetime_seconds,device_limit=excluded.device_limit,users_per_second=10,max_batch_size=10,remaining_operations=40,expires_at=excluded.expires_at,last_error='',updated_at=now() WHERE NOT bulk_lifecycle_scopes.enabled`, r.Panel, r.Inbound, r.ID, r.Target); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE bulk_lifecycle_control SET enabled=true,updated_at=now()`); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("LIFECYCLE_PLANNED run=%s panel=%s inbound=%d baseline=%d baseline_active=%d baseline_sha256=%x\n", r.ID, r.Panel, r.Inbound, len(r.Baseline), active, sha256.Sum256(raw))
		fmt.Printf("LIFECYCLE_RECOVERY bulk-client-canary -lifecycle-cleanup %s\n", r.ID)
	} else {
		// Read-before-write recovery never generates replacement identities.
		rows, e := a.DB.QueryContext(ctx, `SELECT id::text FROM client_mutation_jobs WHERE payload->>'GenerationID'=$1 AND state IN ('PENDING','RUNNING','FAILED')`, r.ID)
		if e != nil {
			return e
		}
		var ids []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			job, e := j.Get(ctx, id)
			if e != nil {
				return e
			}
			if job.State == clientops.StateRunning {
				return fmt.Errorf("worker still running; observe before recovery")
			}
			switch job.Kind {
			case clientops.KindBulkCreate:
				_, missing, e := (clientops.Executor{Journal: j}).ReconcileBulkOnly(ctx, rt, job)
				if e != nil {
					return e
				}
				for _, c := range missing {
					if _, e = a.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ABORTED' WHERE mutation_job_id=$1 AND client_id=$2 AND state='PLANNED'`, id, c.ID); e != nil {
						return e
					}
				}
				if e = j.CompleteBulkRecovery(ctx, job, clientops.StateObsolete); e != nil {
					return e
				}
			case clientops.KindBulkDelete:
				present, e := (clientops.Executor{Journal: j}).ReconcileBulkDeleteOnly(ctx, rt, job)
				if e != nil {
					return e
				}
				if len(present) == 0 {
					if e = j.CompleteBulkRecovery(ctx, job, clientops.StateSucceeded); e != nil {
						return e
					}
				} else if job.State != clientops.StatePending {
					return fmt.Errorf("failed deletion requires conflict review")
				}
			case clientops.KindUpdate:
				// Identity verification is required before the same immutable update may resume.
				obs, _, e := clientops.LifecycleInventory(ctx, rt, r.Inbound)
				if e != nil {
					return e
				}
				var p struct{ ExpectedEmail string }
				if json.Unmarshal(job.Payload, &p) != nil || obs[job.ClientID].Client.Email != p.ExpectedEmail {
					return fmt.Errorf("update identity conflict")
				}
				if job.State != clientops.StatePending {
					return fmt.Errorf("failed update requires conflict review")
				}
			default:
				return fmt.Errorf("unexpected recovery kind")
			}
		}
		if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET enabled=true,allow_create=false,target_users=0,remaining_operations=40,expires_at=now()+interval '10 minutes',updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
			return err
		}
	}
	if err = scaleOpenMain(ctx, a.DB, r); err != nil {
		return err
	}
	if recoverID == "" {
		if err = lifeWait(ctx, j, "initial create", func() (bool, error) {
			active, deleted, pending, e := lifeCounts(ctx, a.DB, r.ID)
			return active == 3 && deleted == 0 && pending == 0, e
		}); err != nil {
			return err
		}
		original, err := scaleExpected(ctx, a.DB, r)
		if err != nil {
			return err
		}
		if len(original) != 3 {
			return fmt.Errorf("initial identity count")
		}
		if err = scaleOutput(ctx, a.DB, r, original, 3); err != nil {
			return err
		}
		if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('created_output_verified',3),phase='VERIFYING',updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
			return err
		}
		if quotaHost != "" {
			if err = quotaPhase(ctx, a.DB, rt, j, r, original, quotaHost); err != nil {
				return err
			}
		} else {
			// Fixed origin time is retained when shortening lifetime; no rolling extension.
			if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET quota_bytes=20971520,lifetime_seconds=75,device_limit=3,updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
				return err
			}
			var expiry int64
			if err = lifeWait(ctx, j, "policy update", func() (bool, error) {
				obs, _, e := clientops.LifecycleInventory(ctx, rt, r.Inbound)
				if errors.Is(e, clientops.ErrVerify) {
					return false, nil
				} // pair can straddle a worker UPDATE; retry reads only.
				if e != nil {
					return false, e
				}
				for id := range original {
					c, ok := obs[id]
					if !ok || c.Client.TotalGB != 20971520 || c.Client.LimitHWID != 3 || c.Client.ExpiryTime <= 0 {
						return false, nil
					}
					expiry = c.Client.ExpiryTime
				}
				_, _, pending, e := lifeCounts(ctx, a.DB, r.ID)
				return pending == 0, e
			}); err != nil {
				return err
			}
			if err = lifeWait(ctx, j, "output expiry boundary", func() (bool, error) {
				var n int
				var latest sql.NullTime
				e := a.DB.QueryRowContext(ctx, `SELECT count(*),max(s.visible_until) FROM output_config_snapshots s JOIN bulk_user_ownership o ON o.client_id=split_part(split_part(s.uri,'://',2),'@',1) WHERE o.generation_id=$1 AND s.panel_id=$2 AND o.state='ACTIVE'`, r.ID, r.Panel).Scan(&n, &latest)
				if e != nil {
					return false, e
				}
				return n == 3 && latest.Valid && latest.Time.UnixMilli() <= expiry-10000, nil
			}); err != nil {
				return err
			}
			if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('output_visible_until_at_least_10_seconds_early',true) WHERE generation_id=$1`, r.ID); err != nil {
				return err
			}
			if time.Until(time.UnixMilli(expiry)) < 20*time.Second {
				return fmt.Errorf("insufficient expiry observation window")
			}
			if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('policy_verified',true,'client_expiry_ms',$2::bigint),updated_at=now() WHERE generation_id=$1`, r.ID, expiry); err != nil {
				return err
			}
			fmt.Printf("LIFECYCLE_POLICY_VERIFIED run=%s quota=20971520 lifetime=75 limitHwid=3 expiry_ms=%d\n", r.ID, expiry)
			timer := time.NewTimer(time.Until(time.UnixMilli(expiry).Add(-5 * time.Second)))
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
			if err = scaleOutput(ctx, a.DB, r, original, 0); err != nil {
				return err
			}
			if time.Now().UnixMilli() >= expiry {
				return fmt.Errorf("pre-expiry output verification was late")
			}
			if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('output_hidden_before_expiry_at',now()),updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
				return err
			}
			fmt.Printf("LIFECYCLE_OUTPUT_HIDDEN run=%s before_expiry_ms=%d\n", r.ID, expiry-time.Now().UnixMilli())
			if err = lifeWait(ctx, j, "expiry and replacement", func() (bool, error) {
				active, deleted, pending, e := lifeCounts(ctx, a.DB, r.ID)
				return active == 3 && deleted == 3 && pending == 0, e
			}); err != nil {
				return err
			}
			all, e := scaleExpected(ctx, a.DB, r)
			if e != nil {
				return e
			}
			if len(all) != 6 {
				return fmt.Errorf("replacement identity count=%d", len(all))
			}
			if err = scaleOutput(ctx, a.DB, r, all, 3); err != nil {
				return err
			}
			obs, _, err := clientops.LifecycleInventory(ctx, rt, r.Inbound)
			if err != nil {
				return err
			}
			for id := range original {
				if _, ok := obs[id]; ok {
					return fmt.Errorf("expired identity still present")
				}
			}
			for id := range all {
				if _, old := original[id]; old {
					continue
				}
				c, ok := obs[id]
				if !ok || c.Client.TotalGB != 20971520 || c.Client.LimitHWID != 3 || !c.Client.Enable {
					return fmt.Errorf("replacement policy mismatch")
				}
			}
			if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||jsonb_build_object('expired_deleted',3,'replacements_output_verified',3),phase='CLEANING',updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
				return err
			}
			fmt.Printf("LIFECYCLE_REPLACEMENT_VERIFIED run=%s deleted=3 new=3\n", r.ID)
		}
		if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET allow_create=false,target_users=0,updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
			return err
		}
	}
	if err = lifeWait(ctx, j, "final owned cleanup", func() (bool, error) {
		active, _, pending, e := lifeCounts(ctx, a.DB, r.ID)
		return active == 0 && pending == 0, e
	}); err != nil {
		return err
	}
	if err = j.FailCloseGate(ctx); err != nil {
		return err
	}
	all, err := scaleExpected(ctx, a.DB, r)
	if err != nil {
		return err
	}
	if err = scaleOutput(ctx, a.DB, r, all, 0); err != nil {
		return err
	}
	current, err := snapshot(ctx, rt, r.Inbound)
	if err != nil {
		return err
	}
	if err = scaleMatch(current, r.Baseline, map[string]sanaei.Client{}); err != nil {
		return err
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET enabled=false,allow_create=false,updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='CLOSED',closed_at=now() WHERE id=$1`, r.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_scale_runs SET phase='SUCCEEDED',completed_at=now(),updated_at=now(),last_error='',evidence=evidence||jsonb_build_object('baseline_restored',true,'cleanup_only',$2::boolean) WHERE generation_id=$1`, r.ID, recoverID != ""); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	raw, _ := json.Marshal(current)
	fmt.Printf("LIFECYCLE_CLOSED run=%s baseline=%d baseline_sha256=%x\n", r.ID, len(current), sha256.Sum256(raw))
	return nil
}
