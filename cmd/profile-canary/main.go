// profile-canary exercises independent policy planning on one already verified,
// unused inbound. Only the production worker may execute the durable queue.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"syscall"
	"time"
)

func main() {
	panel := flag.String("panel", "", "exact verified panel")
	inbound := flag.Int64("inbound", 0, "exact inbound")
	dir := flag.String("evidence-dir", "", "new private evidence directory")
	execute := flag.Bool("execute", false, "run scoped worker-only canary")
	flag.Parse()
	if err := run(*panel, *inbound, *dir, *execute); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type owned struct{ ID, Email, Class string }
type evidence struct {
	Panel    string
	Inbound  int64
	Started  time.Time
	Profiles []configprofiles.Profile
	Clients  map[string]clientops.LifecycleClient
	Routes   map[string]string
	Phase    string
}

func run(panel string, inbound int64, dir string, execute bool) (result error) {
	if panel == "" || inbound < 1 {
		return errors.New("exact panel/inbound required")
	}
	sig, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sig, 10*time.Minute)
	defer cancel()
	a, err := app.Bootstrap(ctx)
	if err != nil {
		return err
	}
	defer a.Close()
	manager := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: time.Second}
	rt, err := manager.Acquire(ctx, panel)
	if err != nil {
		return err
	}
	obs, port, err := clientops.LifecycleInventory(ctx, rt, inbound)
	if err != nil {
		return err
	}
	profiles, err := configprofiles.Read(ctx, a.DB, false)
	if err != nil {
		return err
	}
	var gen string
	var eligible bool
	err = a.DB.QueryRowContext(ctx, `SELECT s.generation_id::text,s.enabled AND s.use_global_policy AND s.allow_create AND s.remaining_operations>=12 AND s.expires_at>now()+interval '30 minutes' AND d.state='READY' AND d.expires_at>now()+interval '30 minutes' FROM bulk_lifecycle_scopes s JOIN panel_instances p ON p.id=s.panel_id JOIN droplets d ON d.id=p.droplet_id WHERE s.panel_id=$1 AND s.inbound_id=$2`, panel, inbound).Scan(&gen, &eligible)
	if err != nil || !eligible {
		return errors.New("inbound is not an eligible lifecycle scope")
	}
	ownership, err := readOwned(ctx, a.DB, gen)
	if err != nil {
		return err
	}
	routes := map[string]string{}
	rows, err := a.DB.QueryContext(ctx, `SELECT client_id,email,route_class FROM panel_client_routes WHERE panel_id=$1`, panel)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, email, cls string
		if err = rows.Scan(&id, &email, &cls); err != nil {
			rows.Close()
			return err
		}
		if c, ok := obs[id]; ok {
			if c.Client.Email != email {
				rows.Close()
				return errors.New("route identity mismatch")
			}
			routes[id] = cls
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err = validateBaseline(obs, ownership, profiles, port, routes); err != nil {
		return err
	}
	manual := map[string]clientops.LifecycleClient{}
	for id, c := range obs {
		manual[id] = c
	}
	for _, o := range ownership {
		delete(manual, o.ID)
	}
	fmt.Printf("BASELINE_VERIFIED port=%d direct=1 residential=1 unused=true\n", port)
	if !execute {
		return nil
	}
	if !filepath.IsAbs(dir) {
		return errors.New("absolute evidence directory required")
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return fmt.Errorf("evidence already exists; inspect prior state before recovery: %w", err)
	}
	ev := evidence{Panel: panel, Inbound: inbound, Started: time.Now().UTC(), Profiles: profiles, Clients: obs, Routes: routes, Phase: "BASELINE"}
	save := func(phase string) error {
		ev.Phase = phase
		b, e := json.MarshalIndent(ev, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "state.json"), b, 0600)
	}
	if err = save("BASELINE"); err != nil {
		return err
	}
	j := clientops.Journal{DB: a.DB}
	defer func() {
		c, cc := context.WithTimeout(context.Background(), 10*time.Second)
		defer cc()
		if e := j.FailCloseGate(c); e != nil {
			result = fmt.Errorf("gate close failed; %v; original %v", e, result)
		}
		if result != nil {
			_ = save("FAILED_READ_BEFORE_RECOVERY")
		}
	}()
	var safe bool
	if err = a.DB.QueryRowContext(ctx, `SELECT (NOT m.enabled AND m.kill_switch AND m.concurrency=1) AND (NOT b.enabled AND b.kill_switch) AND g.enabled AND NOT EXISTS(SELECT 1 FROM client_mutation_jobs WHERE state IN('PENDING','RUNNING')) FROM client_mutation_execution_gate m CROSS JOIN bulk_client_execution_gate b CROSS JOIN global_config_policies g WHERE g.policy_key='reality'`).Scan(&safe); err != nil || !safe {
		return errors.New("gates must be closed, master policy enabled and queue settled")
	}
	desired := append([]configprofiles.Profile(nil), profiles...)
	for i := range desired {
		desired[i].Target = 2
		desired[i].Rate = 1
		desired[i].Quota = 1 << 40
		if desired[i].Class == "RESIDENTIAL" {
			desired[i].Quota = 2 << 40
		}
	}
	expected, err := saveProfiles(ctx, a.DB, profiles, desired)
	if err != nil {
		return err
	}
	if err = save("SCOPED_CREATE_AND_UPDATE"); err != nil {
		return err
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE client_mutation_execution_gate SET enabled=true,kill_switch=false,concurrency=1,panel_id=$1,inbound_id=$2,updated_at=now() WHERE singleton AND NOT enabled AND kill_switch`, panel, inbound); err != nil {
		return err
	}
	wait := func(wanted []configprofiles.Profile, wantOriginal bool) error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			gate, e := j.Gate(ctx)
			if e != nil {
				return e
			}
			if !gate.Enabled || gate.KillSwitch || !gate.PanelID.Valid || gate.PanelID.String != panel || !gate.InboundID.Valid || gate.InboundID.Int64 != inbound {
				return errors.New("worker closed or changed execution gate")
			}
			var failed, pending int
			if e = a.DB.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE state='FAILED'),count(*) FILTER(WHERE state IN('PENDING','RUNNING')) FROM client_mutation_jobs WHERE panel_id=$1 AND inbound_id=$2 AND created_at>=$3`, panel, inbound, ev.Started).Scan(&failed, &pending); e != nil {
				return e
			}
			if failed > 0 {
				return errors.New("canary mutation failed; reconcile before retry")
			}
			own, e := readOwned(ctx, a.DB, gen)
			if e != nil {
				return e
			}
			counts := map[string]int{}
			for _, o := range own {
				counts[o.Class]++
			}
			ready := pending == 0
			for _, p := range wanted {
				ready = ready && counts[p.Class] == p.Target
			}
			if ready {
				live, _, e := clientops.LifecycleInventory(ctx, rt, inbound)
				if e != nil {
					return e
				}
				ready = len(live) == len(own)+len(manual)
				for id, old := range manual {
					c, ok := live[id]
					ready = ready && ok && reflect.DeepEqual(c.Client, old.Client)
				}
				byClass := map[string]configprofiles.Profile{}
				for _, p := range wanted {
					byClass[p.Class] = p
				}
				for _, o := range own {
					c, ok := live[o.ID]
					p := byClass[o.Class]
					ready = ready && ok && c.Client.Email == o.Email && c.Client.TotalGB == p.Quota && c.Client.ExpiryTime == 0 && c.Client.LimitHWID == 0 && c.Client.Enable
				}
				if ready && wantOriginal {
					ready = len(live) == len(ev.Clients)
					for id, old := range ev.Clients {
						c, ok := live[id]
						ready = ready && ok && reflect.DeepEqual(c.Client, old.Client)
					}
				}
				if ready {
					return nil
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
	if err = wait(expected, false); err != nil {
		return err
	}
	fmt.Println("SCOPED_CREATE_UPDATE_VERIFIED direct=2 residential=2 independent_quotas=true")
	// Routes are committed from ownership, not inferred from a 50/50 split.
	if err = waitRoutes(ctx, a.DB, gen, panel); err != nil {
		return err
	}
	if err = save("RESTORING_ORIGINAL_POLICY_AND_SHRINK"); err != nil {
		return err
	}
	expected, err = saveProfiles(ctx, a.DB, expected, profiles)
	if err != nil {
		return err
	}
	if err = wait(expected, true); err != nil {
		return err
	}
	if err = waitRoutes(ctx, a.DB, gen, panel); err != nil {
		return err
	}
	var outside, created, deleted int
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*) FROM client_mutation_jobs WHERE created_at>=$1 AND (panel_id<>$2 OR inbound_id<>$3)`, ev.Started, panel, inbound).Scan(&outside); err != nil {
		return err
	}
	if outside != 0 {
		return errors.New("mutation journal outside acceptance scope")
	}
	if err = a.DB.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE state='DELETED') FROM bulk_user_ownership WHERE generation_id=$1 AND created_at>=$2`, gen, ev.Started).Scan(&created, &deleted); err != nil {
		return err
	}
	if created != 2 || deleted != 2 {
		return fmt.Errorf("canary cleanup mismatch created=%d deleted=%d", created, deleted)
	}
	if err = save("PASS_BASELINE_RESTORED"); err != nil {
		return err
	}
	fmt.Println("PROFILE_CANARY_PASS new_owned=2 deleted=2 original_identities_preserved=true outside_jobs=0 gate_closes_on_exit=true")
	return nil
}

