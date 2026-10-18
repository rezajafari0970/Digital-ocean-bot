package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
	"time"
)

// Publication rechecks desired revision, runtime proof and a fresh native
// snapshot independently for every profile. A failed sibling is withheld alone.
func (s *Server) collectProfileOutput(ctx context.Context, panel, host string, raws []json.RawMessage) []outputRecord {
	rows, e := s.DB.QueryContext(ctx, `SELECT l.profile_id::text,l.credential_ref,l.remote_id,l.managed_key,pi.account_id::text,l.certificate_not_after
 FROM config_driver_listeners l JOIN config_driver_profiles cp ON cp.id=l.profile_id JOIN panel_instances pi ON pi.id=l.panel_id
 WHERE l.panel_id=$1 AND cp.enabled AND (cp.fleet OR l.panel_id=ANY(cp.panel_ids))
 AND l.state='VERIFIED' AND l.applied_revision=cp.revision AND l.applied_spec=cp.spec AND l.verified_at>now()-interval '5 minutes'
 AND (cp.spec->>'security'<>'tls' OR l.certificate_not_after>now()+interval '5 minutes')`, panel)
	if e != nil {
		return nil
	}
	type candidate struct {
		id, ref, remark, account string
		remote                   int64
		until                    sql.NullTime
	}
	list := []candidate{}
	for rows.Next() {
		var c candidate
		if rows.Scan(&c.id, &c.ref, &c.remote, &c.remark, &c.account, &c.until) != nil {
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
		id, e := configdriver.Observe(raws, desired, v.remote)
		if e != nil || id != v.remote {
			continue
		}
		candidateRecords := []outputRecord{}
		valid := true
		for _, cl := range credentials.Clients {
			var owned bool
			e = s.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM config_driver_clients WHERE profile_id=$1 AND panel_id=$2 AND client_id=$3 AND email=$4 AND route_class=$5)", p.ID, panel, cl.ID, cl.Email, cl.Class).Scan(&owned)
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
			candidateRecords = append(candidateRecords, outputRecord{ClientID: cl.ID, URI: uri, VisibleUntil: until, ProfileID: p.ID, ProfileRevision: p.Revision})
		}
		if valid {
			out = append(out, candidateRecords...)
		}
	}
	return out
}

const profileOutputVerified = ` EXISTS(SELECT 1 FROM config_driver_clients dc JOIN config_driver_listeners dl ON dl.profile_id=dc.profile_id AND dl.panel_id=dc.panel_id JOIN config_driver_profiles dp ON dp.id=dc.profile_id
 WHERE dc.panel_id=p.id AND dc.client_id=o.client_id AND o.config_profile_id=dp.id AND o.config_profile_revision=dp.revision AND dp.enabled AND (dp.fleet OR p.id=ANY(dp.panel_ids))
 AND dl.state='VERIFIED' AND dl.applied_revision=dp.revision AND dl.applied_spec=dp.spec AND dl.verified_at>now()-interval '5 minutes'
 AND (dp.spec->>'security'<>'tls' OR dl.certificate_not_after>now()+interval '5 minutes')) `
const profileOutputOwned = ` EXISTS(SELECT 1 FROM config_driver_clients dc WHERE dc.panel_id=p.id AND dc.client_id=o.client_id) `
