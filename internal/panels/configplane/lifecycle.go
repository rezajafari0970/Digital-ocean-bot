package configplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func containsPanel(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (s Service) listenerTarget(ctx context.Context, panel string) (target, error) {
	t := target{Panel: panel}
	e := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,dep.host,
 COALESCE(dep.profile_snapshot->>'ssh_user','root'),COALESCE(dep.profile_snapshot->>'ssh_key_secret_ref',''),x.port,
 COALESCE((dep.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
 FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id JOIN deployments dep ON dep.droplet_id=dr.id
 JOIN xui_panel_deployments x ON x.droplet_id=dr.id AND x.generation=dep.postinstall_generation
 WHERE pi.id=$1 AND dr.state<>'DELETED' AND dep.state='PANEL_COMPLETE'`, panel).Scan(&t.AccountID, &t.DropletID, &t.Host, &t.User, &t.KeySecretRef, &t.PanelPort, &t.Trial)
	t.Port = 22
	return t, e
}

func (s Service) confirmNative(ctx context.Context, p Profile, panel string, remote int64, remark, ref string, credentials configdriver.Credentials, cert sql.NullTime) error {
	spec, _ := json.Marshal(p.Spec)
	ids := []string{}
	for _, c := range credentials.Clients {
		ids = append(ids, c.ID)
	}
	return sanaei.WithConfigEvidenceTx(ctx, s.DB, panel, func(tx *sql.Tx) error {
		var changed bool
		if e := tx.QueryRowContext(ctx, `SELECT applied_revision<>$3 OR applied_spec<>$4 OR remote_id IS DISTINCT FROM $5 OR credential_ref<>$6
  FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2`, p.ID, panel, p.Revision, spec, remote, ref).Scan(&changed); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `UPDATE config_driver_listeners SET remote_id=$3,applied_spec=$4,applied_revision=$5,credential_ref=$6,
  mutation_state='CONFIRMED',mutation_operation='UPSERT',native_observed_at=now(),retired_at=NULL,
  state=CASE WHEN $7 THEN 'APPLYING' ELSE state END,stage=CASE WHEN $7 THEN 'native_confirmed' ELSE stage END,
  verified_at=CASE WHEN $7 THEN NULL ELSE verified_at END,certificate_not_after=$8
  WHERE profile_id=$1 AND panel_id=$2`, p.ID, panel, remote, spec, p.Revision, ref, changed, cert); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO config_driver_client_retirements(profile_id,panel_id,client_id,email)
  SELECT profile_id,panel_id,client_id,email FROM config_driver_clients WHERE profile_id=$1 AND panel_id=$2 AND NOT(client_id=ANY($3::text[])) ON CONFLICT DO NOTHING`, p.ID, panel, pq.Array(ids)); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "DELETE FROM config_driver_clients WHERE profile_id=$1 AND panel_id=$2 AND NOT(client_id=ANY($3::text[]))", p.ID, panel, pq.Array(ids)); e != nil {
			return e
		}
		for _, cl := range credentials.Clients {
			var expires any
			if cl.ExpiresAt > 0 {
				expires = time.UnixMilli(cl.ExpiresAt)
			}
			if _, e := tx.ExecContext(ctx, "UPDATE config_driver_clients SET expires_at=$4,publish=$5,confirmed=true WHERE profile_id=$1 AND panel_id=$2 AND client_id=$3", p.ID, panel, cl.ID, expires, !cl.PolicySet || cl.Publish); e != nil {
				return e
			}
		}
		if changed {
			if _, e := tx.ExecContext(ctx, "DELETE FROM output_config_snapshots WHERE panel_id=$1 AND config_profile_id=$2", panel, p.ID); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, "UPDATE config_driver_class_proofs SET verified_at=NULL WHERE profile_id=$1 AND panel_id=$2", p.ID, panel); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=$1", panel); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, "INSERT INTO config_driver_events(profile_id,panel_id,revision,stage) VALUES($1,$2,$3,'native_confirmed')", p.ID, panel, p.Revision); e != nil {
				return e
			}
		}
		return nil
	})
}

