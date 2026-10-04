package clientops

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"strings"
	"time"
)

type profileRateKey struct{}

func ProfileRateClass(ctx context.Context) string {
	v, _ := ctx.Value(profileRateKey{}).(string)
	return v
}
func profilePolicy(p configprofiles.Profile, s lifecycleScope) LifecyclePolicy {
	return LifecyclePolicy{Class: p.Class, Revision: p.Revision, Target: p.Target, Quota: p.Quota, Lifetime: p.Lifetime, DeviceLimit: p.DeviceLimit, Rate: p.Rate, Chunk: s.Policy.Chunk, Create: s.Policy.Create && p.Enabled}
}

// Class is durable before POST. Unknown manual clients are conservatively
// residential, never relabelled direct to satisfy a capacity target.
func observedClasses(ctx context.Context, db configprofiles.Queryer, panel string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT client_id,route_class FROM panel_client_routes WHERE panel_id=$1
 UNION ALL SELECT o.client_id,o.route_class FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND o.route_class<>'' AND o.state IN('ACTIVE','PLANNED','DELETE_PENDING')`, panel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var id, cls string
		if err = rows.Scan(&id, &cls); err != nil {
			return nil, err
		}
		if old := result[id]; old != "" && old != cls {
			return nil, ErrClientConflict
		}
		result[id] = cls
	}
	return result, rows.Err()
}

func (j Journal) planProfiles(ctx context.Context, tx *sql.Tx, rt *sanaei.PanelRuntime, inbound int64, scope lifecycleScope, observed map[string]LifecycleClient, port int, allowance func(context.Context, int, int) (int, error)) (bool, error) {
	profiles, err := configprofiles.Read(ctx, tx, true)
	if err != nil {
		return false, err
	}
	byClass := map[string]LifecyclePolicy{}
	for _, v := range profiles {
		if v.Applies(port) {
			byClass[v.Class] = profilePolicy(v, scope)
		}
	}
	classes, err := observedClasses(ctx, tx, rt.PanelID)
	if err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT client_id,email,created_at,route_class FROM bulk_user_ownership WHERE generation_id=$1 AND state='ACTIVE' ORDER BY created_at,client_id FOR UPDATE`, scope.Generation)
	if err != nil {
		return false, err
	}
	type ownedClient struct {
		lifecycleOwned
		Class string
	}
	var owned []ownedClient
	for rows.Next() {
		var o ownedClient
		if err = rows.Scan(&o.ID, &o.Email, &o.Created, &o.Class); err != nil {
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
	for i := range owned {
		o := &owned[i]
		if !validBulkEmail(scope.Marker, sanaei.Client{ID: o.ID, Email: o.Email}) {
			return false, ErrClientConflict
		}
		c, ok := observed[o.ID]
		if !ok {
			missing = append(missing, sanaei.Client{ID: o.ID, Email: o.Email})
			continue
		}
		if c.Client.Email != o.Email {
			return false, ErrClientConflict
		}
		if o.Class == "" {
			o.Class = classes[o.ID]
			if o.Class == "" {
				o.Class = "RESIDENTIAL"
			}
			if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET route_class=$3 WHERE generation_id=$1 AND client_id=$2 AND route_class=''`, scope.Generation, o.ID, o.Class); err != nil {
				return false, err
			}
		}
		if cls := classes[o.ID]; cls != "" && cls != o.Class {
			return false, ErrClientConflict
		}
		classes[o.ID] = o.Class
	}
	if len(missing) > 0 {
		global, e := globalWanted(ctx, rt, inbound, missing)
		if e != nil {
			return false, e
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
	now := time.Now()
	active := map[string]int{}
	reasons := map[string]string{}
	total := 0
	for id, c := range observed {
		reason, e := c.InactiveReason(now)
		if e != nil {
			return false, e
		}
		reasons[id] = reason
		if reason == "" {
			cls := classes[id]
			if cls == "" {
				cls = "RESIDENTIAL"
			}
			active[cls]++
			total++
		}
	}
	target := configprofiles.Target(profiles, port)
	deficit := 0
	for cls, p := range byClass {
		if p.Create {
			deficit += max(p.Target-active[cls], 0)
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bulk_lifecycle_scopes SET last_observed_at=now(),active_users=$3,last_error='',updated_at=now() WHERE panel_id=$1 AND inbound_id=$2`, rt.PanelID, inbound, total); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO user_capacity_snapshots(panel_id,inbound_id,port,target_users,active_users,deficit,last_error,observed_at) VALUES($1,$2,$3,$4,$5,$6,'',now()) ON CONFLICT(panel_id,inbound_id) DO UPDATE SET target_users=excluded.target_users,active_users=excluded.active_users,deficit=excluded.deficit,last_error='',observed_at=now()`, rt.PanelID, inbound, port, target, total, deficit); err != nil {
		return false, err
	}
	p := BulkPayload{Lifecycle: true, Policy: scope.Policy, GenerationID: scope.Generation, TargetUsers: max(1, len(observed)), CleanupReasons: map[string]string{}}
	for _, o := range owned {
		c, ok := observed[o.ID]
		if !ok {
			continue
		}
		if reason := reasons[o.ID]; reason != "" && len(p.Clients) < scope.Policy.Chunk {
			v := c.Client
			v.Enable = true
			p.Clients = append(p.Clients, v)
			p.CleanupReasons[o.ID] = reason
		}
	}
	// A paused or moved profile retains existing identities. Shrink only the
	// class whose enabled policy explicitly reduced its target on this port.
	if len(p.Clients) == 0 {
		excess := map[string]int{}
		for cls, pol := range byClass {
			if pol.Create {
				excess[cls] = max(active[cls]-pol.Target, 0)
			}
		}
		for i := len(owned) - 1; i >= 0 && len(p.Clients) < scope.Policy.Chunk; i-- {
			o := owned[i]
			c, ok := observed[o.ID]
			if ok && reasons[o.ID] == "" && excess[o.Class] > 0 {
				p.Clients = append(p.Clients, c.Client)
				p.CleanupReasons[o.ID] = "EXCESS"
				excess[o.Class]--
			}
		}
	}
	if len(p.Clients) > 0 {
		raw, _ := json.Marshal(p)
		if _, err = insertLifecycleJob(ctx, tx, rt, inbound, KindBulkDelete, "", raw); err != nil {
			return false, err
		}
		for _, c := range p.Clients {
			if _, err = tx.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='DELETE_PENDING' WHERE generation_id=$1 AND client_id=$2 AND state='ACTIVE'`, scope.Generation, c.ID); err != nil {
				return false, err
			}
		}
		return true, tx.Commit()
	}
	for _, o := range owned {
		c, ok := observed[o.ID]
		pol, exists := byClass[o.Class]
		if !ok || !exists || !pol.Create {
			continue
		}
		expiry := int64(0)
		if pol.Lifetime > 0 {
			expiry = o.Created.Add(time.Duration(pol.Lifetime) * time.Second).UnixMilli()
		}
		if c.Client.TotalGB == pol.Quota && c.Client.ExpiryTime == expiry && c.Client.LimitHWID == pol.DeviceLimit {
			continue
		}
		patch := ClientPatch{TotalGB: &pol.Quota, ExpiryTime: &expiry, LimitHWID: &pol.DeviceLimit}
		raw, _ := json.Marshal(map[string]any{"Lifecycle": true, "GenerationID": scope.Generation, "ExpectedEmail": o.Email, "Patch": patch, "Policy": pol})
		if _, err = insertLifecycleJob(ctx, tx, rt, inbound, KindUpdate, o.ID, raw); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	for _, profile := range profiles {
		pol, ok := byClass[profile.Class]
		if !ok || !pol.Create || active[pol.Class] >= pol.Target {
			continue
		}
		n := min(pol.Target-active[pol.Class], pol.Chunk, 10000-len(observed))
		if n <= 0 {
			return false, ErrClientConflict
		}
		n, err = allowance(context.WithValue(ctx, profileRateKey{}, pol.Class), pol.Rate, n)
		if err != nil {
			return false, err
		}
		if n == 0 {
			continue
		}
		p.Policy = pol
		p.TargetUsers = len(observed) + n
		p.CleanupReasons = nil
		p.Clients = nil
		expiry := int64(0)
		if pol.Lifetime > 0 {
			expiry = now.Add(time.Duration(pol.Lifetime) * time.Second).UnixMilli()
		}
		for i := 0; i < n; i++ {
			id, e := sanaei.UUIDv4()
			if e != nil {
				return false, e
			}
			email := "u-" + scope.Marker + "-" + strings.ReplaceAll(id, "-", "")[:8]
			p.Clients = append(p.Clients, sanaei.Client{ID: id, Email: email, Enable: true, TotalGB: pol.Quota, ExpiryTime: expiry, LimitHWID: pol.DeviceLimit, Flow: "xtls-rprx-vision"})
		}
		raw, _ := json.Marshal(p)
		jobID, e := insertLifecycleJob(ctx, tx, rt, inbound, KindBulkCreate, "", raw)
		if e != nil {
			return false, e
		}
		for _, c := range p.Clients {
			if _, err = tx.ExecContext(ctx, `INSERT INTO bulk_user_ownership(generation_id,client_id,email,state,mutation_job_id,created_at,route_class) VALUES($1,$2,$3,'PLANNED',$4,$5,$6)`, scope.Generation, c.ID, c.Email, jobID, now, pol.Class); err != nil {
				return false, err
			}
		}
		return true, tx.Commit()
	}
	return false, tx.Commit()
}

