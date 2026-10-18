package configplane

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	secretstore "github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	DB       *sql.DB
	Secrets  trojantcp.Secrets
	Runtimes *sanaei.RuntimeManager
	SSH      interface {
		Run(context.Context, provisioning.Target, []byte, string) (string, error)
	}
	Verify       func(context.Context, provisioning.Target, []byte, map[string]any) error
	VerifyAbsent func(context.Context, provisioning.Target, []byte, int) error
	ProbeTLS     func(context.Context, string, int) (trojantcp.TLSProof, error)
}
type target struct {
	provisioning.Target
	Panel     string
	PanelPort int
	Trial     bool
}

func (s Service) target(ctx context.Context, id, panel string) (target, error) {
	t := target{Panel: panel}
	e := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,dep.host,COALESCE(dep.profile_snapshot->>'ssh_user','root'),COALESCE(dep.profile_snapshot->>'ssh_key_secret_ref',''),x.port,COALESCE((dep.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
 FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id JOIN xui_panel_deployments x ON x.droplet_id=dr.id AND x.generation=dep.postinstall_generation
 WHERE cp.id=$1 AND pi.id=$2 AND `+Eligible, id, panel).Scan(&t.AccountID, &t.DropletID, &t.Host, &t.User, &t.KeySecretRef, &t.PanelPort, &t.Trial)
	t.Port = 22
	return t, e
}
func (s Service) RunOne(ctx context.Context) error       { return s.run(ctx, false) }
func (s Service) RunVerifyOne(ctx context.Context) error { return s.run(ctx, true) }
func (s Service) run(ctx context.Context, verify bool) error {
	// Enrolment is idempotent. Claim due rows atomically so parallel workers do
	// not repeatedly select the same slow server.
	if !verify {
		if e := s.initializeEmpty(ctx); e != nil {
			return e
		}
		if _, e := s.DB.ExecContext(ctx, `INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec)
 SELECT cp.id,pi.id,'dob:profile:'||cp.id::text,'config-profile:'||cp.id::text||':'||pi.id::text,cp.spec
 FROM config_driver_profiles cp CROSS JOIN panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id
 JOIN accounts a ON a.id=pi.account_id JOIN deployments dep ON dep.droplet_id=dr.id WHERE `+Eligible+`
 ON CONFLICT DO NOTHING`); e != nil {
			return e
		}
		if e := s.finishRetired(ctx); e != nil {
			return e
		}
	}
	lane := `(l.needs_reconcile OR NOT cp.enabled OR cp.delete_requested_at IS NOT NULL OR NOT(cp.fleet OR l.panel_id=ANY(cp.panel_ids)) OR l.remote_id IS NULL OR l.applied_revision<>cp.revision OR l.mutation_state IN('PREPARED','SENT') OR (cp.spec->>'security'='tls' AND l.certificate_not_after<now()+interval '48 hours'))`
	if verify {
		lane = `cp.enabled AND cp.delete_requested_at IS NULL AND (cp.fleet OR l.panel_id=ANY(cp.panel_ids)) AND l.remote_id IS NOT NULL AND l.applied_revision=cp.revision AND l.mutation_state NOT IN('PREPARED','SENT') AND NOT l.needs_reconcile`
	}
	var id, panel string
	claim, e := sanaei.UUIDv4()
	if e != nil {
		return e
	}
	e = s.DB.QueryRowContext(ctx, `WITH candidate AS (
 SELECT l.profile_id,l.panel_id FROM config_driver_listeners l JOIN config_driver_profiles cp ON cp.id=l.profile_id
 JOIN panel_instances pi ON pi.id=l.panel_id JOIN droplets dr ON dr.id=pi.droplet_id
 WHERE l.next_attempt_at<=now() AND (l.lease_until IS NULL OR l.lease_until<now()) AND NOT EXISTS(SELECT 1 FROM config_driver_listeners busy WHERE busy.panel_id=l.panel_id AND busy.lease_until>now()) AND cp.deleted_at IS NULL AND (l.state<>'RETIRED' OR (cp.enabled AND cp.delete_requested_at IS NULL AND (cp.fleet OR l.panel_id=ANY(cp.panel_ids))))
 AND dr.state<>'DELETED' AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=l.panel_id AND ct.state<>'SUCCEEDED' AND cj.state NOT IN('SUCCEEDED','CANCELLED'))
 AND `+lane+` ORDER BY CASE WHEN l.attempted_at IS NOT NULL AND l.next_attempt_at<now()-interval '90 seconds' THEN 0 ELSE 1 END,cp.last_scheduled_at NULLS FIRST,l.next_attempt_at,l.attempted_at NULLS FIRST,l.profile_id,l.panel_id FOR UPDATE OF l,cp SKIP LOCKED LIMIT 1), claimed AS (
 UPDATE config_driver_listeners l SET attempted_at=now(),next_attempt_at=now()+interval '30 seconds',lease_until=now()+interval '3 minutes',claim_token=$1
 FROM candidate c WHERE l.profile_id=c.profile_id AND l.panel_id=c.panel_id RETURNING l.profile_id,l.panel_id), touched AS (UPDATE config_driver_profiles cp SET last_scheduled_at=now() FROM claimed c WHERE cp.id=c.profile_id RETURNING cp.id)
 SELECT profile_id::text,panel_id::text FROM claimed`, claim).Scan(&id, &panel)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_, _ = s.DB.ExecContext(c, "UPDATE config_driver_listeners SET lease_until=NULL,claim_token=NULL WHERE profile_id=$1 AND panel_id=$2 AND claim_token=$3", id, panel, claim)
	}()
	if verify && s.Verify == nil {
		return s.VerifyPanel(ctx, panel, id)
	}
	return s.Reconcile(ctx, id, panel)
}
func (s Service) Reconcile(ctx context.Context, id, panel string) (ret error) {
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	lease := "config-profile:" + id + ":" + panel
	var held bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", lease).Scan(&held); e != nil || !held {
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(c, "SELECT pg_advisory_unlock_all()"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	p, e := Read(ctx, s.DB, id)
	if e != nil || p.DeletedAt != nil {
		return e
	}
	var existingMutation, existingOperation string
	var mutationProfile []byte
	var mutationCredential sql.NullString
	err := s.DB.QueryRowContext(ctx, "SELECT mutation_state,mutation_profile,mutation_credential_ref,mutation_operation FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", id, panel).Scan(&existingMutation, &mutationProfile, &mutationCredential, &existingOperation)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingOperation == "DELETE" && existingMutation == "SENT" {
		return s.retireListener(ctx, conn, p, panel)
	}
	recovering := existingMutation == "SENT"
	if recovering {
		if json.Unmarshal(mutationProfile, &p) != nil {
			return errors.New("profile_journal_invalid")
		}
	}
	desired, e := Read(ctx, s.DB, id)
	if e != nil {
		return e
	}
	removing := !desired.Enabled || desired.DeleteRequestedAt != nil || (!desired.Fleet && !containsPanel(desired.Panels, panel))
	if removing && !recovering {
		return s.retireListener(ctx, conn, desired, panel)
	}
	t, e := s.listenerTarget(ctx, panel)
	if !recovering {
		t, e = s.target(ctx, id, panel)
	}
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if e = Validate(p); e != nil {
		return e
	}
	stage := "prepare"
	setStage := func(value string) {
		stage = value
		_, _ = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET stage=$3,stage_started_at=now() WHERE profile_id=$1 AND panel_id=$2 AND stage<>$3", id, panel, value)
	}
	defer func() {
		if ret == nil {
			return
		}
		code := "profile_" + stage + "_failed"
		if errors.Is(ret, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "profile_" + stage + "_timeout"
		}
		if strings.HasPrefix(ret.Error(), "profile_") && !strings.ContainsAny(ret.Error(), " \n") {
			code = ret.Error()
		}
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if code == "profile_panel_busy" {
			_, _ = s.DB.ExecContext(c, "UPDATE config_driver_listeners SET next_attempt_at=now()+interval '2 seconds' WHERE profile_id=$1 AND panel_id=$2", id, panel)
			return
		}
		_, _ = s.DB.ExecContext(c, "UPDATE config_driver_listeners SET state='BLOCKED',stage=$3,error_code=$4,failures=failures+1,next_attempt_at=now()+LEAST(120,10*power(2,LEAST(failures,4))) * interval '1 second' WHERE profile_id=$1 AND panel_id=$2", id, panel, stage, code)
		Event(c, s.DB, p, panel, stage, code)
	}()
	generation, e := sanaei.UUIDv4()
	if e != nil {
		return e
	}
	rawSpec, _ := json.Marshal(p.Spec)
	ref := "config-profile:" + id + ":" + panel + ":" + generation
	remark := configdriver.ManagedPrefix + id
	_, e = s.DB.ExecContext(ctx, `INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,credential_ref,applied_spec,attempted_at,next_attempt_at)
 VALUES($1,$2,$3,$4,$5,now(),now()+interval '30 seconds')
 ON CONFLICT(profile_id,panel_id) DO UPDATE SET attempted_at=now(),next_attempt_at=now()+interval '30 seconds'`, id, panel, remark, ref, rawSpec)
	if e != nil {
		return e
	}
	if t.Trial {
		return errors.New("profile_trial_port_not_supported")
	}
	if p.Spec.Port == t.PanelPort {
		return errors.New("profile_panel_port_conflict")
	}
	var remote sql.NullInt64
	var applied configdriver.Spec
	var appliedRaw, mutationRaw []byte
	var mutation string
	var mutationRevision sql.NullInt64
	var cert sql.NullTime
	if e = s.DB.QueryRowContext(ctx, "SELECT managed_key,credential_ref,remote_id,applied_spec,mutation_state,mutation_spec,mutation_revision,certificate_not_after FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2", id, panel).Scan(&remark, &ref, &remote, &appliedRaw, &mutation, &mutationRaw, &mutationRevision, &cert); e != nil {
		return e
	}
	if json.Unmarshal(appliedRaw, &applied) != nil {
		return errors.New("profile_saved_spec_invalid")
	}
	if mutation == "SENT" {
		if json.Unmarshal(mutationRaw, &p.Spec) != nil || !mutationRevision.Valid {
			return errors.New("profile_journal_invalid")
		}
		p.Revision = mutationRevision.Int64
	}
	if !recovering && p.Spec.Security == "reality" && p.Spec.SNIMode != "" {
		setStage("sni_scan")
		p.Spec, e = s.resolveSNI(ctx, p, panel)
		if e != nil {
			return e
		}
	}
	var credentials, priorCredentials configdriver.Credentials
	setStage("credentials")
	e = s.withPanelLock(ctx, panel, func(ctx context.Context) error {
		raw, err := s.Secrets.Get(ctx, t.AccountID, ref)
		if errors.Is(err, secretstore.ErrSecretNotFound) {
			if remote.Valid || mutation == "SENT" {
				return errors.New("profile_credentials_missing")
			}
			priorCredentials, err = configdriver.NewCredentials(p.ID, panel, p.Classes, 1)
			priorCredentials.Clients = nil
			if err == nil {
				priorCredentials, err = configdriver.ResizePolicies(priorCredentials, p.ID, panel, p.Classes, p.ClassSettings, time.Now(), p.Spec.Flow)
			}
			if err != nil {
				return err
			}
			raw, _ = json.Marshal(priorCredentials)
			if err = s.Secrets.Put(ctx, t.AccountID, ref, "config_profile_credentials", raw); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err = DecodeCredentials(raw, p, &priorCredentials); err != nil {
			return err
		}
		credentials = priorCredentials
		if recovering {
			if !mutationCredential.Valid {
				return errors.New("profile_journal_invalid")
			}
			raw, err = s.Secrets.Get(ctx, t.AccountID, mutationCredential.String)
			if err != nil {
				return err
			}
			if err = DecodeCredentials(raw, p, &credentials); err != nil {
				return err
			}
			ref = mutationCredential.String
		} else {
			resized, err := configdriver.ResizePolicies(priorCredentials, id, panel, p.Classes, p.ClassSettings, time.Now(), p.Spec.Flow)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(resized, priorCredentials) {
				nextRef := "config-profile:" + id + ":" + panel + ":generation:" + generation
				raw, err = s.Secrets.Get(ctx, t.AccountID, nextRef)
				if errors.Is(err, secretstore.ErrSecretNotFound) {
					credentials = resized
					raw, _ = json.Marshal(credentials)
					if err = s.Secrets.Put(ctx, t.AccountID, nextRef, "config_profile_credentials", raw); err != nil {
						return err
					}
				} else if err != nil {
					return err
				} else if err = DecodeCredentials(raw, p, &credentials); err != nil {
					return err
				}
				ref = nextRef
			}

		}
		for _, c := range credentials.Clients {
			if _, err = s.DB.ExecContext(ctx, "INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class) VALUES($1,$2,$3,$4,$5) ON CONFLICT(panel_id,client_id) DO NOTHING", id, panel, c.ID, c.Email, c.Class); err != nil {
				return err
			}
			var same bool
			if err = s.DB.QueryRowContext(ctx, "SELECT profile_id=$3 AND email=$4 AND route_class=$5 FROM config_driver_clients WHERE panel_id=$1 AND client_id=$2", panel, c.ID, id, c.Email, c.Class).Scan(&same); err != nil {
				return err
			}
			if !same {
				return errors.New("profile_identity_conflict")
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	registry := configdriver.Builtins()
	payload, e := registry.Build(p.Spec, t.Host, remark, credentials)
	if e != nil {
		return e
	}
	key, e := s.Secrets.Get(ctx, t.AccountID, t.KeySecretRef)
	if e != nil {
		return e
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	if p.Spec.Security == "tls" && (!cert.Valid || cert.Time.Before(time.Now().Add(48*time.Hour))) {
		setStage("certificate")
		command, e := trojantcp.ProfileCertificateCommand(panel, t.Host, p.Spec.Port, t.PanelPort)
		if e != nil {
			return e
		}
		out, e := s.SSH.Run(ctx, t.Target, key, command)
		if e != nil {
			return errors.New("profile_certificate_provision_failed")
		}
		proof, e := trojantcp.CertificateResult(out, t.Host)
		if e != nil {
			return errors.New("profile_certificate_unverified")
		}
		cert = sql.NullTime{Time: proof.NotAfter, Valid: true}
		if _, e = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET certificate_not_after=$3 WHERE profile_id=$1 AND panel_id=$2", id, panel, proof.NotAfter); e != nil {
			return e
		}
	}
	// The global executor is acquired before the panel lock, matching legacy writers.
	// A short queue timeout prevents a slow/offline profile from starving siblings.
	setStage("executor_wait")
	wantsWrite := !remote.Valid || applied != p.Spec || mutation == "PREPARED" || mutation == "SENT" || !reflect.DeepEqual(credentials, priorCredentials)
	var retiredClients bool
	if e = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_client_retirements WHERE profile_id=$1 AND panel_id=$2 AND confirmed_absent_at IS NULL)", id, panel).Scan(&retiredClients); e != nil {
		return e
	}
	wantsWrite = wantsWrite || retiredClients
	executorHeld := false
	// SENT recovery is observation-only: it must never queue behind writers.
	var unified bool
	if e = s.DB.QueryRowContext(ctx, "SELECT unified FROM config_control_authority WHERE singleton").Scan(&unified); e != nil {
		return e
	}
	if wantsWrite && mutation != "SENT" && !unified {
		wait, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, e = conn.ExecContext(wait, "SELECT pg_advisory_lock(628341902731)")
		cancel()
		if e != nil {
			return e
		}
		executorHeld = true
	}
	setStage("native")
	return s.withPanelLock(ctx, panel, func(ctx context.Context) error {
		current, e := Read(ctx, s.DB, id)
		if e != nil {
			return e
		}
		if !current.Enabled && !recovering {
			return errors.New("profile_disabled")
		}
		if !recovering && current.Revision != p.Revision {
			return errors.New("profile_revision_changed")
		}
		if _, e = s.target(ctx, id, panel); e != nil && !recovering {
			return errors.New("profile_target_not_eligible")
		}
		rt, e := s.Runtimes.Acquire(ctx, panel)
		if e != nil {
			return errors.New("profile_panel_unavailable")
		}
		return rt.WithMutation(ctx, func(ctx context.Context) error {
			raws, e := rt.Session.FreshSnapshot(ctx)
			if e != nil {
				return errors.New("profile_inventory_unavailable")
			}
			observed, matchErr := configdriver.Observe(raws, payload, remote.Int64, credentials)
			before := configdriver.ProtectedHash(raws, remark)
			if matchErr != nil || observed == 0 {
				if !wantsWrite {
					return errors.New("profile_observed_drift")
				}
				if mutation == "SENT" {
					return errors.New("profile_write_outcome_unconfirmed")
				}
				if remote.Valid {
					prior, e := registry.Build(applied, t.Host, remark, priorCredentials)
					if e != nil {
						return e
					}
					got, e := configdriver.Observe(raws, prior, remote.Int64, priorCredentials)
					if e != nil || got != remote.Int64 {
						return errors.New("profile_existing_state_conflict")
					}
				} else if matchErr != nil {
					return matchErr
				}
				if !remote.Valid || applied.Port != p.Spec.Port {
					setStage("port")
					if e = s.checkPort(ctx, t, key, p.Spec.Port); e != nil {
						return e
					}
				}
				if !sanaei.ConfigLockHeld(ctx, s.DB, panel) {
					return sanaei.ErrConfigOwnership
				}
				setStage("admission")
				e = sanaei.WithConfigEvidenceTx(ctx, s.DB, panel, func(tx *sql.Tx) error {
					if _, e := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(180061015)"); e != nil {
						return e
					}
					var allowed bool
					e := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM config_driver_profiles cp WHERE cp.id=$1 AND cp.enabled AND cp.delete_requested_at IS NULL AND cp.revision=$3 AND (cp.fleet OR $2::uuid=ANY(cp.panel_ids))
      AND EXISTS(SELECT 1 FROM client_mutation_execution_gate g WHERE g.singleton AND g.enabled AND NOT g.kill_switch AND g.concurrency=1 AND (g.panel_id IS NULL OR g.panel_id=$2) AND g.inbound_id IS NULL)
      AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=$2 AND ct.state<>'SUCCEEDED' AND cj.state NOT IN('SUCCEEDED','CANCELLED')))`, id, panel, p.Revision).Scan(&allowed)
					if e != nil {
						return e
					}
					if !allowed {
						return errors.New("profile_creation_blocked")
					}
					spec, _ := json.Marshal(p.Spec)
					frozen, _ := json.Marshal(p)
					_, e = tx.ExecContext(ctx, "UPDATE config_driver_listeners SET state='APPLYING',stage='write_prepared',mutation_state='PREPARED',mutation_spec=$3,mutation_revision=$4,mutation_profile=$5,mutation_credential_ref=$6,mutation_operation='UPSERT',verified_at=NULL WHERE profile_id=$1 AND panel_id=$2", id, panel, spec, p.Revision, frozen, ref)
					return e
				})
				if e != nil {
					return e
				}
				setStage("write")
				// Mark SENT durably before POST. After cancellation/restart only observation
				// may confirm it; another blind POST is never authorized by a timeout.
				if _, e = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET mutation_state='SENT',stage='write_sent' WHERE profile_id=$1 AND panel_id=$2", id, panel); e != nil {
					return e
				}
				body := configdriver.PanelBody(payload)
				path := "panel/api/inbounds/add"
				if remote.Valid {
					body["id"] = remote.Int64
					path = "panel/api/inbounds/update/" + strconv.FormatInt(remote.Int64, 10)
				}
				for _, k := range []string{"settings", "streamSettings", "sniffing"} {
					v, _ := json.Marshal(body[k])
					body[k] = string(v)
				}
				b, _ := json.Marshal(body)
				_, _ = rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: path, ContentType: "application/json", Body: b, TimeoutSeconds: 30})
				rt.Session.Invalidate()
				raws, e = rt.Session.FreshSnapshot(ctx)
				if e != nil {
					return errors.New("profile_write_outcome_unconfirmed")
				}
				observed, e = configdriver.Observe(raws, payload, remote.Int64, credentials)
				if e != nil || observed == 0 {
					return errors.New("profile_write_outcome_unconfirmed")
				}
				if before != configdriver.ProtectedHash(raws, remark) {
					return errors.New("profile_sibling_changed")
				}
				Event(ctx, s.DB, p, panel, "native_observed", "")
			}
			setStage("client_policy")
			if e = s.applyGlobalPolicies(ctx, panel, rt.Session.Exec, credentials); e != nil {
				return e
			}
			// Native confirmation and traffic proof are different durable phases.
			// Failed traffic never leaves an already-observed write in SENT forever.
			nativeSnap, err := sanaei.InventoryFromRaw(panel, raws)
			if err != nil {
				return err
			}
			if _, err = (inventory.SQLStore{DB: s.DB}).Sync(ctx, nativeSnap); err != nil {
				return err
			}
			if e = s.confirmNative(ctx, p, panel, observed, remark, ref, credentials, cert); e != nil {
				return e
			}
			if wantsWrite {
				if e = s.retireUnusedClients(ctx, rt, id, panel, raws); e != nil {
					return e
				}
			}
			if removing {
				return nil
			}
			if s.Verify == nil { // Runtime/route proofs have their own scheduling lane.
				_, e = s.DB.ExecContext(ctx, "UPDATE config_driver_listeners SET state='APPLYING',stage='routes_pending',stage_started_at=now(),needs_reconcile=false,error_code='',failures=0,next_attempt_at=now()+interval '3 seconds' WHERE profile_id=$1 AND panel_id=$2", id, panel)
				return e
			}

			for _, raw := range raws {
				var native struct {
					ID  int64
					Tag string
				}
				if json.Unmarshal(raw, &native) == nil && native.ID == observed {
					payload["tag"] = native.Tag
				}
			}
			if payload["tag"] == nil || payload["tag"] == "" {
				return errors.New("profile_native_tag_missing")
			}
			// Native writes and sibling checks are complete. Keep the panel lock for
			// proof consistency, but let unrelated panels use the shared writer.
			if executorHeld {
				var unlocked bool
				if e = conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock(628341902731)").Scan(&unlocked); e != nil {
					return e
				}
				if !unlocked {
					return errors.New("profile_executor_ownership_lost")
				}
				executorHeld = false
			}
			probes := []any{}
			probeClasses := []string{}
			seenClasses := map[string]bool{}
			for _, cl := range credentials.Clients {
				if !seenClasses[cl.Class] {
					out, e := registry.Outbound(p.Spec, t.Host, credentials, cl)
					if e != nil {
						return e
					}
					probes = append(probes, out)
					probeClasses = append(probeClasses, cl.Class)
					seenClasses[cl.Class] = true
				}
			}
			payload["probe_outbounds"] = probes
			payload["probe_classes"] = probeClasses
			if sanaei.ConfigObservationEnabled(panel) {
				if e = sanaei.ConfigProofOnly(ctx, s.DB, panel); e != nil {
					return e
				}
			}
			setStage("runtime")
			verifier := s.Verify
			if verifier == nil {
				verifier = s.verifyRuntime
			}
			if e = verifier(ctx, t.Target, key, payload); e != nil {
				if s.Verify == nil {
					return e
				}
				return errors.New("profile_runtime_unverified")
			}
			if p.Spec.Security == "tls" {
				probe := s.ProbeTLS
				if probe == nil {
					probe = trojantcp.ProbeTLS
				}
				proof, e := probe(ctx, t.Host, p.Spec.Port)
				if e != nil {
					return errors.New("profile_tls_handshake_failed")
				}
				cert = sql.NullTime{Time: proof.NotAfter, Valid: true}
			} else {
				cert = sql.NullTime{}
			}
			snap, e := sanaei.InventoryFromRaw(panel, raws)
			if e != nil {
				return e
			}
			if _, e = (inventory.SQLStore{DB: s.DB}).Sync(ctx, snap); e != nil {
				return e
			}
			spec, _ := json.Marshal(p.Spec)
			verifiedClasses, hasClassProofs := payload["verified_classes"].([]string)
			if !hasClassProofs && s.Verify != nil {
				verifiedClasses = probeClasses
			}
			proofSet := map[string]bool{}
			for _, cl := range verifiedClasses {
				proofSet[cl] = true
			}
			partial := false
			for _, cl := range probeClasses {
				if !proofSet[cl] {
					partial = true
				}
			}
			next := nextProfileCheck(p, credentials, time.Now())
			code := ""
			if partial {
				code = "profile_class_traffic_retrying"
				if due := time.Now().Add(10 * time.Second); due.Before(next) {
					next = due
				}
			}
			return sanaei.WithConfigEvidenceTx(ctx, s.DB, panel, func(tx *sql.Tx) error {
				for _, class := range p.Classes {
					var verified any
					failure := "profile_runtime_traffic_unverified"
					if proofSet[class] {
						verified = time.Now()
						failure = ""
					}
					if _, e := tx.ExecContext(ctx, "INSERT INTO config_driver_class_proofs(profile_id,panel_id,route_class,verified_at,error_code) VALUES($1,$2,$3,$4,$5) ON CONFLICT(profile_id,panel_id,route_class) DO UPDATE SET verified_at=EXCLUDED.verified_at,error_code=EXCLUDED.error_code", id, panel, class, verified, failure); e != nil {
						return e
					}
				}
				if p.Legacy == "trojan" {
					if _, e := tx.ExecContext(ctx, "UPDATE trojan_tcp_listeners SET state='RETIRED' WHERE panel_id=$1", panel); e != nil {
						return e
					}
				}
				_, e := tx.ExecContext(ctx, "UPDATE config_driver_listeners SET state='VERIFIED',stage='runtime_verified',needs_reconcile=false,error_code=$8,failures=0,remote_id=$3,applied_spec=$4,applied_revision=$5,mutation_state='CONFIRMED',certificate_not_after=$6,verified_at=now(),next_attempt_at=$7 WHERE profile_id=$1 AND panel_id=$2", id, panel, observed, spec, p.Revision, cert, next, code)
				return e
			})
		})
	})
}
func DecodeCredentials(raw []byte, p Profile, c *configdriver.Credentials) error {
	if json.Unmarshal(raw, c) == nil && (len(c.Clients) > 0 || c.LastCreated != nil) {
		return nil
	}
	if p.Legacy != "trojan" {
		return errors.New("profile_credentials_invalid")
	}
	var old trojantcp.Credentials
	if json.Unmarshal(raw, &old) != nil {
		return errors.New("profile_credentials_invalid")
	}
	c.Clients = nil
	for _, cl := range p.Classes {
		v := old.Direct
		if cl == "RESIDENTIAL" {
			v = old.Residential
		}
		if v.ID == "" || v.Password == "" {
			return errors.New("profile_credentials_invalid")
		}
		c.Clients = append(c.Clients, configdriver.Client{ID: v.ID, Password: v.Password, Email: v.Email, Class: cl})
	}
	return nil
}
func (s Service) checkPort(ctx context.Context, t target, key []byte, port int) error {
	if configdriver.ValidatePort(port) != nil || port == t.PanelPort {
		return errors.New("profile_port_reserved")
	}
	command := fmt.Sprintf(`python3 - <<'PY'
import socket,shutil,subprocess
s=socket.socket();s.bind(('0.0.0.0',%d))
if shutil.which('firewall-cmd') and subprocess.run(['firewall-cmd','--state'],capture_output=True).returncode==0:raise RuntimeError('unsupported_active_firewall')
if shutil.which('ufw') and 'Status: active' in subprocess.run(['ufw','status'],capture_output=True,text=True,check=True).stdout:
 subprocess.run(['ufw','allow','%d/tcp','comment','dob-config-profile'],capture_output=True,check=True)
s.close();print('free')
PY`, port, port)
	if _, e := s.SSH.Run(ctx, t.Target, key, command); e != nil {
		return errors.New("profile_os_port_occupied")
	}
	return nil
}
