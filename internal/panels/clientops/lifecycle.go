package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sort"
	"strings"
	"time"
)

type LifecyclePolicy struct {
	Class                                                string
	Revision                                             int64
	Target                                               int
	Quota                                                int64
	Lifetime, DeviceLimit, Rate, Chunk, CreationInterval int
	Create                                               bool
}
type lifecycleScope struct {
	Generation, Marker string
	Global             bool
	Policy             LifecyclePolicy
}
type lifecycleOwned struct {
	ID, Email string
	Created   time.Time
}

func isLifecycle(raw json.RawMessage) bool {
	var x struct{ Lifecycle bool }
	return json.Unmarshal(raw, &x) == nil && x.Lifecycle
}

func readLifecycleScope(ctx context.Context, tx *sql.Tx, panel string, inbound int64) (lifecycleScope, error) {
	var s lifecycleScope
	err := tx.QueryRowContext(ctx, `SELECT s.generation_id::text,g.marker,s.use_global_policy,s.allow_create,s.target_users,s.quota_bytes,s.lifetime_seconds,s.device_limit,s.users_per_second,s.max_batch_size FROM bulk_lifecycle_scopes s JOIN bulk_user_generations g ON g.id=s.generation_id JOIN bulk_lifecycle_control c ON c.singleton JOIN client_mutation_execution_gate eg ON eg.singleton JOIN panel_instances p ON p.id=s.panel_id JOIN droplets d ON d.id=p.droplet_id JOIN deployments dep ON dep.droplet_id=d.id JOIN accounts a ON a.id=p.account_id WHERE s.panel_id=$1 AND s.inbound_id=$2 AND c.enabled AND s.enabled AND s.remaining_operations>0 AND s.expires_at>now() AND g.state='ACTIVE' AND g.panel_id=s.panel_id AND g.inbound_id=s.inbound_id AND eg.enabled AND NOT eg.kill_switch AND eg.concurrency=1 AND (eg.panel_id IS NULL OR eg.panel_id=s.panel_id) AND (eg.inbound_id IS NULL OR eg.inbound_id=s.inbound_id) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now()+interval '10 seconds') AND dep.state='PANEL_COMPLETE' FOR SHARE OF s,g,c,p,d,a,dep,eg`, panel, inbound).Scan(&s.Generation, &s.Marker, &s.Global, &s.Policy.Create, &s.Policy.Target, &s.Policy.Quota, &s.Policy.Lifetime, &s.Policy.DeviceLimit, &s.Policy.Rate, &s.Policy.Chunk)
	if err != nil {
		return s, err
	}
	if s.Global {
		var enabled bool
		err = tx.QueryRowContext(ctx, `SELECT enabled,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second FROM global_config_policies WHERE policy_key='reality' FOR SHARE`).Scan(&enabled, &s.Policy.Target, &s.Policy.Quota, &s.Policy.Lifetime, &s.Policy.DeviceLimit, &s.Policy.Rate)
		s.Policy.Create = s.Policy.Create && enabled
	}
	if s.Policy.Target < 0 || s.Policy.Target > 10000 || s.Policy.Rate < 1 || s.Policy.Rate > 100 || s.Policy.Quota < 0 || s.Policy.Lifetime < 0 || s.Policy.DeviceLimit < 0 {
		return s, ErrInvalidRequest
	}
	return s, err
}