// Deletion, pause and scope removal use exactly the same owned-listener path.
// The desired row remains as a tombstone, so a delayed writer cannot resurrect it.
func (s Service) retireListener(ctx context.Context, conn *sql.Conn, p Profile, panel string) (ret error) {
	var remote sql.NullInt64
	var raw []byte
	var ref, remark, state, mutation, operation string
	e := s.DB.QueryRowContext(ctx, `SELECT remote_id,applied_spec,credential_ref,managed_key,state,mutation_state,mutation_operation
 FROM config_driver_listeners WHERE profile_id=$1 AND panel_id=$2`, p.ID, panel).Scan(&remote, &raw, &ref, &remark, &state, &mutation, &operation)
	if errors.Is(e, sql.ErrNoRows) || state == "RETIRED" {
		return nil
	}
	if e != nil {
		return e
	}
	defer func() {
		if ret != nil {
			c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			code := "profile_delete_unverified"
			if ret.Error() == "profile_delete_identity_changed" {
				code = ret.Error()
			}
			_, _ = s.DB.ExecContext(c, "UPDATE config_driver_listeners SET state='BLOCKED',stage='deleting',error_code=$3,failures=failures+1,next_attempt_at=now()+interval '30 seconds' WHERE profile_id=$1 AND panel_id=$2", p.ID, panel, code)
			Event(c, s.DB, p, panel, "deleting", code)
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
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	// Use the same writer order as creation. Retirement never grabs siblings.
	var unified bool
	if e = s.DB.QueryRowContext(ctx, "SELECT unified FROM config_control_authority WHERE singleton").Scan(&unified); e != nil {
		return e
	}
	if !unified {
		if _, e = conn.ExecContext(ctx, "SELECT pg_advisory_lock(628341902731)"); e != nil {
			return e
		}
	}
	return s.withPanelLock(ctx, panel, func(ctx context.Context) error {
		current, e := Read(ctx, s.DB, p.ID)
		if e != nil {
			return e
		}
		if operation != "DELETE" && current.Enabled && current.DeleteRequestedAt == nil && (current.Fleet || containsPanel(current.Panels, panel)) {
			return nil
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
			var applied configdriver.Spec
			if json.Unmarshal(raw, &applied) != nil {
				return errors.New("profile_saved_spec_invalid")
			}
			var credentials configdriver.Credentials
			secret, e := s.Secrets.Get(ctx, t.AccountID, ref)
			if e == nil {
				e = DecodeCredentials(secret, p, &credentials)
			}
			// A row with no native identity and no sent request can be retired only
			// after observing that its ownership marker is absent.
			found := int64(0)
			for _, b := range raws {
				var x struct {
					ID     int64
					Remark string
				}
				if json.Unmarshal(b, &x) != nil {
					return errors.New("profile_inventory_invalid")
				}
				if x.Remark == remark {
					if found != 0 {
						return errors.New("profile_delete_identity_changed")
					}
					found = x.ID
				}
				if remote.Valid && x.ID == remote.Int64 && x.Remark != remark {
					return errors.New("profile_delete_identity_changed")
				}
			}
			if found != 0 {
				if e != nil {
					return errors.New("profile_credentials_missing")
				}
				payload, err := configdriver.Builtins().Build(applied, t.Host, remark, credentials)
				if err != nil {
					return err
				}
				got, err := configdriver.Observe(raws, payload, remote.Int64)
				if err != nil || got != found {
					return errors.New("profile_delete_identity_changed")
				}
				// A fresh matching identity permits an idempotent DELETE of that exact
				// inbound after an uncertain previous DELETE; never recreate on recovery.
				frozen, _ := json.Marshal(p)
				if _, err = s.DB.ExecContext(ctx, `UPDATE config_driver_listeners SET state='APPLYING',stage='deleting',verified_at=NULL,
    mutation_state='SENT',mutation_operation='DELETE',mutation_profile=$3,mutation_spec=applied_spec,
    mutation_revision=$4,mutation_credential_ref=credential_ref WHERE profile_id=$1 AND panel_id=$2`, p.ID, panel, frozen, p.Revision); err != nil {
					return err
				}
				before := configdriver.ProtectedHash(raws, remark)
				_, _ = rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/inbounds/del/" + strconv.FormatInt(found, 10), TimeoutSeconds: 20})
				rt.Session.Invalidate()
				raws, err = rt.Session.FreshSnapshot(ctx)
				if err != nil {
					return errors.New("profile_delete_unverified")
				}
				for _, b := range raws {
					var x struct {
						ID     int64
						Remark string
					}
					if json.Unmarshal(b, &x) != nil || x.ID == found || x.Remark == remark {
						return errors.New("profile_delete_unverified")
					}
				}
				if before != configdriver.ProtectedHash(raws, remark) {
					return errors.New("profile_sibling_changed")
				}
			}
			if e = s.retireUnusedClients(ctx, rt, p.ID, panel, raws); e != nil {
				return e
			}
			// Native global user records may outlive their inbound. Delete only the
			// exact owned identity, and first reject any attachment to a sibling.
			if len(credentials.Clients) > 0 {
				owned := map[string]string{}
				for _, c := range credentials.Clients {
					owned[c.Email] = c.ID
				}
				for _, b := range raws {
					var x struct{ Settings json.RawMessage }
					if json.Unmarshal(b, &x) != nil {
						return errors.New("profile_inventory_invalid")
					}
					var text string
					if json.Unmarshal(x.Settings, &text) == nil {
						x.Settings = []byte(text)
					}
					var settings struct{ Clients []struct{ Email string } }
					if json.Unmarshal(x.Settings, &settings) != nil {
						return errors.New("profile_inventory_invalid")
					}
					for _, c := range settings.Clients {
						if owned[c.Email] != "" {
							return errors.New("profile_delete_identity_changed")
						}
					}
				}
				globals, err := sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
				if err != nil {
					return err
				}
				emails := []string{}
				for _, c := range globals {
					if id := owned[c.Email]; id != "" {
						if id != c.UUID {
							return errors.New("profile_delete_identity_changed")
						}
						emails = append(emails, c.Email)
					}
				}
				if len(emails) > 0 {
					for len(emails) > 0 {
						n := len(emails)
						if n > 100 {
							n = 100
						}
						_, _ = sanaei.BulkDeleteClientsSession(ctx, rt.Session.Exec, emails[:n])
						emails = emails[n:]
					}
					globals, err = sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
					if err != nil {
						return err
					}
					for _, c := range globals {
						if owned[c.Email] != "" {
							return errors.New("profile_delete_unverified")
						}
					}
				}
			}
			verifier := s.VerifyAbsent
			if verifier == nil {
				verifier = s.verifyRuntimeAbsent
			}
			if e = verifier(ctx, t.Target, key, applied.Port); e != nil {
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
				if _, e := tx.ExecContext(ctx, "DELETE FROM config_driver_clients WHERE profile_id=$1 AND panel_id=$2", p.ID, panel); e != nil {
					return e
				}
				if _, e := tx.ExecContext(ctx, "DELETE FROM output_config_snapshots WHERE panel_id=$1 AND config_profile_id=$2", panel, p.ID); e != nil {
					return e
				}
				if _, e := tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=$1", panel); e != nil {
					return e
				}
				_, e := tx.ExecContext(ctx, `UPDATE config_driver_listeners SET state='RETIRED',stage='deleted',error_code='',failures=0,
    mutation_state='CONFIRMED',verified_at=NULL,retired_at=now(),next_attempt_at=now(),remote_id=NULL,applied_revision=0 WHERE profile_id=$1 AND panel_id=$2`, p.ID, panel)
				return e
			})
		})
	})
}

