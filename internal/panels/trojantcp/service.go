package trojantcp

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	secretstore "github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

type Service struct {
	DB      *sql.DB
	Secrets Secrets
	SSH     interface {
		Run(context.Context, provisioning.Target, []byte, string) (string, error)
	}
	Probe    func(context.Context, string, int) (TLSProof, error)
	Runtimes *sanaei.RuntimeManager
}
type target struct {
	Panel, Account, Droplet, Host, User, Key string
	PanelPort                                int
	Trial                                    bool
}
type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ReadPolicy(ctx context.Context, db queryer) (Policy, error) {
	var p Policy
	e := db.QueryRowContext(ctx, "SELECT enabled,port,direct_enabled,residential_enabled,fleet,panel_ids,revision FROM trojan_tcp_policy WHERE singleton").Scan(&p.Enabled, &p.Port, &p.Direct, &p.Residential, &p.Fleet, pq.Array(&p.Panels), &p.Revision)
	return p, e
}

const eligible = `p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dr.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE'
 AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
 AND pol.enabled AND (pol.fleet OR p.id=ANY(pol.panel_ids))
 AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=p.id AND cj.state NOT IN('SUCCEEDED','CANCELLED'))`

func (s Service) RunOne(ctx context.Context) error {
	var delegated bool
	if s.DB != nil {
		if e := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_profiles WHERE legacy_key='trojan')").Scan(&delegated); e != nil {
			return e
		}
		if delegated {
			return nil
		}
	}
	if s.DB == nil || s.Secrets == nil || s.Runtimes == nil {
		return errors.New("trojan_service_unconfigured")
	}
	var panel string
	err := s.DB.QueryRowContext(ctx, `SELECT p.id::text FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=dr.id CROSS JOIN trojan_tcp_policy pol LEFT JOIN trojan_tcp_listeners t ON t.panel_id=p.id WHERE `+eligible+` AND (t.next_attempt_at IS NULL OR t.next_attempt_at<=now()) AND (t.remote_id IS NULL OR t.certificate_not_after<now()+interval '48 hours') AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 minutes') ORDER BY t.attempted_at NULLS FIRST,p.id LIMIT 1`).Scan(&panel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Reconcile(ctx, panel)
}
func (s Service) loadTarget(ctx context.Context, panel string) (target, error) {
	var t target
	e := s.DB.QueryRowContext(ctx, `SELECT p.id::text,p.account_id::text,p.droplet_id::text,dep.host,COALESCE(dep.profile_snapshot->>'ssh_user','root'),COALESCE(dep.profile_snapshot->>'ssh_key_secret_ref',''),x.port,COALESCE((dep.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
 FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dep ON dep.droplet_id=dr.id JOIN xui_panel_deployments x ON x.droplet_id=dr.id AND x.generation=dep.postinstall_generation CROSS JOIN trojan_tcp_policy pol WHERE p.id=$1 AND `+eligible, panel).Scan(&t.Panel, &t.Account, &t.Droplet, &t.Host, &t.User, &t.Key, &t.PanelPort, &t.Trial)
	return t, e
}
func (s Service) Reconcile(ctx context.Context, panel string) error {
	return s.reconcile(ctx, panel, false)
}