// PlanLifecycle only observes and commits durable work. The worker is the sole
// mutation executor. The scoped transaction lock serializes competing planners.
func (j Journal) PlanLifecycle(ctx context.Context, rt *sanaei.PanelRuntime, inbound int64, allowance func(context.Context, int, int) (int, error)) (bool, error) {
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,941))`, fmt.Sprintf("%s:%d", rt.PanelID, inbound)).Scan(&locked); err != nil || !locked {
		return false, err
	}
	scope, err := readLifecycleScope(ctx, tx, rt.PanelID, inbound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var blocked bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND state IN ('PENDING','RUNNING','FAILED')) OR EXISTS(SELECT 1 FROM bulk_user_ownership WHERE generation_id=$3 AND state IN ('PLANNED','DELETE_PENDING'))`, rt.PanelID, inbound, scope.Generation).Scan(&blocked)
	if err != nil || blocked {
		return false, err
	}
	observed, port, err := LifecycleInventory(ctx, rt, inbound)
	if err != nil {
		return false, err
	}
	if scope.Global {
		var wanted bool
		err = tx.QueryRowContext(ctx, `SELECT ports @> to_jsonb(ARRAY[$1::integer]) FROM global_config_policies WHERE policy_key='reality'`, port).Scan(&wanted)
		if err != nil || !wanted {
			return false, err
		}
	}
	if scope.Global {
		return j.planProfiles(ctx, tx, rt, inbound, scope, observed, port, allowance)
	}
	now := time.Now()
	active := 0
	for _, o := range observed {
		reason, e := o.InactiveReason(now)
		if e != nil {
			return false, e
		}
		if reason == "" {
			active++
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT client_id,email,created_at FROM bulk_user_ownership WHERE generation_id=$1 AND state='ACTIVE' ORDER BY created_at,client_id FOR UPDATE`, scope.Generation)
	if err != nil {
		return false, err
	}
	var owned []lifecycleOwned
	for rows.Next() {
		var o lifecycleOwned
		if err = rows.Scan(&o.ID, &o.Email, &o.Created); err != nil {
			rows.Close()
			return false, err
		}
		owned = append(owned, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	var missing []sanaei.Client
	for _, o := range owned {
		if !validBulkEmail(scope.Marker, sanaei.Client{ID: o.ID, Email: o.Email}) {
			return false, ErrClientConflict
		}
		if c, ok := observed[o.ID]; ok {
			if c.Client.Email != o.Email {
				return false, ErrClientConflict
			}
		} else {
			missing = append(missing, sanaei.Client{ID: o.ID, Email: o.Email})
		}
	}
	if len(missing) > 0 {
		global, err := globalWanted(ctx, rt, inbound, missing)
		if err != nil {
			return false, err
		}
		if len(global) > 0 {
			return false, fmt.Errorf("%w: global-only owned client", ErrVerify)
		}
		for _, c := range missing {
			if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETED',deleted_at=now() WHERE generation_id=$1 AND client_id=$2 AND state='ACTIVE'`, scope.Generation, c.ID); err != nil {
				return false, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET last_observed_at=now(),active_users=$3,last_error='',updated_at=now() WHERE panel_id=$1 AND inbound_id=$2`, rt.PanelID, inbound, active)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit,last_error,observed_at) VALUES($1,$2,$3,$4,$5,GREATEST($4::integer-$5::integer,0),'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET target_users=excluded.target_users,active_users=excluded.active_users,deficit=excluded.deficit,last_error='',observed_at=now()`, rt.PanelID, inbound, port, scope.Policy.Target, active)
	if err != nil {
		return false, err
	}
	p := BulkPayload{Lifecycle: true, Policy: scope.Policy, GenerationID: scope.Generation, TargetUsers: max(1, len(observed)), CleanupReasons: map[string]string{}}
	// Expired/disabled/quota-exhausted owned identities are handled before drift.
	for _, o := range owned {
		c, ok := observed[o.ID]
		if !ok {
			continue
		}
		reason, e := c.InactiveReason(now)
		if e != nil {
			return false, e
		}
		if reason != "" && len(p.Clients) < scope.Policy.Chunk {
			v := c.Client
			v.Enable = true
			p.Clients = append(p.Clients, v)
			p.CleanupReasons[o.ID] = reason
		}
	}
	excess := active - scope.Policy.Target
	if len(p.Clients) == 0 && excess > 0 {
		for i := len(owned) - 1; i >= 0 && len(p.Clients) < min(excess, scope.Policy.Chunk); i-- {
			o := owned[i]
			c, ok := observed[o.ID]
			if !ok {
				continue
			}
			reason, _ := c.InactiveReason(now)
			if reason == "" {
				p.Clients = append(p.Clients, c.Client)
				p.CleanupReasons[o.ID] = "EXCESS"
			}
		}
	}
	if len(p.Clients) > 0 {
		raw, _ := json.Marshal(p)
		id, e := insertLifecycleJob(ctx, tx, rt, inbound, KindBulkDelete, "", raw)
		if e != nil {
			return false, e
		}
		for _, c := range p.Clients {
			if _, e = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE generation_id=$1 AND client_id=$2 AND state='ACTIVE'`, scope.Generation, c.ID); e != nil {
				return false, e
			}
		}
		_ = id
		return true, tx.Commit()
	}
	for _, o := range owned {
		c, ok := observed[o.ID]
		if !ok {
			continue
		}
		expiry := int64(0)
		if scope.Policy.Lifetime > 0 {
			expiry = o.Created.Add(time.Duration(scope.Policy.Lifetime) * time.Second).UnixMilli()
		}
		if c.Client.TotalGB == scope.Policy.Quota && c.Client.ExpiryTime == expiry && c.Client.LimitHWID == scope.Policy.DeviceLimit {
			continue
		}
		patch := ClientPatch{TotalGB: &scope.Policy.Quota, ExpiryTime: &expiry, LimitHWID: &scope.Policy.DeviceLimit}
		raw, _ := json.Marshal(map[string]any{"Lifecycle": true, "GenerationID": scope.Generation, "ExpectedEmail": o.Email, "Patch": patch, "Policy": scope.Policy})
		if _, err = insertLifecycleJob(ctx, tx, rt, inbound, KindUpdate, o.ID, raw); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if scope.Policy.Create && active < scope.Policy.Target {
		n := min(scope.Policy.Target-active, scope.Policy.Chunk, 10000-len(observed))
		if n <= 0 {
			return false, ErrClientConflict
		}
		n, err = allowance(ctx, scope.Policy.Rate, n)
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, tx.Commit()
		}
		expiry := int64(0)
		if scope.Policy.Lifetime > 0 {
			expiry = now.Add(time.Duration(scope.Policy.Lifetime) * time.Second).UnixMilli()
		}
		p.TargetUsers = len(observed) + n
		p.CleanupReasons = nil
		for i := 0; i < n; i++ {
			id, e := sanaei.UUIDv4()
			if e != nil {
				return false, e
			}
			email := "u-" + scope.Marker + "-" + strings.ReplaceAll(id, "-", "")[:8]
			p.Clients = append(p.Clients, sanaei.Client{ID: id, Email: email, Enable: true, TotalGB: scope.Policy.Quota, ExpiryTime: expiry, LimitHWID: scope.Policy.DeviceLimit, Flow: "xtls-rprx-vision"})
		}
		raw, _ := json.Marshal(p)
		jobID, e := insertLifecycleJob(ctx, tx, rt, inbound, KindBulkCreate, "", raw)
		if e != nil {
			return false, e
		}
		for _, c := range p.Clients {
			if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_user_ownership(generation_id,client_id,email,state,mutation_job_id,created_at) VALUES($1,$2,$3,'PLANNED',$4,$5)`, scope.Generation, c.ID, c.Email, jobID, now); err != nil {
				return false, err
			}
		}
		return true, tx.Commit()
	}
	return false, tx.Commit()
}
func insertLifecycleJob(ctx context.Context, tx *sql.Tx, rt *sanaei.PanelRuntime, inbound int64, kind Kind, client string, raw []byte) (string, error) {
	id, err := sanaei.UUIDv4()
	if err != nil {
		return "", err
	}
	if client == "" {
		client = id
	}
	var job string
	err = tx.QueryRowContext(ctx, `INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`, rt.AccountID, rt.PanelID, inbound, client, string(kind), "lifecycle:"+id, raw).Scan(&job)
	return job, err
}

