package residentialsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
)

type relayChoice struct {
	Proxy    rp
	Provider string
	Retained bool
}

// Install every eligible distinct path. A transient health failure is handled by
// Xray's native healthy-only balancer, not a pool rewrite/restart that disrupts
// all healthy peers. Lifetime, ownership and native policy proofs remain gates.
// Keep assignments stable. Distinct panels alone are insufficient:
// both underlying server IDs and public IPs must be different.
func selectRelays(choices []relayChoice, limits ...int) []rp {
	limit := len(choices)
	if len(limits) > 0 {
		limit = min(limit, limits[0])
	}
	out := []rp{}
	hosts := map[string]bool{}
	servers := map[string]bool{}
	providers := map[string]int{}
	accounts := map[string]int{}
	for len(out) < limit {
		best := -1
		score := int(^uint(0) >> 1)
		for i, c := range choices {
			if hosts[c.Proxy.Host] || servers[c.Proxy.DonorDroplet] {
				continue
			}
			rank := providers[c.Provider]*100 + accounts[c.Proxy.DonorAccount]*10
			if c.Retained {
				rank -= 10000
			}
			if rank < score {
				best = i
				score = rank
			}
		}
		if best < 0 {
			break
		}
		c := choices[best]
		out = append(out, c.Proxy)
		hosts[c.Proxy.Host] = true
		servers[c.Proxy.DonorDroplet] = true
		providers[c.Provider]++
		accounts[c.Proxy.DonorAccount]++
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s Service) relayProxies(ctx context.Context, receiver string, p routePolicy) ([]rp, error) {
	rows, err := s.DB.QueryContext(ctx, `
 SELECT p.id::text,p.droplet_id::text,p.account_id::text,a.provider,dep.host,rs.plan_hash,
 ep.secret_ref,ep.transport_hash,ep.port,COALESCE(previous.selected AND previous.transport_hash=ep.transport_hash,false)
 FROM panel_instances p
 JOIN droplets dr ON dr.id=p.droplet_id
 JOIN accounts a ON a.id=p.account_id
 JOIN LATERAL(SELECT d.host,d.profile_snapshot FROM deployments d WHERE d.droplet_id=dr.id AND d.account_id=a.id AND d.state='PANEL_COMPLETE' ORDER BY d.created_at DESC,d.id DESC LIMIT 1)dep ON true
 JOIN panel_routing_state rs ON rs.panel_id=p.id
 CROSS JOIN residential_routing_control rc
 LEFT JOIN panel_relay_assignments previous ON previous.receiver_panel_id=$1 AND previous.donor_panel_id=p.id
 JOIN panel_relay_endpoints ep ON ep.panel_id=p.id AND ep.enabled AND ep.state='APPLIED'
 AND ep.verified_at>now()-CASE WHEN COALESCE(previous.selected AND previous.donor_plan_hash=rs.plan_hash AND previous.transport_hash=ep.transport_hash AND previous.secret_ref=ep.secret_ref,false) THEN interval '3 minutes' ELSE interval '60 seconds' END AND ep.plan_hash=rs.plan_hash
 AND ep.host=dep.host
 AND (SELECT d.host||'/32' FROM panel_instances receiver JOIN LATERAL(
 SELECT host FROM deployments WHERE droplet_id=receiver.droplet_id AND state='PANEL_COMPLETE' ORDER BY created_at DESC,id DESC LIMIT 1)d ON true WHERE receiver.id=$1)=ANY(ep.allowed_sources)
 WHERE p.id<>$1 AND p.enabled AND dr.state='READY' AND a.enabled AND a.provider_state='ACTIVE'
  AND a.deletion_requested_at IS NULL AND a.deleted_at IS NULL
  AND NOT a.upcloud_trial_compatible AND NOT COALESCE((dep.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
  AND a.provider IN('vultr','digitalocean','linode')
  AND (dr.expires_at IS NULL OR dr.expires_at>now()+CASE WHEN COALESCE(previous.selected,false) THEN interval '5 minutes' ELSE interval '10 minutes' END)
  AND ep.valid_until>now()+CASE WHEN COALESCE(previous.selected,false) THEN interval '5 minutes' ELSE interval '10 minutes' END
  AND rc.enabled AND (rc.fleet OR p.id=ANY(rc.panel_ids))
  AND (rs.state='APPLIED' OR (rs.state='FAILED' AND COALESCE(previous.selected AND previous.donor_plan_hash=rs.plan_hash AND previous.transport_hash=ep.transport_hash AND previous.secret_ref=ep.secret_ref,false))) AND rs.revision=rc.revision AND rs.verified_at>now()-CASE WHEN COALESCE(previous.selected AND previous.donor_plan_hash=rs.plan_hash AND previous.transport_hash=ep.transport_hash AND previous.secret_ref=ep.secret_ref,false) THEN interval '3 minutes' ELSE interval '60 seconds' END
  AND NOT rs.relay_mode AND rs.category_digest=$2
  AND (COALESCE(previous.selected,false) OR previous.cooldown_until IS NULL OR previous.cooldown_until<=now())
  AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=p.id AND cj.state NOT IN('SUCCEEDED','CANCELLED'))
  AND NOT EXISTS(SELECT 1 FROM server_protection_nodes pn WHERE pn.panel_id=p.id AND
   (pn.verified_status->>'admission_blocked'='true' OR (pn.verified_status->>'enabled'='true' AND pn.verified_status->>'xui_state' IN('failed','inactive','active_xray_failed'))))
  AND EXISTS(SELECT 1 FROM residential_proxies rp WHERE (rs.pool_enabled OR rp.proxy_id=rs.selected_proxy_id)
   AND rp.enabled AND rp.type='socks5' AND rp.status='healthy' AND rp.last_success_at>now()-interval '3 minutes')
 ORDER BY hashtextextended(p.id::text||$1::text,4243),p.id`, receiver, categoryDigest(p))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	choices := []relayChoice{}
	for rows.Next() {
		var id, droplet, account, provider, host, plan, ref, transport string
		var port int
		var retained bool
		if err = rows.Scan(&id, &droplet, &account, &provider, &host, &plan, &ref, &transport, &port, &retained); err != nil {
			return nil, err
		}
		raw, e := s.Secrets.Get(ctx, account, ref)
		if e != nil {
			continue
		}
		var credential relayCredential
		if json.Unmarshal(raw, &credential) != nil || !relayCredentialValid(credential) {
			continue
		}
		proxy, e := managedSOCKS(id, host, ref, port, credential)
		if e == nil && proxy.TransportHash != transport {
			e = errors.New("relay credential revision mismatch")
		}
		if e != nil {
			continue
		}
		proxy.DonorDroplet = droplet
		proxy.DonorAccount = account
		proxy.DonorPlan = plan
		choices = append(choices, relayChoice{proxy, provider, retained})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if !RelayExpandedEnabled(receiver) {
		return selectRelays(choices, RelayMinimumHealthy), nil
	}
	return selectRelays(choices), nil
}

func persistRelays(ctx context.Context, tx *sql.Tx, panel, hash string, p routePolicy, desired map[string]any) error {
	if _, e := tx.ExecContext(ctx, "UPDATE panel_relay_assignments SET selected=false,applied=false WHERE receiver_panel_id=$1", panel); e != nil {
		return e
	}
	if !p.RelayMode {
		return nil
	}
	for _, proxy := range p.Proxies {
		tag := tagged(desired, proxy.Tag)
		_, e := tx.ExecContext(ctx, `INSERT INTO panel_relay_assignments(receiver_panel_id,donor_panel_id,donor_droplet_id,donor_account_id,secret_ref,transport_hash,donor_plan_hash,receiver_plan_hash,outbound_tag)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(receiver_panel_id,donor_panel_id) DO UPDATE SET
   donor_droplet_id=excluded.donor_droplet_id,donor_account_id=excluded.donor_account_id,secret_ref=excluded.secret_ref,
   transport_hash=excluded.transport_hash,donor_plan_hash=excluded.donor_plan_hash,receiver_plan_hash=excluded.receiver_plan_hash,
   outbound_tag=excluded.outbound_tag,selected=true,applied=false,
 alive=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.alive ELSE false END,
 failed_since=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.failed_since ELSE NULL END,
 failures=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.failures ELSE 0 END,
 last_try=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.last_try ELSE 0 END,
 last_success_at=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.last_success_at ELSE NULL END,
 observed_at=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.observed_at ELSE NULL END,
 cooldown_until=CASE WHEN panel_relay_assignments.outbound_tag=excluded.outbound_tag THEN panel_relay_assignments.cooldown_until ELSE NULL END,updated_at=now()`,
			panel, proxy.ID, proxy.DonorDroplet, proxy.DonorAccount, proxy.Relay.SecretRef, proxy.TransportHash, proxy.DonorPlan, hash, tag)
		if e != nil {
			return e
		}
	}
	return nil
}
