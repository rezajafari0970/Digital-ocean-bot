package transportapply

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportcore"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
	"time"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}
type RemoteSSH interface {
	Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error
	Run(context.Context, provisioning.Target, []byte, string) (string, error)
}
type Service struct {
	DB       *sql.DB
	Secrets  Secrets
	Runtimes *sanaei.RuntimeManager
	SSH      RemoteSSH
}
type job struct {
	id, panel, key, ref, phase string
	revision                   int64
	remote                     sql.NullInt64
	before, target, source     transportprofile.Config
	attempts                   int
	restartSent                bool
	previousProcess            string
	rollback                   bool
}

func (s Service) RunOne(ctx context.Context) error {
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	// Same global executor fence as durable client mutations; no concurrent
	// cross-process client write may race a full native inbound update.
	var locked bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(628341902731)").Scan(&locked); e != nil || !locked {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(c, "SELECT pg_advisory_unlock(628341902731)"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	var j job
	var before, target, source []byte
	e = s.DB.QueryRowContext(ctx, `SELECT id::text,panel_id::text,managed_key,policy_revision,before_config,target_config,before_secret_ref,remote_id,phase,attempts,restart_sent_at IS NOT NULL,source_config,restart_before_process,rollback_of IS NOT NULL
 FROM reality_transport_apply_jobs WHERE state IN('PENDING','RUNNING') AND next_retry_at<=now()
 ORDER BY CASE WHEN state='RUNNING' THEN 0 ELSE 1 END,created_at LIMIT 1`).Scan(&j.id, &j.panel, &j.key, &j.revision, &before, &target, &j.ref, &j.remote, &j.phase, &j.attempts, &j.restartSent, &source, &j.previousProcess, &j.rollback)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if json.Unmarshal(before, &j.before) != nil || json.Unmarshal(target, &j.target) != nil || json.Unmarshal(source, &j.source) != nil {
		return blocked(ctx, s.DB, j.id, "INVALID_SAVED_PROFILE")
	}
	if j.attempts >= 5 {
		return blocked(ctx, s.DB, j.id, "RETRY_LIMIT")
	}
	// A lost advisory connection must never authorize the next mutation.
	fence := func() error { var one int; return conn.QueryRowContext(ctx, "SELECT 1").Scan(&one) }
	// Drain pre-existing client work before freezing the native client set.
	// New reservations are rejected by the migration's admission trigger.
	if j.ref == "" {
		var pending bool
		if e = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND state IN('PENDING','RUNNING'))", j.panel).Scan(&pending); e != nil {
			return e
		}
		if pending {
			var expired bool
			if e = s.DB.QueryRowContext(ctx, "SELECT created_at<now()-interval '2 minutes' FROM reality_transport_apply_jobs WHERE id=$1", j.id).Scan(&expired); e != nil {
				return e
			}
			if expired {
				if !jsonEqual(j.source, j.before) {
					return blocked(ctx, s.DB, j.id, "ROLLBACK_WAITING_FOR_CLIENT_JOBS")
				}
				_, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='FAILED',phase='CLIENT_DRAIN_TIMEOUT',last_error='CLIENT_WORK_DID_NOT_DRAIN',completed_at=now(),updated_at=now() WHERE id=$1 AND before_secret_ref=''", j.id)
				return e
			}
			return s.retry(ctx, j, "WAITING_FOR_CLIENT_JOBS")
		}
	}
	var account, droplet, host, user, keyRef string
	var alive, allowed bool
	e = s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,dep.host,COALESCE(dep.profile_snapshot->>'ssh_user','root'),dep.profile_snapshot->>'ssh_key_secret_ref',
 d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now()+interval '90 seconds'),
 pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND dep.state='PANEL_COMPLETE'
 AND EXISTS(SELECT 1 FROM global_config_policies g WHERE g.policy_key='reality' AND g.enabled)
 AND NOT EXISTS(SELECT 1 FROM reality_rollout_control c WHERE c.mode='canary' AND NOT EXISTS(SELECT 1 FROM reality_rollout_panels rp WHERE rp.panel_id=pi.id AND rp.enabled))
 FROM panel_instances pi JOIN droplets d ON d.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=d.id WHERE pi.id=$1`, j.panel).Scan(&account, &droplet, &host, &user, &keyRef, &alive, &allowed)
	if e != nil {
		return e
	}
	if !alive {
		if j.ref != "" || !jsonEqual(j.source, j.before) {
			return blocked(ctx, s.DB, j.id, "SERVER_RETIRED_WITH_UNRESOLVED_TRANSPORT")
		}
		_, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='OBSOLETE',phase='SERVER_RETIRED',completed_at=now(),updated_at=now() WHERE id=$1", j.id)
		return e
	}
	if !allowed {
		_, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET next_retry_at=now()+interval '30 seconds',last_error='WAITING_FOR_PANEL_ELIGIBILITY' WHERE id=$1", j.id)
		return e
	}
	targetHost := provisioning.Target{AccountID: account, DropletID: droplet, Host: host, User: user, KeySecretRef: keyRef}
	return sanaei.WithConfigLock(ctx, s.DB, j.panel, func(ctx context.Context) error {
		if _, e := s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET attempts=attempts+1,updated_at=now() WHERE id=$1", j.id); e != nil {
			return e
		}
		rt, e := s.Runtimes.Acquire(ctx, j.panel)
		if e != nil {
			return s.retry(ctx, j, "PANEL_UNAVAILABLE")
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error {
			if e = fence(); e != nil {
				return e
			}
			if _, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='RUNNING',next_retry_at=now()+interval '60 seconds',updated_at=now() WHERE id=$1", j.id); e != nil {
				return e
			}
			current, id, e := read(ctx, rt, j.key)
			if e != nil {
				return s.retry(ctx, j, "NATIVE_READ_UNAVAILABLE")
			}
			if j.remote.Valid && j.remote.Int64 != id {
				return blocked(ctx, s.DB, j.id, "LISTENER_ID_CHANGED")
			}
			var before []byte
			if j.ref == "" {
				// Verify the old pin against a fresh native read before recording any intent.
				if e = Verify(current, current, j.key, j.source); e != nil && !(j.rollback && Verify(current, current, j.key, j.target) == nil) {
					return blocked(ctx, s.DB, j.id, "OLD_LISTENER_DRIFT")
				}
				j.ref = "transport-apply-" + j.id
				if e = s.Secrets.Put(ctx, account, j.ref, "transport-apply-before", current); e != nil {
					return e
				}
				if _, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET before_secret_ref=$2,remote_id=$3,phase='PREPARING',updated_at=now() WHERE id=$1", j.id, j.ref, id); e != nil {
					return e
				}
				before = current
			} else {
				before, e = s.Secrets.Get(ctx, account, j.ref)
				if e != nil {
					return blocked(ctx, s.DB, j.id, "BACKUP_UNAVAILABLE")
				}
			}
			defer clear(before)
			candidate, payload, e := Transform(before, j.key, j.target)
			if e != nil {
				return blocked(ctx, s.DB, j.id, "UNSUPPORTED_LISTENER")
			}
			defer clear(candidate)
			sshKey, e := s.Secrets.Get(ctx, account, keyRef)
			if e != nil {
				return s.retry(ctx, j, "SSH_KEY_UNAVAILABLE")
			}
			defer clear(sshKey)
			// An unknown prior result is first reconciled from the native API. Never
			// blindly repeat the POST after a timeout or process restart.
			matched := Verify(before, current, j.key, j.target) == nil
			if !matched {
				if Verify(before, current, j.key, j.source) != nil {
					return blocked(ctx, s.DB, j.id, "UNEXPECTED_NATIVE_CHANGE")
				}
				if e = fence(); e != nil {
					return e
				}
				if e = transportcore.Prepare(ctx, s.SSH, targetHost, sshKey, payload); e != nil {
					return s.retry(ctx, j, "CORE_PREFLIGHT_FAILED")
				}
				// Core/panel upgrade may replace the session. Re-acquire and fresh-read.
				rt.Session.Invalidate()
				fresh, freshID, e := read(ctx, rt, j.key)
				if e != nil {
					return s.retry(ctx, j, "POST_PREFLIGHT_READ_FAILED")
				}
				if freshID != id || Verify(before, fresh, j.key, j.source) != nil {
					return blocked(ctx, s.DB, j.id, "PREFLIGHT_CHANGED_LISTENER")
				}
				if e = fence(); e != nil {
					return e
				}
				var eligible bool
				if e = s.DB.QueryRowContext(ctx, `SELECT pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now()+interval '60 seconds') FROM panel_instances pi JOIN droplets d ON d.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id WHERE pi.id=$1`, j.panel).Scan(&eligible); e != nil || !eligible {
					return s.retry(ctx, j, "ELIGIBILITY_CHANGED")
				}
				if _, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET phase='WRITE_SENT',updated_at=now() WHERE id=$1", j.id); e != nil {
					return e
				}
				// The live payload is canonical JSON, preserved as RawMessage.
				_, writeErr := sanaei.UpdateInboundRaw(ctx, rt.Session.Exec, id, json.RawMessage(candidate))
				current, freshID, e = read(ctx, rt, j.key)
				if e != nil {
					return s.retry(ctx, j, "WRITE_OUTCOME_UNKNOWN")
				}
				if freshID != id || Verify(before, current, j.key, j.target) != nil {
					if writeErr != nil && Verify(before, current, j.key, j.source) == nil {
						return s.retry(ctx, j, "WRITE_NOT_OBSERVED")
					}
					return blocked(ctx, s.DB, j.id, "NATIVE_READBACK_MISMATCH")
				}
			}
			// A fresh process is required, since native hot apply can leave the
			// startup file stale. Persist its old identity before one restart.
			if !j.restartSent {
				j.previousProcess, e = RuntimeProcessID(ctx, s.SSH, targetHost, sshKey)
				if e != nil {
					return s.retry(ctx, j, "PROCESS_ID_UNAVAILABLE")
				}
				if e = fence(); e != nil {
					return e
				}
				if _, e = s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET phase='RESTART_SENT',restart_sent_at=now(),restart_before_process=$2,updated_at=now() WHERE id=$1", j.id, j.previousProcess); e != nil {
					return e
				}
				j.restartSent = true
				_, _ = rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/server/restartXrayService", TimeoutSeconds: 30})
			}
			current, _, e = read(ctx, rt, j.key)
			if e != nil || Verify(before, current, j.key, j.target) != nil {
				return s.retry(ctx, j, "RESTART_READBACK_PENDING")
			}
			if e = VerifyRuntime(ctx, s.SSH, targetHost, sshKey, payload, current, j.previousProcess); e != nil {
				return s.retry(ctx, j, "CORE_READBACK_PENDING")
			}

			if e = fence(); e != nil {
				return e
			}
			return s.promote(ctx, j)
		})
	})
}
func read(ctx context.Context, rt *sanaei.PanelRuntime, key string) ([]byte, int64, error) {
	raws, e := rt.Session.FreshSnapshot(ctx)
	if e != nil {
		return nil, 0, e
	}
	var result []byte
	var id int64
	for _, raw := range raws {
		var head struct {
			ID     int64
			Remark string
		}
		if json.Unmarshal(raw, &head) != nil {
			return nil, 0, ErrConflict
		}
		if head.Remark == key {
			if result != nil || head.ID <= 0 {
				return nil, 0, ErrConflict
			}
			result = append([]byte(nil), raw...)
			id = head.ID
		}
	}
	if result == nil {
		return nil, 0, ErrConflict
	}
	return result, id, nil
}
func (s Service) retry(ctx context.Context, j job, code string) error {
	_, e := s.DB.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET last_error=$2,next_retry_at=now()+interval '30 seconds',updated_at=now() WHERE id=$1", j.id, code)
	if e != nil {
		return e
	}
	return fmt.Errorf("transport apply waiting: %s", code)
}
func (s Service) promote(ctx context.Context, j job) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw []byte
	if e = tx.QueryRowContext(ctx, "SELECT config FROM reality_listener_profiles WHERE panel_id=$1 AND managed_key=$2 FOR UPDATE", j.panel, j.key).Scan(&raw); e != nil {
		return e
	}
	var current transportprofile.Config
	if json.Unmarshal(raw, &current) != nil || !jsonEqual(current, j.before) {
		return ErrConflict
	}
	target, _ := json.Marshal(j.target)
	var path any
	if j.target.Path != "" {
		path = j.target.Path
	}
	if _, e = tx.ExecContext(ctx, "UPDATE reality_listener_profiles SET config=$3,path=$4,policy_revision=$5 WHERE panel_id=$1 AND managed_key=$2", j.panel, j.key, target, path, j.revision); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE panel_inbound_inventory SET transport=$3 WHERE panel_id=$1 AND remark=$2", j.panel, j.key, j.target.NetworkName()); e != nil {
		return e
	}
	// In-flight collectors share the config lock. Delete prior links atomically
	// with pin promotion so no old snapshot leaks after publication resumes.
	if _, e = tx.ExecContext(ctx, "DELETE FROM output_config_snapshots WHERE panel_id=$1", j.panel); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE reality_transport_apply_jobs SET state='SUCCEEDED',phase='CORE_VERIFIED_PIN_PROMOTED',last_error='',completed_at=now(),updated_at=now() WHERE id=$1", j.id); e != nil {
		return e
	}
	return tx.Commit()
}