// Verification has its own bounded lane: ACME cannot starve existing listeners.
func (s Service) RunVerifyOne(ctx context.Context) error {
	var delegated bool
	if s.DB != nil {
		if e := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_profiles WHERE legacy_key='trojan')").Scan(&delegated); e != nil {
			return e
		}
		if delegated {
			return nil
		}
	}
	var panel string
	e := s.DB.QueryRowContext(ctx, `WITH candidate AS (
 SELECT t.panel_id FROM trojan_tcp_listeners t JOIN panel_instances p ON p.id=t.panel_id
 JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 JOIN deployments dep ON dep.droplet_id=dr.id CROSS JOIN trojan_tcp_policy pol
 WHERE `+eligible+` AND t.remote_id IS NOT NULL
 AND (t.verify_attempted_at IS NULL OR t.verify_attempted_at<now()-interval '30 seconds')
 AND (t.verified_at IS NULL OR t.verified_at<now()-interval '2 minutes' OR t.state<>'VERIFIED')
 ORDER BY t.verify_attempted_at NULLS FIRST,t.panel_id FOR UPDATE OF t SKIP LOCKED LIMIT 1)
 UPDATE trojan_tcp_listeners t SET verify_attempted_at=now()
 FROM candidate c WHERE t.panel_id=c.panel_id RETURNING t.panel_id::text`).Scan(&panel)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	return s.reconcile(ctx, panel, true)
}
func (s Service) reconcile(ctx context.Context, panel string, verifyOnly bool) (ret error) {
	started := time.Now()

	// Certificate issuance has its own lease. It must not hold the VLESS configuration lock.
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	var held bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", fmt.Sprintf("trojan-tcp:%t:%s", verifyOnly, panel)).Scan(&held); e != nil || !held {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		if e := conn.QueryRowContext(c, "SELECT pg_advisory_unlock(hashtextextended($1,0))", fmt.Sprintf("trojan-tcp:%t:%s", verifyOnly, panel)).Scan(&unlocked); e != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	p, e := ReadPolicy(ctx, s.DB)
	if e != nil || !p.Enabled {
		return e
	}
	t, e := s.loadTarget(ctx, panel)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	var ref string
	if !verifyOnly {
		generation, e := sanaei.UUIDv4()
		if e != nil {
			return e
		}
		ref = "trojan-tcp:" + panel + ":" + generation
		_, e = s.DB.ExecContext(ctx, `INSERT INTO trojan_tcp_listeners(panel_id,port,managed_key,credential_ref,policy_revision,attempted_at,next_attempt_at) VALUES($1,$2,$3,$4,$5,now(),now()+interval '5 minutes') ON CONFLICT(panel_id) DO UPDATE SET attempted_at=now(),next_attempt_at=now()+interval '5 minutes',updated_at=now()`, panel, p.Port, fmt.Sprintf("%s%05d", ManagedPrefix, p.Port), ref, p.Revision)
		if e != nil {
			return e
		}

	}
	stage := "preflight"
	defer func() {
		if ret != nil {
			c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			// Error codes only: native bodies and credentials never enter the ledger.
			code := "trojan_" + stage + "_failed"
			if strings.HasPrefix(ret.Error(), "trojan_") && !strings.ContainsAny(ret.Error(), " \n") {
				code = ret.Error()
			}
			_, _ = s.DB.ExecContext(c, "UPDATE trojan_tcp_listeners SET state='BLOCKED',stage=$2,error_code=$3,next_attempt_at=now()+interval '5 minutes',updated_at=now() WHERE panel_id=$1 AND (verified_at IS NULL OR verified_at<=$4)", panel, stage, code, started)
		}
	}()
	if t.Trial {
		return errors.New("trojan_trial_port_not_supported")
	}
	if net.ParseIP(t.Host) == nil || ValidatePort(p.Port, t.PanelPort, nil, nil) != nil {
		return errors.New("trojan_port_reserved")
	}
	var storedPort int
	var certUntil sql.NullTime
	var remote sql.NullInt64
	if e = s.DB.QueryRowContext(ctx, "SELECT port,remote_id,certificate_not_after,credential_ref FROM trojan_tcp_listeners WHERE panel_id=$1", panel).Scan(&storedPort, &remote, &certUntil, &ref); e != nil {
		return e
	}
	if storedPort != p.Port {
		return errors.New("trojan_port_change_requires_cleanup")
	}
	stage = "credentials"
	var credentials Credentials
	e = sanaei.WithConfigLock(ctx, s.DB, panel, func(ctx context.Context) error {
		raw, e := s.Secrets.Get(ctx, t.Account, ref)
		if errors.Is(e, secretstore.ErrSecretNotFound) {
			if remote.Valid || verifyOnly {
				return errors.New("trojan_credentials_missing")
			}
			credentials, e = NewCredentials(panel)
			if e != nil {
				return e
			}
			raw, e = json.Marshal(credentials)
			if e != nil {
				return e
			}
			if !sanaei.ConfigLockHeld(ctx, s.DB, panel) {
				return sanaei.ErrConfigOwnership
			}
			if e = s.Secrets.Put(ctx, t.Account, ref, "trojan_tcp_credentials", raw); e != nil {
				return e
			}
		} else if e != nil {
			return e
		} else if json.Unmarshal(raw, &credentials) != nil {
			return errors.New("trojan_credentials_invalid")
		}
		clients := credentials.Clients(p)
		// Ownership is durable before any native POST. Conflicting identities are never adopted.
		for _, c := range clients {
			_, e = s.DB.ExecContext(ctx, `INSERT INTO trojan_tcp_clients(panel_id,route_class,client_id,email) VALUES($1,$2,$3,$4) ON CONFLICT(panel_id,route_class) DO NOTHING`, panel, c.RouteClass, c.ID, c.Email)
			if e != nil {
				return e
			}
			var same bool
			if e = s.DB.QueryRowContext(ctx, "SELECT client_id=$3 AND email=$4 FROM trojan_tcp_clients WHERE panel_id=$1 AND route_class=$2", panel, c.RouteClass, c.ID, c.Email).Scan(&same); e != nil {
				return e
			}
			if !same {
				return errors.New("trojan_identity_conflict")
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	clients := credentials.Clients(p)
	payload, e := Build(p, t.Host, clients)
	if e != nil {
		return e
	}
	stage = "certificate"
	if !verifyOnly && (!certUntil.Valid || certUntil.Time.Before(time.Now().Add(48*time.Hour))) {
		command, e := CertificateCommand(panel, t.Host, p.Port, t.PanelPort)
		if e != nil {
			return e
		}
		key, e := s.Secrets.Get(ctx, t.Account, t.Key)
		if e != nil {
			return e
		}
		result, e := s.SSH.Run(ctx, provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, Port: 22, User: t.User, KeySecretRef: t.Key}, key, command)
		for i := range key {
			key[i] = 0
		}
		if e != nil {
			return errors.New("trojan_certificate_provision_failed")
		}
		proof, e := CertificateResult(result, t.Host)
		if e != nil {
			return e
		}
		_, e = s.DB.ExecContext(ctx, "UPDATE trojan_tcp_listeners SET state='TLS_READY',stage='certificate',certificate_not_after=$2,certificate_sha256=$3,updated_at=now() WHERE panel_id=$1", panel, proof.NotAfter, proof.SHA256)
		if e != nil {
			return e
		}
	}
	// Join the shared executor before taking the native configuration lock.
	// A contended creation must not block Reality reads or wait five minutes
	// after losing a try-lock race with another creator.
	if !verifyOnly {
		stage = "executor_wait"
		release, err := acquireCreationExecutor(ctx, conn)
		defer release()
		if err != nil {
			return err
		}
	}
	stage = "native"
	return sanaei.WithConfigLock(ctx, s.DB, panel, func(ctx context.Context) error {
		current, e := ReadPolicy(ctx, s.DB)
		if e != nil {
			return e
		}
		if !current.Enabled || current.Revision != p.Revision {
			return errors.New("trojan_policy_changed")
		}
		if _, e = s.loadTarget(ctx, panel); e != nil {
			return errors.New("trojan_target_not_eligible")
		}
		rt, e := s.Runtimes.Acquire(ctx, panel)
		if e != nil {
			return errors.New("trojan_panel_unavailable")
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error {
			raws, e := rt.Session.FreshSnapshot(ctx)
			if e != nil {
				return errors.New("trojan_inventory_unavailable")
			}
			id, e := Observe(raws, p, t.Host, clients, remote)
			if e != nil {
				return e
			}
			before := ProtectedHash(raws)
			if id == 0 {
				if verifyOnly {
					return errors.New("trojan_listener_missing")
				}
				if e = s.preflightPort(ctx, t, p.Port); e != nil {
					return e
				}
				if !sanaei.ConfigLockHeld(ctx, s.DB, panel) {
					return sanaei.ErrConfigOwnership
				}
				// Verify policy/cleanup under the shared admission fence immediately before POST.
				if e = s.admit(ctx, t, p); e != nil {
					return e
				}
				body := map[string]any{}
				for k, v := range payload {
					body[k] = v
				}
				for _, k := range []string{"settings", "streamSettings", "sniffing"} {
					v, _ := json.Marshal(body[k])
					body[k] = string(v)
				}
				b, _ := json.Marshal(body)
				_, _ = rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/inbounds/add", ContentType: "application/json", Body: b, TimeoutSeconds: 30})
				rt.Session.Invalidate()
				raws, e = rt.Session.FreshSnapshot(ctx)
				if e != nil {
					return errors.New("trojan_create_outcome_unknown")
				}
				id, e = Observe(raws, p, t.Host, clients, sql.NullInt64{})
				if e != nil {
					return e
				}
				if id == 0 {
					return errors.New("trojan_create_not_observed")
				}
				if before != ProtectedHash(raws) {
					return errors.New("trojan_other_listener_changed")
				}
			}
			probe := s.Probe
			if probe == nil {
				probe = ProbeTLS
			}
			var proof TLSProof
			for attempt := 0; attempt < 3; attempt++ {
				proof, e = probe(ctx, t.Host, p.Port)
				if e == nil || verifyOnly {
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
			if e != nil {
				return e
			}
			snap, e := sanaei.InventoryFromRaw(panel, raws)
			if e != nil {
				return e
			}
			if _, e = (inventory.SQLStore{DB: s.DB}).Sync(ctx, snap); e != nil {
				return e
			}
			return sanaei.WithConfigEvidenceTx(ctx, s.DB, panel, func(tx *sql.Tx) error {
				_, e := tx.ExecContext(ctx, "UPDATE trojan_tcp_listeners SET state='VERIFIED',stage='tls_handshake_verified',error_code='',remote_id=$2,policy_revision=$3,certificate_not_after=$4,certificate_sha256=$5,verified_at=now(),next_attempt_at=now()+interval '2 minutes',updated_at=now() WHERE panel_id=$1", panel, id, p.Revision, proof.NotAfter, proof.SHA256)
				return e
			})
		})
	})
}
func (s Service) admit(ctx context.Context, t target, p Policy) error {
	return sanaei.WithConfigEvidenceTx(ctx, s.DB, t.Panel, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(180061015)"); e != nil {
			return e
		}
		var allowed bool
		e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM trojan_tcp_policy pol WHERE pol.enabled AND pol.revision=$2 AND EXISTS(SELECT 1 FROM client_mutation_execution_gate g WHERE g.singleton AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=$1) AND g.inbound_id IS NULL) AND (pol.fleet OR $1::uuid=ANY(pol.panel_ids)) AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=$1 AND cj.state NOT IN('SUCCEEDED','CANCELLED')))`, t.Panel, p.Revision).Scan(&allowed)
		if e != nil {
			return e
		}
		if !allowed {
			return errors.New("trojan_creation_blocked")
		}
		_, e = tx.ExecContext(ctx, "UPDATE trojan_tcp_listeners SET stage='verify_required' WHERE panel_id=$1", t.Panel)
		return e
	})
}
func (s Service) preflightPort(ctx context.Context, t target, port int) error {
	key, e := s.Secrets.Get(ctx, t.Account, t.Key)
	if e != nil {
		return e
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	cmd := fmt.Sprintf(`python3 - <<'PYPORT'
import socket,shutil,subprocess
s=socket.socket();s.bind(('0.0.0.0',%d))
if shutil.which('firewall-cmd') and subprocess.run(['firewall-cmd','--state'],capture_output=True).returncode==0:raise RuntimeError('unsupported_active_firewall')
if shutil.which('ufw') and 'Status: active' in subprocess.run(['ufw','status'],capture_output=True,text=True,check=True).stdout:
 subprocess.run(['ufw','allow','%d/tcp','comment','dob-trojan'],capture_output=True,check=True)
s.close();print('free')
PYPORT`, port, port)
	_, e = s.SSH.Run(ctx, provisioning.Target{AccountID: t.Account, DropletID: t.Droplet, Host: t.Host, Port: 22, User: t.User, KeySecretRef: t.Key}, key, cmd)
	if e != nil {
		return errors.New("trojan_os_port_occupied")
	}
	return nil
}

func ProtectedHash(raws []json.RawMessage) string {
	values := []string{}
	for _, raw := range raws {
		var in map[string]json.RawMessage
		if json.Unmarshal(raw, &in) != nil {
			return "invalid"
		}
		var remark string
		_ = json.Unmarshal(in["remark"], &remark)
		if strings.HasPrefix(remark, ManagedPrefix) {
			continue
		}
		selected := map[string]json.RawMessage{}
		for _, k := range []string{"id", "tag", "protocol", "port", "listen", "enable", "settings", "streamSettings", "sniffing", "expiryTime", "total"} {
			selected[k] = in[k]
		}
		b, _ := json.Marshal(selected)
		values = append(values, string(b))
	}
	sort.Strings(values)
	b, _ := json.Marshal(values)
	return Digest(b)
}