func (s Service) verifyRuntimeAbsent(ctx context.Context, t provisioning.Target, key []byte, port int) error {
	if configdriver.ValidatePort(port) != nil {
		return errors.New("profile_port_invalid")
	}
	command := fmt.Sprintf(`python3 - <<'PY'
import pathlib,os
port=%d
for p in pathlib.Path('/proc').iterdir():
 if not p.name.isdigit():continue
 try:
  if not os.readlink(p/'exe').startswith('/usr/local/x-ui/bin/xray-'):continue
  owned=set()
  for fd in (p/'fd').iterdir():
   try:
    s=os.readlink(fd)
    if s.startswith('socket:['):owned.add(s[8:-1])
   except OSError:pass
  for family in ('tcp','tcp6'):
   for line in (p/'net'/family).read_text().splitlines()[1:]:
    c=line.split()
    if c[3]=='0A' and int(c[1].split(':')[1],16)==port and c[9] in owned:raise RuntimeError('listener still running')
 except FileNotFoundError:pass
print('absent')
PY`, port)
	if _, e := s.SSH.Run(ctx, t, key, command); e != nil {
		return errors.New("profile_delete_runtime_unverified")
	}
	return nil
}

func (s Service) finishRetired(ctx context.Context) error {
	// Provider-confirmed deletion is also conclusive absence. Disabled accounts
	// or failed polls alone never release ownership or claim successful deletion.
	if _, e := s.DB.ExecContext(ctx, `UPDATE config_driver_listeners l SET state='RETIRED',stage='server_removed',error_code='',retired_at=now(),verified_at=NULL,remote_id=NULL,applied_revision=0,mutation_state='CONFIRMED'
 FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id WHERE l.panel_id=pi.id AND l.state<>'RETIRED' AND dr.state='DELETED'
 AND NOT EXISTS(SELECT 1 FROM resources r WHERE r.account_id=dr.account_id AND r.provider_resource_id=dr.provider_resource_id AND r.managed AND r.state<>'deleted')`); e != nil {
		return e
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE config_driver_profiles p SET deleted_at=now() WHERE p.delete_requested_at IS NOT NULL AND p.deleted_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM config_driver_listeners l WHERE l.profile_id=p.id AND l.state<>'RETIRED')
 AND NOT EXISTS(SELECT 1 FROM config_control_authority c JOIN panel_cleanup_jobs j ON j.id=c.reset_job_id WHERE j.state NOT IN('SUCCEEDED','CANCELLED') AND p.delete_requested_at<=c.activated_at)`)
	return e
}
