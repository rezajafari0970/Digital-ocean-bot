package residentialsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"os"
	"strings"
	"time"
)

type Secrets interface {
	sanaei.RuntimeSecrets
	GetResidential(context.Context, string, string) ([]byte, error)
}
type Service struct {
	DB       *sql.DB
	Secrets  Secrets
	Runtimes *sanaei.RuntimeManager
}

func (s Service) ReconcilePanel(ctx context.Context, p readyworker.Panel, dry bool) (err error) {
	if s.DB == nil || s.Secrets == nil || s.Runtimes == nil || p.ID == "" {
		return errors.New("residential sync configuration")
	}
	var due bool
	err = s.DB.QueryRowContext(ctx, `SELECT c.enabled AND (c.fleet OR $1::uuid=ANY(c.panel_ids)) AND
 (r.panel_id IS NULL OR r.revision<>c.revision OR r.next_check_at<=now())
 FROM residential_routing_control c LEFT JOIN panel_routing_state r ON r.panel_id=$1 WHERE c.singleton`, p.ID).Scan(&due)
	if err != nil || !due || dry {
		return err
	}

	return sanaei.WithConfigLock(ctx, s.DB, p.ID, func(ctx context.Context) (err error) {
		defer func() {
			if err != nil {
				c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				defer cancel()
				_, _ = s.DB.ExecContext(c, `INSERT INTO panel_routing_state(panel_id,state,last_error,next_check_at) VALUES($1,'FAILED','routing verification pending',now()+interval '10 seconds')
  ON CONFLICT(panel_id) DO UPDATE SET state='FAILED',last_error=excluded.last_error,next_check_at=excluded.next_check_at`, p.ID)
			}
		}()
		rt, e := s.Runtimes.Acquire(ctx, p.ID)
		if e != nil {
			return e
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error { return s.apply(ctx, p.ID, rt) })
	})
}
func (s Service) policy(ctx context.Context, panel string) (routePolicy, int64, error) {
	p := routePolicy{AdsOnly: adsOnlyPanel(panel), Harden: hardeningPanel(panel)}
	var revision int64
	var allowed bool
	err := s.DB.QueryRowContext(ctx, `SELECT c.revision,c.enabled AND (c.fleet OR $1::uuid=ANY(c.panel_ids)),g.generate_residential,g.generate_direct,(SELECT count(*) FROM residential_proxies)
 FROM residential_routing_control c CROSS JOIN global_config_policies g WHERE c.singleton AND g.policy_key='reality'`, panel).Scan(&revision, &allowed, &p.Residential, &p.Direct, &p.Configured)
	if err != nil {
		return p, 0, err
	}
	if !allowed {
		return p, 0, errors.New("routing scope closed")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT rp.proxy_id::text,rp.type,rp.host,rp.port,COALESCE(rp.username,''),rp.outbound_tag,COALESCE(rp.secret_ref,''),rp.status='healthy' AND rp.last_success_at>now()-interval '3 minutes'
 FROM residential_proxies rp
 WHERE rp.enabled AND rp.last_success_at>=rp.updated_at
 ORDER BY (rp.status='healthy' AND rp.last_success_at>now()-interval '3 minutes') DESC,rp.priority,rp.proxy_id`)
	if err != nil {
		return p, 0, err
	}
	type selected struct {
		x       rp
		ref     string
		healthy bool
	}
	var choices []selected
	for rows.Next() {
		var x selected
		if err = rows.Scan(&x.x.ID, &x.x.Type, &x.x.Host, &x.x.Port, &x.x.User, &x.x.Tag, &x.ref, &x.healthy); err != nil {
			rows.Close()
			return p, 0, err
		}
		choices = append(choices, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, 0, err
	}
	for _, v := range choices {
		if v.ref != "" {
			b, e := s.Secrets.GetResidential(ctx, v.x.ID, v.ref)
			if e != nil {
				continue
			}
			v.x.Password = string(b)
			for i := range b {
				b[i] = 0
			}
		} else if v.x.User != "" {
			continue
		}
		if v.healthy {
			p.HealthyCount++
		}
		// A previously verified endpoint remains the only protected egress when
		// its probe fails. A failed SOCKS/HTTP connection cannot fall back direct.
		// Output separately requires current health; transient health must not
		// restart every Xray or withdraw unrelated DIRECT subscriptions.
		if len(p.Proxies) == 0 {
			p.Proxies = append(p.Proxies, v.x)
		}
	}
	return p, revision, nil
}
func (s Service) apply(ctx context.Context, panel string, rt *sanaei.PanelRuntime) error {
	p, revision, err := s.policy(ctx, panel)
	if err != nil {
		return err
	}
	previous := map[string]clientRoute{}
	rows, err := s.DB.QueryContext(ctx, "SELECT client_id,email,route_class,effective_class FROM panel_client_routes WHERE panel_id=$1", panel)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c clientRoute
		if err = rows.Scan(&c.ID, &c.Email, &c.Class, &c.Effective); err != nil {
			rows.Close()
			return err
		}
		previous[c.ID] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return err
	}
	if p.AdsOnly && p.Residential && (p.Harden || p.Configured > 0) {
		if err = validateAdSniffing(raws); err != nil {
			if !p.Harden {
				return err
			}
			// Persist and apply a blocking plan instead of merely withholding Output
			// while old already-shared clients continue using unsafe routing.
			p.SniffingBlocked = true
		}
	}
	clients, tags, err := planClients(raws, previous, p)
	if err != nil {
		return err
	}
	current, testURL, err := readXraySetting(ctx, rt.Session.Exec)
	if err != nil {
		return err
	}
	desired, err := buildSettings(current, clients, tags, p)
	if err != nil {
		return err
	}
	hash := settingsHash(desired)
	var unchanged bool
	if err = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM panel_routing_state WHERE panel_id=$1 AND revision=$2 AND plan_hash=$3 AND state='APPLIED')", panel, revision, hash).Scan(&unchanged); err != nil {
		return err
	}
	if !unchanged {
		if err = s.persistPlan(ctx, panel, revision, hash, p, clients); err != nil {
			return err
		}
	}

	if err = s.checkPolicy(ctx, panel, p, revision); err != nil {
		return err
	}
	// The executor always reads the template and running routes before a retry.
	if err = applyAndVerify(ctx, rt.Session.Exec, current, desired, testURL, clients, tags, p, func() error {
		_, e := s.DB.ExecContext(ctx, "UPDATE panel_routing_state SET state='APPLYING' WHERE panel_id=$1 AND revision=$2 AND plan_hash=$3", panel, revision, hash)
		return e
	}); err != nil {
		return err
	}
	rt.Session.Invalidate()
	// Membership may have changed through a panel operator while we held our lock.
	after, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return err
	}
	confirmed, confirmedTags, err := planClients(after, routeMap(clients), p)
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(struct {
		C []clientRoute
		T []string
	}{clients, tags})
	afterJSON, _ := json.Marshal(struct {
		C []clientRoute
		T []string
	}{confirmed, confirmedTags})
	if string(beforeJSON) != string(afterJSON) {
		return errors.New("routing client inventory changed")
	}
	if err = s.checkPolicy(ctx, panel, p, revision); err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE panel_routing_state SET state='APPLIED',verified_at=now(),next_check_at=now()+interval '20 seconds',last_error='',healthy_count=$4
 WHERE panel_id=$1 AND revision=$2 AND plan_hash=$3 AND EXISTS(SELECT 1 FROM residential_routing_control WHERE singleton AND revision=$2 AND enabled AND(fleet OR $1::uuid=ANY(panel_ids)))`, panel, revision, hash, p.HealthyCount)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("routing revision changed during apply")
	}
	return nil
}
func routeMap(cs []clientRoute) map[string]clientRoute {
	m := map[string]clientRoute{}
	for _, c := range cs {
		m[c.ID] = c
	}
	return m
}

