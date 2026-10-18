package configplane

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"net"
	"reflect"
	"strconv"
	"sync"
	"time"
)

//go:embed runtime_batch.py
var runtimeBatchScript string

// One wave of four jobs fits the 45-second worker deadline even when peers time out.

type batchCandidate struct {
	id, ref, remark, account string
	remote                   int64
	spec                     []byte
	cert                     sql.NullTime
}
type batchJob struct {
	candidate   batchCandidate
	p           Profile
	credentials configdriver.Credentials
	input       map[string]any
}
type batchResult struct {
	ID       string          `json:"profile_id"`
	Verified bool            `json:"verified"`
	Reason   string          `json:"reason"`
	Classes  map[string]bool `json:"classes"`
}

// VerifyPanel amortizes inventory, SSH and core startup across a panel's
// independent profiles. One profile or route failure never aborts its siblings.
// Each published class still requires authenticated traffic and public reachability.
func (s Service) VerifyPanel(ctx context.Context, panel, claimedProfile string) error {
	lease, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer lease.Close()
	var held bool
	if e = lease.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", "config-profile-proof:"+panel).Scan(&held); e != nil || !held {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if _, err := lease.ExecContext(c, "SELECT pg_advisory_unlock_all()"); err != nil {
			_ = lease.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	t, e := s.listenerTarget(ctx, panel)
	if e != nil {
		return e
	}
	key, e := s.Secrets.Get(ctx, t.AccountID, t.KeySecretRef)
	if e != nil {
		return e
	}
	defer clear(key)
	return s.withPanelLock(ctx, panel, func(ctx context.Context) error {
		rt, e := s.Runtimes.Acquire(ctx, panel)
		if e != nil {
			return e
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error {
			raws, e := rt.Session.FreshSnapshot(ctx)
			if e != nil {
				return e
			}
			rows, e := s.DB.QueryContext(ctx, `SELECT l.profile_id::text,l.credential_ref,l.managed_key,pi.account_id::text,l.remote_id,l.applied_spec,l.certificate_not_after
 FROM config_driver_listeners l JOIN config_driver_profiles cp ON cp.id=l.profile_id JOIN panel_instances pi ON pi.id=l.panel_id JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id
 WHERE l.panel_id=$1 AND l.remote_id IS NOT NULL AND l.applied_revision=cp.revision AND l.mutation_state='CONFIRMED' AND NOT l.needs_reconcile AND (l.next_attempt_at<=now() OR l.profile_id::text=$2) AND `+Eligible+` ORDER BY (l.profile_id::text=$2) DESC,l.next_attempt_at,l.profile_id LIMIT 4`, panel, claimedProfile)
			if e != nil {
				return e
			}
			candidates := []batchCandidate{}
			for rows.Next() {
				var c batchCandidate
				if e = rows.Scan(&c.id, &c.ref, &c.remark, &c.account, &c.remote, &c.spec, &c.cert); e != nil {
					break
				}
				candidates = append(candidates, c)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return e
			}
			jobs := []batchJob{}
			inputs := []map[string]any{}
			registry := configdriver.Builtins()
			var globals []sanaei.GlobalClient
			globalsLoaded := false
			for _, c := range candidates {
				p, e := Read(ctx, s.DB, c.id)
				if e != nil {
					return e
				}
				if p.Spec.Security == "reality" && p.Spec.SNIMode != "" {
					var fresh bool
					if err := s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_sni_selections WHERE profile_id=$1 AND panel_id=$2 AND revision=$3 AND verified_at>now()-interval '6 hours')", p.ID, panel, p.Revision).Scan(&fresh); err != nil {
						return err
					}
					if !fresh {
						_, err := s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET needs_reconcile=true,next_attempt_at=now() WHERE profile_id=$1 AND panel_id=$2", p.ID, panel)
						if err != nil {
							return err
						}
						continue
					}
				}
				raw, e := s.Secrets.Get(ctx, c.account, c.ref)
				var credentials configdriver.Credentials
				if e != nil || DecodeCredentials(raw, p, &credentials) != nil {
					s.batchFailure(ctx, c.id, panel, "profile_credentials_missing", true)
					continue
				}
				resized, e := configdriver.ResizePolicies(credentials, p.ID, panel, p.Classes, p.ClassSettings, time.Now(), p.Spec.Flow)
				if e != nil || !reflect.DeepEqual(resized, credentials) {
					_, _ = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET needs_reconcile=true,next_attempt_at=now() WHERE profile_id=$1 AND panel_id=$2", c.id, panel)
					continue
				}
				if managedClientPolicies(credentials) {
					if !globalsLoaded {
						globals, e = sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
						if e != nil {
							return errors.New("profile_global_policy_inventory_unavailable")
						}
						globalsLoaded = true
					}
					if verifyGlobalPolicies(credentials, globals) != nil {
						s.batchFailure(ctx, c.id, panel, "profile_global_policy_unverified", true)
						continue
					}
				}
				if json.Unmarshal(c.spec, &p.Spec) != nil {
					s.batchFailure(ctx, c.id, panel, "profile_saved_spec_invalid", true)
					continue
				}
				payload, e := registry.Build(p.Spec, t.Host, c.remark, credentials)
				if e != nil {
					s.batchFailure(ctx, c.id, panel, "profile_saved_spec_invalid", true)
					continue
				}
				observed, e := configdriver.Observe(raws, payload, c.remote, credentials)
				if e != nil || observed != c.remote {
					s.batchFailure(ctx, c.id, panel, "profile_observed_drift", true)
					continue
				}
				tag := ""
				for _, raw := range raws {
					var x struct {
						ID  int64
						Tag string
					}
					if json.Unmarshal(raw, &x) == nil && x.ID == c.remote {
						tag = x.Tag
					}
				}
				if tag == "" {
					s.batchFailure(ctx, c.id, panel, "profile_native_tag_missing", true)
					continue
				}
				emails := []string{}
				probes := []any{}
				classes := []string{}
				seen := map[string]bool{}
				for _, cl := range credentials.Clients {
					emails = append(emails, cl.Email)
					if !seen[cl.Class] {
						out, e := registry.Outbound(p.Spec, t.Host, credentials, cl)
						if e != nil {
							return e
						}
						probes = append(probes, out)
						classes = append(classes, cl.Class)
						seen[cl.Class] = true
					}
				}
				input := map[string]any{"profile_id": p.ID, "port": p.Spec.Port, "tag": tag, "emails": emails, "probes": probes, "probe_classes": classes, "core_hashes": profileCoreHashes()}
				jobs = append(jobs, batchJob{c, p, credentials, input})
				inputs = append(inputs, input)
				_, _ = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET stage='runtime',stage_started_at=now() WHERE profile_id=$1 AND panel_id=$2", p.ID, panel)
			}
			if len(jobs) == 0 {
				return nil
			}
			ssh, ok := s.SSH.(interface {
				RunInput(context.Context, provisioning.Target, []byte, string, []byte) (provisioning.CommandResult, error)
			})
			if !ok {
				return errors.New("profile_runtime_stdin_unavailable")
			}
			// Writers remain fenced, but independent inventory/route readers may proceed.
			if sanaei.ConfigObservationEnabled(panel) {
				if err := sanaei.ConfigProofOnly(ctx, s.DB, panel); err != nil {
					return err
				}
			}
			input, _ := json.Marshal(inputs)
			command := "python3 -c \"import base64;exec(base64.b64decode('" + base64.StdEncoding.EncodeToString([]byte(runtimeBatchScript)) + "'))\""
			out, runErr := ssh.RunInput(ctx, t.Target, key, command, input)
			var response struct {
				Results []batchResult `json:"results"`
			}
			if runErr != nil || json.Unmarshal([]byte(out.Stdout), &response) != nil {
				for _, j := range jobs {
					s.batchFailure(ctx, j.p.ID, panel, "profile_runtime_batch_unavailable", false)
				}
				return nil
			}
			expected := make([]string, 0, len(jobs))
			for _, j := range jobs {
				expected = append(expected, j.p.ID)
			}
			results, err := validateBatchResults(expected, response.Results)
			if err != nil {
				for _, j := range jobs {
					s.batchFailure(ctx, j.p.ID, panel, "profile_runtime_batch_invalid", false)
				}
				return err
			}
			// Public probes share no SQL sessions and cannot consume another panel's lane.
			public := map[string]bool{}
			var mu sync.Mutex
			var wg sync.WaitGroup
			slots := make(chan struct{}, 4)
			for _, j := range jobs {
				if !results[j.p.ID].Verified {
					continue
				}
				wg.Add(1)
				go func(j batchJob) {
					defer wg.Done()
					select {
					case slots <- struct{}{}:
					case <-ctx.Done():
						return
					}
					defer func() { <-slots }()
					probe, cancel := context.WithTimeout(ctx, 4*time.Second)
					defer cancel()
					good := false
					if j.p.Spec.Security == "tls" {
						_, e := trojantcp.ProbeTLS(probe, t.Host, j.p.Spec.Port)
						good = e == nil
					} else {
						conn, e := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort(t.Host, strconv.Itoa(j.p.Spec.Port)))
						if e == nil {
							conn.Close()
							good = true
						}
					}
					mu.Lock()
					public[j.p.ID] = good
					mu.Unlock()
				}(j)
			}
			wg.Wait()
			snapshot, e := sanaei.InventoryFromRaw(panel, raws)
			if e != nil {
				return e
			}
			if _, e = (inventory.SQLStore{DB: s.DB}).Sync(ctx, snapshot); e != nil {
				return e
			}
			for _, j := range jobs {
				proof, ok := results[j.p.ID]
				if !ok || !proof.Verified || !public[j.p.ID] {
					reason := "profile_runtime_unverified"
					if ok {
						switch proof.Reason {
						case "binary", "socket", "users", "traffic":
							reason = "profile_runtime_" + proof.Reason + "_unverified"
						}
						if proof.Verified && !public[j.p.ID] {
							reason = "profile_public_port_unreachable"
						}
					}
					s.batchFailure(ctx, j.p.ID, panel, reason, false)
					continue
				}
				next := nextProfileCheck(j.p, j.credentials, time.Now())
				partial := false
				for _, class := range j.p.Classes {
					if !proof.Classes[class] {
						partial = true
					}
				}
				code := ""
				if partial {
					code = "profile_class_traffic_retrying"
					next = time.Now().Add(10 * time.Second)
				}
				e = sanaei.WithConfigEvidenceTx(ctx, s.DB, panel, func(tx *sql.Tx) error {
					var current bool
					if e := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_profiles WHERE id=$1 AND revision=$2 AND enabled AND delete_requested_at IS NULL)", j.p.ID, j.p.Revision).Scan(&current); e != nil {
						return e
					}
					if !current {
						return nil
					}
					for _, class := range j.p.Classes {
						var verified any
						failure := "profile_runtime_traffic_unverified"
						if proof.Classes[class] {
							verified = time.Now()
							failure = ""
						}
						if _, e := tx.ExecContext(ctx, "INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at,error_code) VALUES($1,$2,$3,$4,$5) ON CONFLICT(profile_id,panel_id,route_class) DO UPDATE SET verified_at=EXCLUDED.verified_at,error_code=EXCLUDED.error_code", j.p.ID, panel, class, verified, failure); e != nil {
							return e
						}
					}
					_, e := tx.ExecContext(ctx, "UPDATE config_driver_listeners SET state='VERIFIED',stage='runtime_verified',error_code=$4,failures=0,verified_at=now(),next_attempt_at=$5 WHERE profile_id=$1 AND panel_id=$2 AND applied_revision=$3", j.p.ID, panel, j.p.Revision, code, next)
					return e
				})
				if e != nil {
					return e
				}
			}
			return nil
		})
	})
}
func (s Service) batchFailure(ctx context.Context, id, panel, code string, reconcile bool) {
	c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	_, _ = s.DB.ExecContext(c, "UPDATE config_driver_listeners SET state='BLOCKED',stage='runtime',error_code=$3,failures=failures+1,next_attempt_at=now()+LEAST(120,10*power(2,LEAST(failures,4))) * interval '1 second',needs_reconcile=needs_reconcile OR $4 OR ($3='profile_runtime_traffic_unverified' AND failures>=2 AND EXISTS(SELECT 1 FROM config_driver_profiles p WHERE p.id=$1 AND p.spec->>'sni_mode'='auto')) WHERE profile_id=$1 AND panel_id=$2", id, panel, code, reconcile)
	if code == "profile_runtime_traffic_unverified" {
		_, _ = s.DB.ExecContext(c, "DELETE FROM config_driver_sni_selections ss USING config_driver_listeners l,config_driver_profiles p WHERE ss.profile_id=$1 AND ss.panel_id=$2 AND l.profile_id=ss.profile_id AND l.panel_id=ss.panel_id AND l.needs_reconcile AND l.failures>=3 AND p.id=ss.profile_id AND p.spec->>'sni_mode'='auto'", id, panel)
	}
}
func validateBatchResults(expected []string, proofs []batchResult) (map[string]batchResult, error) {
	result := map[string]batchResult{}
	wanted := map[string]bool{}
	for _, id := range expected {
		wanted[id] = true
	}
	if len(expected) != len(proofs) {
		return nil, errors.New("profile_runtime_batch_invalid")
	}
	for _, p := range proofs {
		if !wanted[p.ID] {
			return nil, errors.New("profile_runtime_batch_invalid")
		}
		if _, ok := result[p.ID]; ok {
			return nil, errors.New("profile_runtime_batch_invalid")
		}
		for c := range p.Classes {
			if c != "DIRECT" && c != "RESIDENTIAL" {
				return nil, errors.New("profile_runtime_batch_invalid")
			}
		}
		result[p.ID] = p
	}
	return result, nil
}
