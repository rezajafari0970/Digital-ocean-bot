package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"time"
)

// Publication rechecks desired revision, runtime proof and a fresh native
// snapshot independently for every profile. A failed sibling is withheld alone.
func (s *Server) collectProfileOutput(ctx context.Context, panel, host string, raws []json.RawMessage) []outputRecord {
	rows, e := s.DB.QueryContext(ctx, `SELECT l.profile_id::text,l.credential_ref,l.remote_id,l.managed_key,pi.account_id::text,l.certificate_not_after,l.applied_spec
 FROM config_driver_listeners l JOIN config_driver_profiles cp ON cp.id=l.profile_id JOIN panel_instances pi ON pi.id=l.panel_id
 WHERE l.panel_id=$1 AND cp.enabled AND cp.delete_requested_at IS NULL AND (cp.fleet OR l.panel_id=ANY(cp.panel_ids))
 AND l.state='VERIFIED' AND l.applied_revision=cp.revision AND (l.applied_spec - 'sni')=(cp.spec - 'sni') AND (COALESCE(cp.spec->>'sni_mode','')='auto' OR l.applied_spec->>'sni' IS NOT DISTINCT FROM cp.spec->>'sni') AND l.verified_at>now()-interval '5 minutes'
 AND (cp.spec->>'security'<>'tls' OR l.certificate_not_after>now()+interval '5 minutes')`, panel)
	if e != nil {
		return nil
	}
	type candidate struct {
		id, ref, remark, account string
		remote                   int64
		until                    sql.NullTime
		spec                     []byte
	}
	list := []candidate{}
	for rows.Next() {
		var c candidate
		if rows.Scan(&c.id, &c.ref, &c.remote, &c.remark, &c.account, &c.until, &c.spec) != nil {
			rows.Close()
			return nil
		}
		list = append(list, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil
	}
	out := []outputRecord{}
	registry := configdriver.Builtins()
	for _, v := range list {
		p, e := configplane.Read(ctx, s.DB, v.id)
		if e != nil {
			continue
		}
		if json.Unmarshal(v.spec, &p.Spec) != nil {
			continue
		}
		b, e := s.Container.Secrets.Get(ctx, v.account, v.ref)
		if e != nil {
			continue
		}
		var credentials configdriver.Credentials
		if configplane.DecodeCredentials(b, p, &credentials) != nil {
			continue
		}
		desired, e := registry.Build(p.Spec, host, v.remark, credentials)
		if e != nil {
			continue
		}
		id, e := configdriver.Observe(raws, desired, v.remote, credentials)
		if e != nil || id != v.remote {
			continue
		}
		endpoints := []cloudflare.Endpoint{{Address: host}}
		if p.Spec.CloudflareAccount != "" {
			endpoints, e = s.cloudflareService().Endpoints(ctx, p.Spec.CloudflareAccount, p.ID, panel, p.Revision)
			if e != nil {
				continue
			}
		}
		candidateRecords := []outputRecord{}
		valid := true
		for _, cl := range credentials.Clients {
			if !configdriver.Active(cl, time.Now()) || cl.PolicySet && !cl.Publish {
				continue
			}
			var owned bool
			e = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_clients WHERE profile_id=$1 AND panel_id=$2 AND client_id=$3 AND email=$4 AND route_class=$5 AND confirmed)", p.ID, panel, cl.ID, cl.Email, cl.Class).Scan(&owned)
			if e != nil || !owned {
				valid = false
				break
			}
			uri, e := registry.URI(p.Spec, host, p.Name, credentials, cl)
			if e != nil {
				valid = false
				break
			}
			var until *time.Time
			if p.Spec.Security == "tls" && v.until.Valid {
				u := v.until.Time.Add(-5 * time.Minute)
				until = &u
			}
			if cl.ExpiresAt > 0 {
				expiry := time.UnixMilli(cl.ExpiresAt)
				if until == nil || expiry.Before(*until) {
					until = &expiry
				}
			}
			for _, endpoint := range endpoints {
				endpointURI := uri
				endpointUntil := until
				if endpoint.ID != "" {
					allowed := false
					for _, class := range endpoint.Classes {
						allowed = allowed || class == cl.Class
					}
					if !allowed {
						continue
					}
					endpointURI, e = registry.CDNURI(p.Spec, host, endpoint.Address, endpoint.Hostname, p.Name, credentials, cl)
					if e != nil {
						continue
					}
					expiry := endpoint.ExpiresAt.Add(-5 * time.Minute)
					if endpointUntil == nil || expiry.Before(*endpointUntil) {
						endpointUntil = &expiry
					}
				}
				candidateRecords = append(candidateRecords, outputRecord{CloudflareRecordID: endpoint.ID, ClientID: cl.ID, URI: endpointURI, VisibleUntil: endpointUntil, ProfileID: p.ID, ProfileRevision: p.Revision})
			}
		}
		if valid {
			out = append(out, candidateRecords...)
		}
	}
	return out
}