func selectedProxy(p routePolicy) string {
	if len(p.Proxies) > 0 {
		return p.Proxies[0].ID
	}
	return ""
}
func (s Service) checkPolicy(ctx context.Context, panel string, want routePolicy, revision int64) error {
	fresh, rev, err := s.policy(ctx, panel)
	if err != nil {
		return err
	}
	// Sniffing is fresh observed runtime state, not a saved policy field.
	want.SniffingBlocked = false
	want.HealthyCount = 0
	fresh.HealthyCount = 0
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(fresh)
	if rev != revision || string(a) != string(b) {
		return errors.New("routing policy or selected proxy health changed")
	}
	return nil
}
func (s Service) EligiblePanels(ctx context.Context) ([]readyworker.Panel, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT p.id::text FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id
 WHERE p.enabled AND dr.state<>'DELETED' AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=p.droplet_id AND d.state='PANEL_COMPLETE') ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ps []readyworker.Panel
	for rows.Next() {
		var p readyworker.Panel
		if err = rows.Scan(&p.ID); err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, rows.Err()
}

func (s Service) persistPlan(ctx context.Context, panel string, revision int64, hash string, p routePolicy, clients []clientRoute) error {
	// Persist exact membership before a request can change the remote router.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fresh int64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM residential_routing_control WHERE singleton FOR SHARE").Scan(&fresh); err != nil {
		return err
	}
	if fresh != revision {
		return errors.New("routing revision changed")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO panel_routing_state(panel_id,revision,plan_hash,state,configured_count,healthy_count,selected_proxy_id) VALUES($1,$2,$3,'APPLYING',$4,$5,NULLIF($6,'')::uuid)
 ON CONFLICT(panel_id) DO UPDATE SET revision=excluded.revision,plan_hash=excluded.plan_hash,state='APPLYING',configured_count=excluded.configured_count,healthy_count=excluded.healthy_count,selected_proxy_id=excluded.selected_proxy_id,last_error=''`, panel, revision, hash, p.Configured, p.HealthyCount, selectedProxy(p)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM panel_client_routes WHERE panel_id=$1", panel); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn("panel_client_routes", "panel_id", "client_id", "email", "route_class", "effective_class", "revision"))
	if err != nil {
		return err
	}
	for _, c := range clients {
		if _, err = stmt.ExecContext(ctx, panel, c.ID, c.Email, c.Class, c.Effective, revision); err != nil {
			stmt.Close()
			return err
		}
	}
	_, err = stmt.ExecContext(ctx)
	closeErr := stmt.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// NextDuePanel selects one task without a fleet-wide completion barrier.
// The independent lanes share the existing panel/config lock and together run
// at most two requests. Backoff and freshness are evaluated in the database.
func (s Service) NextDuePanel(ctx context.Context, serving bool) (readyworker.Panel, bool, error) {
	var p readyworker.Panel
	err := s.DB.QueryRowContext(ctx, `SELECT p.id::text FROM panel_instances p
 JOIN droplets dr ON dr.id=p.droplet_id CROSS JOIN residential_routing_control c
 LEFT JOIN panel_routing_state r ON r.panel_id=p.id
 LEFT JOIN worker_item_failures f ON f.kind='residential_sync' AND f.item_id=p.id::text
 WHERE c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids))
 AND p.enabled AND dr.state<>'DELETED'
 AND ((dr.state IN ('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now()))=$1)
 AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=p.droplet_id AND d.state='PANEL_COMPLETE')
 AND (r.panel_id IS NULL OR r.revision<>c.revision OR r.next_check_at<=now())
 AND (f.next_retry_at IS NULL OR f.next_retry_at<=now())
 ORDER BY COALESCE(r.next_check_at,'-infinity'::timestamptz),p.id LIMIT 1`, serving).Scan(&p.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	return p, err == nil, err
}

// Optional deployment-only canary allowlist. Unset means the complete new
// policy. Other panels keep their exact old plan while the canary is tested.
func adsOnlyPanel(panel string) bool {
	scope := strings.TrimSpace(os.Getenv("DOB_RESIDENTIAL_ADS_ONLY_PANELS"))
	if scope == "" {
		return true
	}
	for _, id := range strings.Split(scope, ",") {
		if strings.TrimSpace(id) == panel {
			return true
		}
	}
	return false
}

// Deployment-only scoped rollout. An unset scope enables the verified policy
// fleet-wide; a nonmatching panel retains its prior exact plan.
func hardeningPanel(panel string) bool {
	scope := strings.TrimSpace(os.Getenv("DOB_RESIDENTIAL_HARDENING_PANELS"))
	if scope == "" {
		return true
	}
	for _, id := range strings.Split(scope, ",") {
		if strings.TrimSpace(id) == panel {
			return true
		}
	}
	return false
}