func lifecycleGeneration(job Job) string {
	var p struct{ GenerationID string }
	_ = json.Unmarshal(job.Payload, &p)
	return p.GenerationID
}
func (e Executor) lifecyclePreflight(ctx context.Context, job Job, generation string) error {
	release, err := e.lifecycleFence(ctx, job, generation, false)
	if err == nil {
		release()
	}
	return err
}
func (e Executor) lifecycleFence(ctx context.Context, job Job, generation string, checkPolicy bool) (func(), error) {
	tx, err := e.Journal.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var id, marker string
	var useGlobal bool
	err = tx.QueryRowContext(ctx, `SELECT m.id::text,gen.marker,s.use_global_policy FROM client_mutation_jobs m JOIN bulk_lifecycle_scopes s ON s.panel_id=m.panel_id AND s.inbound_id=m.inbound_id JOIN bulk_lifecycle_control c ON c.singleton JOIN bulk_user_generations gen ON gen.id=s.generation_id JOIN client_mutation_execution_gate g ON g.singleton JOIN panel_instances p ON p.id=m.panel_id JOIN droplets d ON d.id=p.droplet_id JOIN deployments dep ON dep.droplet_id=d.id JOIN accounts a ON a.id=p.account_id WHERE m.id=$1 AND m.state='RUNNING' AND m.attempts=$2 AND m.payload->>'Lifecycle'='true' AND s.generation_id=$3 AND gen.state='ACTIVE' AND gen.panel_id=m.panel_id AND gen.inbound_id=m.inbound_id AND c.enabled AND s.enabled AND s.expires_at>now() AND COALESCE(jsonb_array_length(m.payload->'Clients'),1)<=s.max_batch_size AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=m.panel_id) AND (g.inbound_id IS NULL OR g.inbound_id=m.inbound_id) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND d.state IN ('READY','EXPIRING') AND dep.state='PANEL_COMPLETE' AND (d.expires_at IS NULL OR d.expires_at>now()+interval '10 seconds') FOR SHARE OF m,s,c,gen,g,p,d,a,dep`, job.ID, job.Attempts, generation).Scan(&id, &marker, &useGlobal)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrExecutionGated
		}
		return nil, err
	}
	if useGlobal {
		var key string
		if err = tx.QueryRowContext(ctx, `SELECT policy_key FROM global_config_policies WHERE policy_key='reality' FOR SHARE`).Scan(&key); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if checkPolicy && job.Kind != KindBulkDelete {
		var live LifecyclePolicy
		err = tx.QueryRowContext(ctx, `SELECT allow_create,target_users,quota_bytes,lifetime_seconds,device_limit,users_per_second,max_batch_size FROM bulk_lifecycle_scopes WHERE panel_id=$1 AND inbound_id=$2`, job.PanelID, job.InboundID).Scan(&live.Create, &live.Target, &live.Quota, &live.Lifetime, &live.DeviceLimit, &live.Rate, &live.Chunk)
		if err == nil && useGlobal {
			var enabled bool
			err = tx.QueryRowContext(ctx, `SELECT enabled,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second FROM global_config_policies WHERE policy_key='reality'`).Scan(&enabled, &live.Target, &live.Quota, &live.Lifetime, &live.DeviceLimit, &live.Rate)
			live.Create = live.Create && enabled
		}
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		var planned struct{ Policy LifecyclePolicy }
		if json.Unmarshal(job.Payload, &planned) != nil {
			tx.Rollback()
			return nil, ErrInvalidRequest
		}
		if useGlobal {
			var portAllowed bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM panel_inbound_inventory i CROSS JOIN global_config_policies p WHERE i.panel_id=$1 AND i.remote_id=$2 AND i.present AND i.enabled AND p.policy_key='reality' AND p.ports @> to_jsonb(ARRAY[i.port]))`, job.PanelID, job.InboundID).Scan(&portAllowed); err != nil {
				tx.Rollback()
				return nil, err
			}
			if !portAllowed {
				tx.Rollback()
				return nil, ErrLifecycleSuperseded
			}
		}
		if useGlobal {
			if err = checkProfilePolicy(ctx, tx, job, live); err != nil {
				tx.Rollback()
				return nil, err
			}
		} else if live != planned.Policy {
			tx.Rollback()
			return nil, ErrLifecycleSuperseded
		}
	}
	var clients []sanaei.Client
	if job.Kind == KindUpdate {
		var p struct{ ExpectedEmail string }
		_ = json.Unmarshal(job.Payload, &p)
		clients = []sanaei.Client{{ID: job.ClientID, Email: p.ExpectedEmail}}
	} else {
		var p BulkPayload
		if json.Unmarshal(job.Payload, &p) != nil || p.Validate() != nil {
			tx.Rollback()
			return nil, ErrInvalidRequest
		}
		clients = p.Clients
	}
	var plannedClass struct{ Policy LifecyclePolicy }
	_ = json.Unmarshal(job.Payload, &plannedClass)
	for _, cl := range clients {
		if !validBulkEmail(marker, cl) {
			tx.Rollback()
			return nil, ErrClientConflict
		}
		if useGlobal && plannedClass.Policy.Class != "" {
			var cls string
			if err = tx.QueryRowContext(ctx, `SELECT route_class FROM bulk_user_ownership WHERE generation_id=$1 AND client_id=$2 FOR SHARE`, generation, cl.ID).Scan(&cls); err != nil || cls != plannedClass.Policy.Class {
				tx.Rollback()
				return nil, ErrClientConflict
			}
		}
		var valid bool
		err = tx.QueryRowContext(ctx, `SELECT true FROM bulk_user_ownership WHERE generation_id=$1 AND client_id=$2 AND email=$3 AND (($4='UPDATE' AND state='ACTIVE') OR ($4='BULK_CREATE' AND state IN ('PLANNED','ACTIVE') AND mutation_job_id=$5) OR ($4='BULK_DELETE' AND state IN ('DELETE_PENDING','DELETED'))) FOR SHARE`, generation, cl.ID, cl.Email, string(job.Kind), job.ID).Scan(&valid)
		if err != nil || !valid {
			tx.Rollback()
			if err == nil {
				err = ErrClientConflict
			}
			return nil, err
		}
	}
	return func() { tx.Rollback() }, nil
}

func (e Executor) lifecycleDeleteGuard(ctx context.Context, rt *sanaei.PanelRuntime, job Job, p BulkPayload, present []sanaei.Client) error {
	obs, port, err := LifecycleInventory(ctx, rt, job.InboundID)
	if err != nil {
		return err
	}
	var useGlobal bool
	if err = e.Journal.DB.QueryRowContext(ctx, `SELECT use_global_policy FROM bulk_lifecycle_scopes WHERE panel_id=$1 AND inbound_id=$2`, job.PanelID, job.InboundID).Scan(&useGlobal); err != nil {
		return err
	}
	if useGlobal {
		return e.profileDeleteGuard(ctx, rt, job, p, present, obs, port)
	}
	now := time.Now()
	active := 0
	for _, c := range obs {
		r, e := c.InactiveReason(now)
		if e != nil {
			return e
		}
		if r == "" {
			active++
		}
	}
	var target int
	err = e.Journal.DB.QueryRowContext(ctx, `SELECT CASE WHEN s.use_global_policy THEN p.target_users_per_inbound ELSE s.target_users END FROM bulk_lifecycle_scopes s CROSS JOIN global_config_policies p WHERE s.panel_id=$1 AND s.inbound_id=$2 AND p.policy_key='reality'`, job.PanelID, job.InboundID).Scan(&target)
	if err != nil {
		return err
	}
	excess := 0
	for _, c := range present {
		o, ok := obs[c.ID]
		if !ok || o.Client.Email != c.Email {
			return ErrClientConflict
		}
		reason, e := o.InactiveReason(now)
		if e != nil {
			return e
		}
		expected := p.CleanupReasons[c.ID]
		if expected == "EXCESS" {
			if reason != "" {
				return ErrClientConflict
			}
			excess++
		} else if expected == "" || reason == "" {
			return ErrClientConflict
		}
	}
	if excess > max(active-target, 0) {
		return ErrClientConflict
	}
	return nil
}

// Stable scope ordering is useful for bounded rollout tooling.
func (j Journal) LifecycleInbounds(ctx context.Context, panel string) ([]int64, error) {
	rows, err := j.DB.QueryContext(ctx, `SELECT s.inbound_id FROM bulk_lifecycle_scopes s JOIN client_mutation_execution_gate g ON g.singleton WHERE s.panel_id=$1 AND s.enabled AND s.remaining_operations>0 AND s.expires_at>now() AND g.enabled AND NOT g.kill_switch AND (g.panel_id IS NULL OR g.panel_id=s.panel_id) AND (g.inbound_id IS NULL OR g.inbound_id=s.inbound_id)`, panel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, rows.Err()
}

// This result can only be raised before a provider POST. CREATE has just
// reconciled its immutable identities; only freshly absent PLANNED rows abort.
var ErrLifecycleSuperseded = errors.New("lifecycle plan superseded before POST")

func (j Journal) supersedeLifecycle(ctx context.Context, job Job) error {
	if !isLifecycle(job.Payload) || (job.Kind != KindBulkCreate && job.Kind != KindUpdate) {
		return ErrInvalidRequest
	}
	tx, err := j.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `UPDATE client_mutation_jobs SET state='OBSOLETE',completed_at=now(),next_retry_at=NULL,last_error='policy superseded before POST',result=result||jsonb_build_object('phase','SUPERSEDED_BEFORE_POST'),updated_at=now() WHERE id=$1 AND state='RUNNING' AND attempts=$2 RETURNING id::text`, job.ID, job.Attempts).Scan(&id)
	if err != nil {
		return err
	}
	if job.Kind == KindBulkCreate {
		_, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ABORTED' WHERE mutation_job_id=$1 AND state='PLANNED'`, job.ID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