const profileOutputVerified = ` EXISTS(SELECT 1 FROM config_driver_clients dc JOIN config_driver_listeners dl ON dl.profile_id=dc.profile_id AND dl.panel_id=dc.panel_id JOIN config_driver_profiles dp ON dp.id=dc.profile_id
 WHERE dc.panel_id=p.id AND dc.client_id=o.client_id AND o.config_profile_id=dp.id AND o.config_profile_revision=dp.revision AND dp.enabled AND dp.delete_requested_at IS NULL AND (dp.fleet OR p.id=ANY(dp.panel_ids))
 AND EXISTS(SELECT 1 FROM config_driver_class_proofs proof WHERE proof.profile_id=dp.id AND proof.panel_id=p.id AND proof.route_class=dc.route_class AND proof.verified_at>now()-interval '5 minutes' AND proof.error_code='')
 AND (COALESCE(dp.spec->>'cloudflare_account','')='' AND o.cloudflare_record_id IS NULL OR EXISTS(
 SELECT 1 FROM cloudflare_records cf JOIN cloudflare_accounts ca ON ca.id=cf.account_id JOIN cloudflare_zones cz ON cz.account_id=cf.account_id AND cz.id=cf.zone_id
 WHERE cf.id=o.cloudflare_record_id AND cf.profile_id=dp.id AND cf.panel_id=p.id AND cf.account_id::text=dp.spec->>'cloudflare_account'
 AND cf.operator=COALESCE(dp.spec->>'cloudflare_operator','') AND cf.state='DNS_READY' AND cf.expires_at>now()+interval '5 minutes' AND cf.observed_at>now()-interval '10 minutes' AND cf.verified_at>now()-interval '5 minutes' AND cf.verified_revision=dp.revision AND dc.route_class=ANY(cf.verified_classes) AND ca.enabled AND cz.ssl_strict AND cz.checked_at>now()-interval '10 minutes' AND cz.enabled AND cz.present AND cz.status='active'))
 AND dc.confirmed AND dc.publish AND (dc.expires_at IS NULL OR dc.expires_at>now())
 AND COALESCE((dp.class_settings->dc.route_class->>'publish')::boolean,true)
 AND dc.email=cr.email AND dc.route_class=cr.route_class AND dc.route_class=cr.effective_class
 AND rc.enabled AND (rc.fleet OR p.id=ANY(rc.panel_ids)) AND rs.state='APPLIED' AND rs.revision=rc.revision AND cr.revision=rc.revision AND rs.verified_at>now()-interval '60 seconds'
 AND dl.state='VERIFIED' AND dl.applied_revision=dp.revision AND (dl.applied_spec - 'sni')=(dp.spec - 'sni') AND (COALESCE(dp.spec->>'sni_mode','')='auto' OR dl.applied_spec->>'sni' IS NOT DISTINCT FROM dp.spec->>'sni') AND dl.verified_at>now()-interval '5 minutes'
 AND (dp.spec->>'security'<>'tls' OR dl.certificate_not_after>now()+interval '5 minutes')) `
const profileOutputOwned = ` EXISTS(SELECT 1 FROM config_driver_clients dc WHERE dc.panel_id=p.id AND dc.client_id=o.client_id) `