func readOwned(ctx context.Context, db *sql.DB, gen string) ([]owned, error) {
	rows, err := db.QueryContext(ctx, `SELECT client_id,email,route_class FROM bulk_user_ownership WHERE generation_id=$1 AND state='ACTIVE' ORDER BY client_id`, gen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []owned
	for rows.Next() {
		var o owned
		if err = rows.Scan(&o.ID, &o.Email, &o.Class); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}
func validateBaseline(obs map[string]clientops.LifecycleClient, own []owned, profiles []configprofiles.Profile, port int, routes map[string]string) error {
	if len(obs) < 2 || len(obs) > 3 || len(own) != 2 || len(profiles) != 2 || len(routes) != len(obs) {
		return errors.New("canary requires one owned baseline client per class, at most one preserved manual client, and two profiles")
	}
	counts := map[string]int{}
	for _, c := range obs {
		if !c.Client.Enable || c.Client.TotalGB != 0 || c.Client.ExpiryTime != 0 || c.Client.LimitHWID != 0 || c.Traffic == nil || c.Traffic.Up != 0 || c.Traffic.Down != 0 {
			return errors.New("baseline identity, limits or unused traffic precondition failed")
		}
	}
	for _, o := range own {
		c, ok := obs[o.ID]
		if !ok || c.Client.Email != o.Email || routes[o.ID] != o.Class {
			return errors.New("owned baseline class mismatch")
		}
		counts[o.Class]++
	}
	for _, p := range profiles {
		if !p.Enabled || !p.Applies(port) || p.Target != 1 || p.Quota != 0 || p.Lifetime != 0 || p.DeviceLimit != 0 || counts[p.Class] != 1 {
			return errors.New("baseline independent profile precondition failed")
		}
	}
	return nil
}
func saveProfiles(ctx context.Context, db *sql.DB, expected, desired []configprofiles.Profile) ([]configprofiles.Profile, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(628341902732)`); err != nil {
		return nil, err
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled AND NOT EXISTS(SELECT 1 FROM panel_cleanup_jobs WHERE state NOT IN('SUCCEEDED','CANCELLED')) FROM global_config_policies WHERE policy_key='reality' FOR UPDATE`).Scan(&active); err != nil || !active {
		return nil, errors.New("master policy changed or cleanup active")
	}
	live, err := configprofiles.Read(ctx, tx, true)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(live, expected) {
		return nil, errors.New("profile changed externally; refusing overwrite")
	}
	for _, p := range desired {
		raw, _ := json.Marshal(p.Ports)
		if _, err = tx.ExecContext(ctx, `UPDATE reality_config_profiles SET enabled=$2,ports=$3,target_users_per_inbound=$4,user_quota_bytes=$5,user_lifetime_seconds=$6,device_limit=$7,users_per_second=$8,revision=revision+1,updated_at=now() WHERE route_class=$1`, p.Class, p.Enabled, raw, p.Target, p.Quota, p.Lifetime, p.DeviceLimit, p.Rate); err != nil {
			return nil, err
		}
	}
	saved, err := configprofiles.Read(ctx, tx, false)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("policy outcome unknown: read before retry: %w", err)
	}
	return saved, nil
}
func waitRoutes(ctx context.Context, db *sql.DB, gen, panel string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var ok bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM panel_routing_state s JOIN residential_routing_control c ON c.singleton WHERE s.panel_id=$2 AND s.state='APPLIED' AND s.revision=c.revision AND s.verified_at>now()-interval '90 seconds') AND NOT EXISTS(SELECT 1 FROM bulk_user_ownership o LEFT JOIN panel_client_routes r ON r.panel_id=$2 AND r.client_id=o.client_id AND r.email=o.email WHERE o.generation_id=$1 AND o.state='ACTIVE' AND (r.route_class IS NULL OR r.route_class<>o.route_class))`, gen, panel).Scan(&ok)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("fresh route class verification timed out")
		case <-ticker.C:
		}
	}
}
