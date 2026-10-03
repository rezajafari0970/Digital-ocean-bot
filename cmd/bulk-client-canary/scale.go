package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/usercapacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

type scaleRun struct {
	ID, Panel, Marker, Phase string
	Inbound                  int64
	Target                   int
	Baseline                 map[string]sanaei.Client
	Expires                  time.Time
}
type scaleHealth struct {
	Time      float64 `json:"time"`
	Available int64   `json:"available_kib"`
	RSS       int64   `json:"rss_kib"`
	Processes int     `json:"processes"`
	Active    bool    `json:"active"`
}

func scaleTargetValid(n int) bool { return n == 11 || n == 101 || n == 10000 }
func scaleMatch(current, baseline, wanted map[string]sanaei.Client) error {
	if len(current) != len(baseline)+len(wanted) {
		return fmt.Errorf("inventory mismatch observed=%d expected=%d", len(current), len(baseline)+len(wanted))
	}
	for id, c := range baseline {
		if current[id] != c {
			return fmt.Errorf("baseline changed")
		}
	}
	for id, c := range wanted {
		got, ok := current[id]
		got.LimitHWID = 0
		c.LimitHWID = 0
		if !ok || got != c {
			return fmt.Errorf("owned inventory mismatch")
		}
	}
	return nil
}
func scaleOpenBulk(ctx context.Context, db *sql.DB, r scaleRun, n int) error {
	result, err := db.ExecContext(ctx, `UPDATE bulk_client_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,max_batch_size=$3,remaining_batches=1,expires_at=now()+interval '2 minutes',updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, r.Panel, r.Inbound, n)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("bulk gate changed")
	}
	return nil
}
func scaleOpenMain(ctx context.Context, db *sql.DB, r scaleRun) error {
	result, err := db.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,panel_id=$1,inbound_id=$2,concurrency=1,updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, r.Panel, r.Inbound)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("main gate changed")
	}
	return nil
}
func scaleExpected(ctx context.Context, db *sql.DB, r scaleRun) (map[string]sanaei.Client, error) {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND kind='BULK_CREATE' AND payload->>'GenerationID'=$3 ORDER BY created_at`, r.Panel, r.Inbound, r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := map[string]sanaei.Client{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var p clientops.BulkPayload
		if err = json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		for _, c := range p.Clients {
			if _, ok := all[c.ID]; ok {
				return nil, fmt.Errorf("duplicate planned identity")
			}
			all[c.ID] = c
		}
	}
	return all, rows.Err()
}
func scaleActive(ctx context.Context, db *sql.DB, r scaleRun, all map[string]sanaei.Client) (map[string]sanaei.Client, error) {
	rows, err := db.QueryContext(ctx, `SELECT client_id,email,state FROM bulk_user_ownership WHERE generation_id=$1`, r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := map[string]sanaei.Client{}
	for rows.Next() {
		var id, email, state string
		if err = rows.Scan(&id, &email, &state); err != nil {
			return nil, err
		}
		c, ok := all[id]
		if !ok || c.Email != email {
			return nil, fmt.Errorf("ownership differs from immutable plan")
		}
		switch state {
		case "ACTIVE":
			active[id] = c
		case "DELETED", "ABORTED":
		default:
			return nil, fmt.Errorf("unreconciled ownership state=%s", state)
		}
	}
	return active, rows.Err()
}
func scaleOutput(ctx context.Context, db *sql.DB, r scaleRun, all map[string]sanaei.Client, want int) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		rows, err := db.QueryContext(ctx, `SELECT uri FROM output_config_snapshots WHERE panel_id=$1 AND visible_until>now() AND last_seen_at>now()-interval '15 seconds'`, r.Panel)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for rows.Next() {
			var uri string
			if err = rows.Scan(&uri); err != nil {
				rows.Close()
				return err
			}
			u, e := url.Parse(uri)
			if e == nil && u.User != nil {
				if _, ok := all[u.User.Username()]; ok {
					seen[u.User.Username()] = true
				}
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(seen) == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("output count=%d wanted=%d", len(seen), want)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func runScale(panel string, inbound int64, target int, runID string, cleanup bool) (retErr error) {
	if cleanup && runID == "" {
		return fmt.Errorf("scale cleanup requires existing run ID")
	}
	if runID == "" && (!scaleTargetValid(target) || panel == "" || inbound <= 0) {
		return fmt.Errorf("new scale requires exact scope and target 11, 101 or 10000")
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, 2*time.Hour)
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
	var enabled bool
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled FROM global_config_policies WHERE policy_key='reality'`).Scan(&enabled); err != nil {
		return err
	}
	if enabled {
		return fmt.Errorf("global creation policy must be disabled")
	}
	if err = a.DB.QueryRowContext(ctx, `SELECT enabled OR NOT kill_switch FROM bulk_client_execution_gate WHERE singleton`).Scan(&enabled); err != nil {
		return err
	}
	if enabled {
		return fmt.Errorf("bulk gate must be closed")
	}
	r := scaleRun{Panel: panel, Inbound: inbound, Target: target}
	if runID != "" {
		var raw []byte
		err = a.DB.QueryRowContext(ctx, `SELECT s.generation_id::text,g.panel_id::text,g.inbound_id,g.marker,s.target_users,s.baseline,s.phase,s.expires_at FROM bulk_scale_runs s JOIN bulk_user_generations g ON g.id=s.generation_id WHERE s.generation_id=$1 AND g.purpose='CANARY'`, runID).Scan(&r.ID, &r.Panel, &r.Inbound, &r.Marker, &r.Target, &raw, &r.Phase, &r.Expires)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &r.Baseline); err != nil {
			return err
		}
		if r.Phase == "SUCCEEDED" {
			fmt.Printf("SCALE_ALREADY_CLOSED run=%s\n", r.ID)
			return nil
		}
		if !cleanup && r.Phase == "FAILED" {
			return fmt.Errorf("failed run requires explicit -scale-cleanup")
		}
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
		return fmt.Errorf("scale scope already held")
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,942))`, fmt.Sprintf("%s:%d", r.Panel, r.Inbound))
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		if e := j.FailCloseGate(c); e != nil {
			log.Printf("CRITICAL gate close: %v", e)
		}
		if retErr != nil && r.ID != "" {
			a.DB.ExecContext(c, `UPDATE bulk_scale_runs SET phase='FAILED',last_error=$2,updated_at=now() WHERE generation_id=$1 AND phase<>'SUCCEEDED'`, r.ID, retErr.Error())
		}
	}()
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: 5 * time.Second}
	rt, err := manager.Acquire(ctx, r.Panel)
	if err != nil {
		return err
	}
	// SSH is exclusively a read-only resource guard; all client state uses Sanaei.
	var acc, did, host, user, keyref string
	err = a.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1`, r.Panel).Scan(&acc, &did, &host, &user, &keyref)
	if err != nil {
		return err
	}
	key, err := a.Container.Secrets.Get(ctx, acc, keyref)
	if err != nil {
		return err
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	ssh := provisioning.SSHClient{HostKeys: provisioning.SQLHostKeyPins{DB: a.DB}}
	sshTarget := provisioning.Target{AccountID: acc, DropletID: did, Host: host, Port: 22, User: user, KeySecretRef: keyref}
	health := func() (scaleHealth, error) {
		c, cc := context.WithTimeout(ctx, 10*time.Second)
		defer cc()
		result, e := ssh.RunDetailed(c, sshTarget, key, scaleHealthCommand)
		if e != nil {
			return scaleHealth{}, e
		}
		var h scaleHealth
		if e = json.Unmarshal([]byte(result.Stdout), &h); e != nil {
			return h, e
		}
		if !h.Active || h.Processes < 2 || h.Available < 384*1024 {
			return h, fmt.Errorf("resource guard: active=%t processes=%d available_kib=%d", h.Active, h.Processes, h.Available)
		}
		b, _ := json.Marshal(h)
		fmt.Printf("SCALE_HEALTH %s\n", b)
		return h, nil
	}
	if _, err = health(); err != nil {
		return err
	}
	if r.ID == "" {
		var enoughLifetime bool
		if err = a.DB.QueryRowContext(ctx, `SELECT d.expires_at IS NULL OR d.expires_at>now()+interval '2 hours 10 seconds' FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id WHERE p.id=$1`, r.Panel).Scan(&enoughLifetime); err != nil {
			return err
		}
		if !enoughLifetime {
			return fmt.Errorf("server expiry does not cover bounded scale window")
		}
		var pending int
		if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING')`, r.Panel, r.Inbound).Scan(&pending); err != nil {
			return err
		}
		if pending != 0 {
			return fmt.Errorf("scope has pending work")
		}
		r.Baseline, err = snapshot(ctx, rt, r.Inbound)
		if err != nil {
			return err
		}
		if len(r.Baseline) != 1 {
			return fmt.Errorf("new scale expects exactly one preserved manual client")
		}
		var owned int
		if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state IN ('PLANNED','ACTIVE','DELETE_PENDING')`, r.Panel, r.Inbound).Scan(&owned); err != nil {
			return err
		}
		if owned != 0 {
			return fmt.Errorf("scope already has owned clients")
		}
		r.ID, err = sanaei.UUIDv4()
		if err != nil {
			return err
		}
		r.Marker = "bulkscale" + strings.ReplaceAll(r.ID, "-", "")
		r.Phase = "CREATING"
		r.Expires = time.Now().Add(2 * time.Hour)
		b, _ := json.Marshal(r.Baseline)
		tx, e := a.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_user_generations(id,panel_id,inbound_id,purpose,marker) VALUES($1,$2,$3,'CANARY',$4)`, r.ID, r.Panel, r.Inbound, r.Marker); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_scale_runs(generation_id,target_users,baseline,phase,expires_at) VALUES($1,$2,$3,'CREATING',$4)`, r.ID, r.Target, b, r.Expires); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("SCALE_PLANNED run=%s target_total=%d baseline=1 baseline_sha256=%x\n", r.ID, r.Target, sha256.Sum256(b))
		fmt.Printf("SCALE_RECOVERY bulk-client-canary -scale-run %s -scale-cleanup\n", r.ID)
	}
	all, err := scaleExpected(ctx, a.DB, r)
	if err != nil {
		return err
	}
	// Explicit recovery first observes ambiguous creates; it never retries a POST.
	if cleanup {
		rows, e := a.DB.QueryContext(ctx, `SELECT id::text FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING','FAILED') AND kind IN ('BULK_CREATE','BULK_DELETE')`, r.Panel, r.Inbound)
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
			var p clientops.BulkPayload
			if json.Unmarshal(job.Payload, &p) != nil || p.GenerationID != r.ID {
				return fmt.Errorf("unrelated unresolved batch")
			}
			if job.State == clientops.StateRunning {
				return fmt.Errorf("job %s still running; read again after worker reconciliation", id)
			}
			if job.Kind == clientops.KindBulkDelete {
				present, e := (clientops.Executor{Journal: j}).ReconcileBulkDeleteOnly(ctx, rt, job)
				if e != nil {
					return e
				}
				if len(present) == 0 {
					if e = j.CompleteBulkRecovery(ctx, job, clientops.StateSucceeded); e != nil {
						return e
					}
					continue
				}
				if _, e = a.DB.ExecContext(ctx, `UPDATE client_mutation_jobs SET state='PENDING',next_retry_at=now(),updated_at=now(),last_error=last_error||'; explicit cleanup recovery after fresh read' WHERE id=$1 AND state IN ('PENDING','FAILED')`, id); e != nil {
					return e
				}
				if e = scaleOpenBulk(ctx, a.DB, r, len(p.Clients)); e != nil {
					return e
				}
				if e = scaleOpenMain(ctx, a.DB, r); e != nil {
					return e
				}
				if e = waitJob(ctx, j, id); e != nil {
					return e
				}
				if e = j.FailCloseGate(ctx); e != nil {
					return e
				}
				continue
			}
			_, missing, e := (clientops.Executor{Journal: j}).ReconcileBulkOnly(ctx, rt, job)
			if e != nil {
				return e
			}
			for _, c := range missing {
				if _, e = a.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ABORTED' WHERE generation_id=$1 AND client_id=$2 AND state='PLANNED'`, r.ID, c.ID); e != nil {
					return e
				}
			}
			if e = j.CompleteBulkRecovery(ctx, job, clientops.StateObsolete); e != nil {
				return e
			}
		}
		r.Phase = "CLEANING"
	}
	active, err := scaleActive(ctx, a.DB, r, all)
	if err != nil {
		return err
	}
	current, err := snapshot(ctx, rt, r.Inbound)
	if err != nil {
		return err
	}
	if err = scaleMatch(current, r.Baseline, active); err != nil {
		return err
	}
	start := time.Now()
	svc := usercapacity.Service{DB: a.DB, Secrets: a.Container.Secrets}
	if r.Phase == "CREATING" {
		for len(active)+len(r.Baseline) < r.Target {
			if time.Now().After(r.Expires) {
				return fmt.Errorf("durable scale deadline reached")
			}
			if _, err = health(); err != nil {
				return err
			}
			deficit := r.Target - len(active) - len(r.Baseline)
			if deficit > 100 {
				deficit = 100
			}
			n, e := svc.BulkAllowance(ctx, r.Panel, r.Inbound, 100, deficit)
			if e != nil {
				return e
			}
			if n == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
				continue
			}
			p := clientops.BulkPayload{GenerationID: r.ID, TargetUsers: r.Target}
			for i := 0; i < n; i++ {
				id, e := sanaei.UUIDv4()
				if e != nil {
					return e
				}
				p.Clients = append(p.Clients, sanaei.Client{ID: id, Email: "u-" + r.Marker + "-" + strings.ReplaceAll(id, "-", "")[:8], Enable: true, TotalGB: 104857600, ExpiryTime: r.Expires.Add(time.Hour).UnixMilli(), LimitHWID: 2, Flow: "xtls-rprx-vision"})
			}
			if err = scaleOpenBulk(ctx, a.DB, r, n); err != nil {
				return err
			}
			id, planned, e := j.ReserveBulk(ctx, rt.AccountID, r.Panel, r.Inbound, p)
			if e != nil {
				return e
			}
			if !planned {
				return fmt.Errorf("scope blocked before planning")
			}
			if err = scaleOpenMain(ctx, a.DB, r); err != nil {
				return err
			}
			if err = waitJob(ctx, j, id); err != nil {
				return err
			}
			if err = j.FailCloseGate(ctx); err != nil {
				return err
			}
			for _, c := range p.Clients {
				all[c.ID] = c
				active[c.ID] = c
			}
			current, err = snapshot(ctx, rt, r.Inbound)
			if err != nil {
				return err
			}
			if err = scaleMatch(current, r.Baseline, active); err != nil {
				return err
			}
			fmt.Printf("SCALE_CREATE run=%s job=%s batch=%d total=%d elapsed_ms=%d\n", r.ID, id, n, len(current), time.Since(start).Milliseconds())
		}
		r.Phase = "VERIFYING"
		if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET phase='VERIFYING',updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
			return err
		}
	}
	if r.Phase == "VERIFYING" {
		if len(active)+len(r.Baseline) != r.Target {
			return fmt.Errorf("target inventory incomplete")
		}
		if err = scaleOutput(ctx, a.DB, r, all, len(active)); err != nil {
			return err
		}
		h, e := health()
		if e != nil {
			return e
		}
		b, _ := json.Marshal(map[string]any{"target_observed": r.Target, "owned_output_observed": len(active), "health_at_target": h, "target_verified_at": time.Now().UTC()})
		if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET evidence=evidence||$2::jsonb,phase='CLEANING',updated_at=now() WHERE generation_id=$1`, r.ID, b); err != nil {
			return err
		}
		fmt.Printf("SCALE_TARGET_VERIFIED run=%s total=%d owned_output=%d elapsed_ms=%d\n", r.ID, r.Target, len(active), time.Since(start).Milliseconds())
		r.Phase = "CLEANING"
	}
	if r.Phase != "CLEANING" {
		return fmt.Errorf("unsupported scale phase %s", r.Phase)
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE bulk_scale_runs SET phase='CLEANING',updated_at=now() WHERE generation_id=$1`, r.ID); err != nil {
		return err
	}
	for len(active) > 0 {
		if _, err = health(); err != nil {
			return err
		}
		ids := make([]string, 0, len(active))
		for id := range active {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) > 100 {
			ids = ids[:100]
		}
		p := clientops.BulkPayload{GenerationID: r.ID, TargetUsers: r.Target}
		for _, id := range ids {
			p.Clients = append(p.Clients, active[id])
		}
		if err = scaleOpenBulk(ctx, a.DB, r, len(ids)); err != nil {
			return err
		}
		id, e := j.ReserveBulkDelete(ctx, rt.AccountID, r.Panel, r.Inbound, p)
		if e != nil {
			return e
		}
		if err = scaleOpenMain(ctx, a.DB, r); err != nil {
			return err
		}
		if err = waitJob(ctx, j, id); err != nil {
			return err
		}
		if err = j.FailCloseGate(ctx); err != nil {
			return err
		}
		for _, id := range ids {
			delete(active, id)
		}
		current, err = snapshot(ctx, rt, r.Inbound)
		if err != nil {
			return err
		}
		if err = scaleMatch(current, r.Baseline, active); err != nil {
			return err
		}
		fmt.Printf("SCALE_DELETE run=%s job=%s batch=%d remaining_owned=%d elapsed_ms=%d\n", r.ID, id, len(ids), len(active), time.Since(start).Milliseconds())
	}
	if err = scaleOutput(ctx, a.DB, r, all, 0); err != nil {
		return err
	}
	if _, err = health(); err != nil {
		return err
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_generations SET state='CLOSED',closed_at=now() WHERE id=$1`, r.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_scale_runs SET phase='SUCCEEDED',completed_at=now(),updated_at=now(),last_error='',evidence=evidence||jsonb_build_object('baseline_restored',true,'cleanup_only',$2::boolean) WHERE generation_id=$1`, r.ID, cleanup); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	raw, _ := json.Marshal(current)
	fmt.Printf("SCALE_CLOSED run=%s baseline_count=%d baseline_sha256=%x elapsed_ms=%d\n", r.ID, len(current), sha256.Sum256(raw), time.Since(start).Milliseconds())
	return nil
}

const scaleHealthCommand = `python3 - <<'PY'
import pathlib,json,time,subprocess
root=pathlib.Path('/proc');rss=0;processes=0
for p in root.iterdir():
 if not p.name.isdigit():continue
 try:
  name=(p/'comm').read_text().strip()
  if name!='x-ui' and not name.startswith('xray'):continue
  processes+=1
  for line in (p/'status').read_text().splitlines():
   if line.startswith('VmRSS:'):rss+=int(line.split()[1])
 except (FileNotFoundError,ProcessLookupError):continue
available=next(int(line.split()[1]) for line in (root/'meminfo').read_text().splitlines() if line.startswith('MemAvailable:'))
active=subprocess.run(['systemctl','is-active','--quiet','x-ui']).returncode==0
print(json.dumps(dict(time=time.time(),available_kib=available,rss_kib=rss,processes=processes,active=active)))
PY`
