package residentialsync

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	secretstore "github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"net"
	"sort"
	"time"
)

type relayWriter interface {
	Put(context.Context, string, string, string, []byte) error
}

func (s Service) relaySources(ctx context.Context) ([]string, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT p.id::text,d.host FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 JOIN LATERAL(SELECT host,profile_snapshot FROM deployments WHERE droplet_id=dr.id AND state='PANEL_COMPLETE' ORDER BY created_at DESC,id DESC LIMIT 1)d ON true
 WHERE p.enabled AND dr.state='READY' AND a.enabled AND a.deletion_requested_at IS NULL AND a.deleted_at IS NULL
 AND COALESCE((d.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
 AND(dr.expires_at IS NULL OR dr.expires_at>now()+interval '60 seconds') ORDER BY p.id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	sources := []string{}
	seen := map[string]bool{}
	for rows.Next() {
		var id, host string
		if e = rows.Scan(&id, &host); e != nil {
			return nil, e
		}
		ip := net.ParseIP(host)
		if !RelayEnabled(id) || ip == nil || ip.To4() == nil || ip.IsPrivate() || !ip.IsGlobalUnicast() {
			continue
		}
		source := ip.String() + "/32"
		if !seen[source] {
			sources = append(sources, source)
			seen[source] = true
		}
	}
	sort.Strings(sources)
	return sources, rows.Err()
}

// Runs only under the existing panel configuration lock. Provisioning is a
// template+route transaction; no unauthenticated or temporarily direct inlet.
func (s Service) prepareRelay(ctx context.Context, panel string, rt *sanaei.PanelRuntime) error {
	if !relayConfigured() {
		_, e := s.DB.ExecContext(ctx, "UPDATE panel_relay_endpoints SET enabled=false,state='DISABLED' WHERE panel_id=$1 AND enabled", panel)
		return e
	}
	sources, e := s.relaySources(ctx)
	if e != nil {
		return e
	}
	var account, host string
	var serverExpiry sql.NullTime
	e = s.DB.QueryRowContext(ctx, `SELECT p.account_id::text,dep.host,dr.expires_at FROM panel_instances p
 JOIN droplets dr ON dr.id=p.droplet_id JOIN accounts a ON a.id=p.account_id
 JOIN LATERAL(SELECT host,profile_snapshot FROM deployments WHERE droplet_id=dr.id AND state='PANEL_COMPLETE' ORDER BY created_at DESC,id DESC LIMIT 1)dep ON true
 WHERE p.id=$1 AND p.enabled AND dr.state='READY' AND a.enabled AND a.provider_state='ACTIVE'
 AND a.deletion_requested_at IS NULL AND a.deleted_at IS NULL AND a.provider IN('vultr','digitalocean','linode')
 AND NOT a.upcloud_trial_compatible AND NOT COALESCE((dep.profile_snapshot->>'upcloud_trial_compatible')::boolean,false)
 AND(dr.expires_at IS NULL OR dr.expires_at>now()+CASE WHEN EXISTS(SELECT 1 FROM panel_relay_endpoints ep WHERE ep.panel_id=p.id AND ep.enabled) THEN interval '5 minutes' ELSE interval '10 minutes' END)`, panel).Scan(&account, &host, &serverExpiry)
	if errors.Is(e, sql.ErrNoRows) || len(sources) == 0 {
		_, e = s.DB.ExecContext(ctx, "UPDATE panel_relay_endpoints SET enabled=false,state='DISABLED' WHERE panel_id=$1 AND enabled", panel)
		return e
	}
	if e != nil {
		return e
	}
	// A donor must itself enforce the requested protected routing contract.
	donorPolicy := (routePolicy{StrictAllowlist: strictAllowlistPanel(panel), AdsOnly: adsOnlyPanel(panel), Harden: hardeningPanel(panel)}).normalized()
	if !donorPolicy.StrictAllowlist || !donorPolicy.AdsOnly || !donorPolicy.Harden || !ClientPathsEnabled(panel) || !poolPanel(panel) {
		return nil
	}
	writer, ok := s.Secrets.(relayWriter)
	if !ok {
		return errors.New("managed relay secret writer unavailable")
	}
	var ref string
	var existingPort int
	e = s.DB.QueryRowContext(ctx, "SELECT secret_ref,port FROM panel_relay_endpoints WHERE panel_id=$1", panel).Scan(&ref, &existingPort)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var credential relayCredential
	if ref != "" {
		raw, x := s.Secrets.Get(ctx, account, ref)
		if x != nil && !errors.Is(x, secretstore.ErrSecretNotFound) {
			return errors.New("relay secret unavailable")
		}
		if x == nil && json.Unmarshal(raw, &credential) != nil {
			return errors.New("relay secret invalid")
		}
	}
	if !relayCredentialValid(credential) {
		credential, e = newRelayCredential()
		if e != nil {
			return e
		}
		id := make([]byte, 16)
		if _, e = rand.Read(id); e != nil {
			return e
		}
		ref = "trial-relay-" + hex.EncodeToString(id)
		raw, _ := json.Marshal(credential)
		if e = writer.Put(ctx, account, ref, "trial_relay", raw); e != nil {
			return e
		}
	}
	rt.Session.Invalidate()
	raws, e := rt.Session.Snapshot(ctx)
	if e != nil {
		return e
	}
	current, _, e := readXraySetting(ctx, rt.Session.Exec)
	if e != nil {
		return e
	}
	port, e := chooseRelayPort(raws, current, relayInletPrefix+panel)
	if e != nil {
		return e
	}
	validUntil := relayCredentialExpiry(credential)
	if serverExpiry.Valid && serverExpiry.Time.Before(validUntil) {
		validUntil = serverExpiry.Time
	}
	if !validUntil.After(time.Now().Add(5 * time.Minute)) {
		return errors.New("relay lifetime too short")
	}
	proxy, e := managedSOCKS(panel, host, ref, port, credential)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO panel_relay_endpoints(panel_id,account_id,host,port,secret_ref,transport_hash,allowed_sources,valid_until,enabled,state)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,'PREPARING') ON CONFLICT(panel_id) DO UPDATE SET
 host=excluded.host,port=excluded.port,secret_ref=excluded.secret_ref,transport_hash=excluded.transport_hash,allowed_sources=excluded.allowed_sources,valid_until=excluded.valid_until,enabled=true,
 state=CASE WHEN panel_relay_endpoints.transport_hash=excluded.transport_hash AND panel_relay_endpoints.allowed_sources=excluded.allowed_sources AND panel_relay_endpoints.enabled THEN panel_relay_endpoints.state ELSE 'PREPARING' END,
 updated_at=CASE WHEN panel_relay_endpoints.transport_hash=excluded.transport_hash AND panel_relay_endpoints.allowed_sources=excluded.allowed_sources AND panel_relay_endpoints.enabled THEN panel_relay_endpoints.updated_at ELSE now() END`,
		panel, account, host, port, ref, proxy.TransportHash, pq.Array(sources), validUntil)
	return e
}
func (s Service) loadRelayInlet(ctx context.Context, panel string) (*relayInlet, error) {
	if !relayConfigured() {
		return nil, nil
	}
	var in relayInlet
	var account string
	e := s.DB.QueryRowContext(ctx, "SELECT panel_id::text,account_id::text,host,port,secret_ref,allowed_sources FROM panel_relay_endpoints WHERE panel_id=$1 AND enabled", panel).Scan(&in.Panel, &account, &in.Host, &in.Port, &in.SecretRef, pq.Array(&in.Sources))
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	raw, e := s.Secrets.Get(ctx, account, in.SecretRef)
	if e != nil {
		return nil, errors.New("relay secret unavailable")
	}
	if json.Unmarshal(raw, &in.Credential) != nil {
		return nil, errors.New("relay secret invalid")
	}
	return &in, nil
}