func checkProfilePolicy(ctx context.Context, tx *sql.Tx, job Job, live LifecyclePolicy) error {
	var planned struct{ Policy LifecyclePolicy }
	if json.Unmarshal(job.Payload, &planned) != nil {
		return ErrInvalidRequest
	}
	if planned.Policy.Class == "" {
		return ErrLifecycleSuperseded
	}
	profiles, err := configprofiles.Read(ctx, tx, true)
	if err != nil {
		return err
	}
	var port int
	if err = tx.QueryRowContext(ctx, `SELECT port FROM panel_inbound_inventory WHERE panel_id=$1 AND remote_id=$2 AND present AND enabled`, job.PanelID, job.InboundID).Scan(&port); err != nil {
		return err
	}
	for _, p := range profiles {
		if p.Class == planned.Policy.Class && p.Applies(port) {
			if profilePolicy(p, lifecycleScope{Policy: live}) == planned.Policy {
				return nil
			}
			return ErrLifecycleSuperseded
		}
	}
	return ErrLifecycleSuperseded
}

func (e Executor) profileDeleteGuard(ctx context.Context, rt *sanaei.PanelRuntime, job Job, p BulkPayload, present []sanaei.Client, obs map[string]LifecycleClient, port int) error {
	profiles, err := configprofiles.Read(ctx, e.Journal.DB, false)
	if err != nil {
		return err
	}
	classes, err := observedClasses(ctx, e.Journal.DB, job.PanelID)
	if err != nil {
		return err
	}
	active := map[string]int{}
	now := time.Now()
	for id, c := range obs {
		reason, e := c.InactiveReason(now)
		if e != nil {
			return e
		}
		if reason == "" {
			cls := classes[id]
			if cls == "" {
				cls = "RESIDENTIAL"
			}
			active[cls]++
		}
	}
	excess := map[string]int{}
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
			if reason != "" || classes[c.ID] == "" {
				return ErrClientConflict
			}
			excess[classes[c.ID]]++
		} else if expected == "" || reason == "" {
			return ErrClientConflict
		}
	}
	for cls, n := range excess {
		allowed := 0
		for _, profile := range profiles {
			if profile.Class == cls && profile.Enabled && profile.Applies(port) {
				allowed = max(active[cls]-profile.Target, 0)
			}
		}
		if n > allowed {
			return ErrClientConflict
		}
	}
	return nil
}
