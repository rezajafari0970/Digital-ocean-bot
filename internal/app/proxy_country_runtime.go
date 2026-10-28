package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/geoctx"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxycontrol"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

// Country snapshots contain references and version stamps, never passwords.
// Probe results are provisional until the snapshot, fence and transport epoch
// are checked again in the same transaction that publishes the complete route.
type countryProxy struct {
	ID, Type, Host, Username, Adapter, SecretRef, SecretVersion, Status string
	Port, Priority                                                      int
}
type countryConfig struct {
	Account, Provider, Mode string
	Enabled, Cleanup        bool
	Revision                int64
	Countries               []string
	Pool                    []countryProxy
}
type countryCandidate struct {
	ProxyID, Session, Target string
	Port                     int
	Geo                      stickyGeo
	FirstSeen                time.Time
	ObservedAt               time.Time
}
type countrySnapshot struct {
	Config                                             countryConfig
	Active, Session, IP, Target, Stage, LearnedCountry string
	Port                                               int
	Health, LegacyFallback                             bool
	Epoch, Fence, Applied                              int64
	Checkpoint                                         proxyselection.Checkpoint
	Candidate                                          *countryCandidate
	Scope                                              string
	HealthVersion                                      string
}
type countryQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadCountrySnapshot(ctx context.Context, q countryQuery, account string) (countrySnapshot, error) {
	var s countrySnapshot
	var raw, checkpoint, candidate []byte
	err := q.QueryRowContext(ctx, `SELECT jsonb_build_object(
 'Account',a.id,'Provider',a.provider,'Mode',n.mode,'Enabled',a.enabled,'Cleanup',a.deletion_requested_at IS NOT NULL,
 'Revision',COALESCE(p.revision,0),'Countries',COALESCE(p.countries,'["AUTO"]'::jsonb),
 'Pool',COALESCE((SELECT jsonb_agg(jsonb_build_object('ID',px.id,'Type',px.type,'Host',px.host,'Port',px.port,
 'Username',COALESCE(px.username,''),'Adapter',px.adapter,'SecretRef',COALESCE(px.secret_ref,''),
 'SecretVersion',COALESCE((SELECT secret.xmin::text||':'||secret.updated_at::text FROM secrets secret WHERE secret.proxy_id=px.id AND secret.id=px.secret_ref),''),
 'Status',px.status,'Priority',ap.priority) ORDER BY ap.priority,px.id)
 FROM account_proxy_pool ap JOIN proxies px ON px.id=ap.proxy_id WHERE ap.account_id=a.id AND ap.enabled),'[]'::jsonb)),
 COALESCE(n.proxy_id::text,''),COALESCE(i.sticky_session,''),COALESCE(host(i.exit_ip),''),
 COALESCE(i.selected_target_country,''),COALESCE(i.sticky_port,0),COALESCE(i.last_health_ok,false) AND i.rotation_started_at IS NULL,
 COALESCE(t.transport_epoch,0),COALESCE(s.fence,0),COALESCE(s.applied_revision,0),COALESCE(s.active_stage,''),
 COALESCE(s.checkpoint,'{}'::jsonb),s.candidate,COALESCE(i.preferred_country_code,''),COALESCE(i.fallback_active,false),COALESCE((SELECT r.xmin::text FROM proxy_runtime_state r WHERE r.account_id=a.id AND r.proxy_id=n.proxy_id AND r.provider=a.provider),'')
 FROM accounts a JOIN network_profiles n ON n.account_id=a.id
 LEFT JOIN account_proxy_country_policies p ON p.account_id=a.id
 LEFT JOIN account_proxy_selection s ON s.account_id=a.id
 LEFT JOIN account_network_identities i ON i.account_id=a.id
 LEFT JOIN account_transport_state t ON t.account_id=a.id WHERE a.id=$1`, account).Scan(&raw, &s.Active, &s.Session, &s.IP, &s.Target, &s.Port, &s.Health, &s.Epoch, &s.Fence, &s.Applied, &s.Stage, &checkpoint, &candidate, &s.LearnedCountry, &s.LegacyFallback, &s.HealthVersion)
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(raw, &s.Config); err != nil {
		return s, err
	}
	normalized, err := proxyselection.Normalize(s.Config.Countries)
	if err != nil {
		return s, err
	}
	s.Config.Countries = normalized
	data, _ := json.Marshal(s.Config)
	hash := sha256.Sum256(data)
	s.Scope = hex.EncodeToString(hash[:])
	if err = json.Unmarshal(checkpoint, &s.Checkpoint); err != nil {
		return s, err
	}
	if len(candidate) > 0 && string(candidate) != "null" {
		if err = json.Unmarshal(candidate, &s.Candidate); err != nil {
			return s, err
		}
	}
	if s.Checkpoint.Scope != s.Scope {
		s.Checkpoint = proxyselection.Checkpoint{}
		s.Candidate = nil
	}
	return s, nil
}

func countryRouteTx(ctx context.Context, db *sql.DB, account string) (*sql.Tx, error) {
	var canonicalErr error
	account, canonicalErr = network.CanonicalAccountID(account)
	if canonicalErr != nil {
		return nil, canonicalErr
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, prefix := range []string{"account-proxy:", "account-route:"} {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, prefix+account); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	// Settings writers own the account row before route locks. NOWAIT avoids a
	// reverse-order wait; the next keeper tick safely retries from a new snapshot.
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id::text FROM accounts WHERE id=$1 FOR SHARE NOWAIT`, account).Scan(&id); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err = checkProxyKeeperLease(ctx, tx, account); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func (c Container) claimCountryWork(ctx context.Context, account string) (countrySnapshot, error) {
	var s countrySnapshot
	tx, err := countryRouteTx(ctx, c.DB, account)
	if err != nil {
		return s, err
	}
	defer tx.Rollback()
	s, err = loadCountrySnapshot(ctx, tx, account)
	if err != nil {
		return s, err
	}
	if !s.Config.Enabled && !s.Config.Cleanup {
		return s, ErrAccountDisabled
	}
	if s.Config.Mode != "proxy_required" || proxyselection.Legacy(s.Config.Countries) {
		return s, tx.Commit()
	}
	s.Fence, err = FenceCountryWriterTx(ctx, tx, account)
	if err != nil {
		return s, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_proxy_selection SET work_revision=$2,scope_hash=$3 WHERE account_id=$1`, account, s.Config.Revision, s.Scope)
	if err != nil {
		return s, err
	}
	return s, tx.Commit()
}

func (s countrySnapshot) proxy(id string) (countryProxy, bool) {
	for _, p := range s.Config.Pool {
		if p.ID == id {
			return p, true
		}
	}
	return countryProxy{}, false
}
func countrySession() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (c Container) countryCandidatePort(ctx context.Context, s countrySnapshot) (int, error) {
	if s.Port >= 10000 && s.Port <= 20000 {
		return s.Port, nil
	}
	// Provisional allocation never erases the incumbent. Publication rechecks
	// exclusivity under the same global lock used by the legacy allocator.
	var port int
	err := c.DB.QueryRowContext(ctx, `SELECT p FROM generate_series(10000,20000) p WHERE NOT EXISTS
 (SELECT 1 FROM account_network_identities i WHERE i.sticky_port=p) ORDER BY md5($1||p::text) LIMIT 1`, s.Config.Account).Scan(&port)
	return port, err
}

type countryGatewayFactory func(context.Context, string, network.Proxy, network.ProxyCredentials) (*network.Gateway, error)

func (c Container) verifyCountryCandidate(ctx context.Context, s countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
	return c.verifyCountryCandidateWithGateway(ctx, s, p, v, c.newIdentityGateway)
}
func (c Container) verifyCountryCandidateWithGateway(ctx context.Context, s countrySnapshot, p countryProxy, v countryCandidate, gatewayFor countryGatewayFactory) (countryCandidate, proxyselection.Evaluation) {
	return c.verifyCountryCandidateUsing(ctx, s, p, v, gatewayFor, c.DB, c.Secrets.GetProxyAtVersion)
}

func (c Container) verifyCountryCandidateUsing(ctx context.Context, s countrySnapshot, p countryProxy, v countryCandidate, gatewayFor countryGatewayFactory, query countryQuery, passwordFor func(context.Context, string, string, string) ([]byte, error)) (countryCandidate, proxyselection.Evaluation) {
	unknown := func(reason string) (countryCandidate, proxyselection.Evaluation) {
		return v, proxyselection.Evaluation{Health: proxyselection.Unknown, Reason: reason}
	}
	unusable := func(reason string, all bool) (countryCandidate, proxyselection.Evaluation) {
		return v, proxyselection.Evaluation{Health: proxyselection.Unusable, Reason: reason, ExhaustProvider: all}
	}
	if !ProxyAdapterRegistered(p.Adapter) {
		return unusable("driver unavailable", true)
	}
	cap := proxyAdapterByName(p.Adapter).Capabilities()
	if v.Target != "" && !cap.StickySession && !cap.FixedEgress {
		return unusable("driver does not promise stable identity", true)
	}
	if p.Status != "healthy" {
		return unusable("configured proxy endpoint is not healthy", true)
	}
	var password []byte
	var err error
	if p.SecretRef != "" {
		password, err = passwordFor(ctx, p.ID, p.SecretRef, p.SecretVersion)
		if err != nil {
			return unknown("proxy credential unavailable")
		}
		defer wipe(password)
	}
	user := p.Username
	if cap.StickySession {
		target := v.Target
		if target == "" && s.Stage == proxyselection.Auto && !s.LegacyFallback {
			target = s.LearnedCountry
		}
		user = proxySessionUsername(p.Adapter, p.Username, target, v.Session, target != "" && cap.CountryTargeting)
	}
	proxy := network.Proxy{ID: p.ID, Type: network.ProxyType(p.Type), Host: p.Host, Port: v.Port, Status: network.StatusHealthy}
	probeCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	gateway, err := gatewayFor(probeCtx, s.Config.Account, proxy, network.ProxyCredentials{Username: user, Password: string(password)})
	if err != nil {
		return unknown("identity gateway unavailable")
	}
	geo, err := observeStickyCandidate(probeCtx, gateway)
	if err != nil {
		if network.IsProxyAuthFailure(err) {
			return unusable("proxy authentication rejected", true)
		}
		return unknown("country observation unavailable")
	}
	if net.ParseIP(geo.IP) == nil || net.ParseIP(geo.IP).To4() == nil {
		return unknown("IPv4 observation unavailable")
	}
	if _, err = time.LoadLocation(geo.Timezone); err != nil || geo.Timezone == "" {
		return unknown("timezone observation unavailable")
	}
	if codes, e := proxyselection.Normalize([]string{geo.CountryCode}); e != nil || len(codes) != 1 || codes[0] == proxyselection.Auto {
		return unknown("country observation unavailable")
	}
	if v.Target != "" && !strings.EqualFold(geo.CountryCode, v.Target) {
		return unusable("candidate is in a different country", cap.FixedEgress)
	}
	if v.Geo.IP != "" && v.Geo.IP != geo.IP && (v.Target != "" || cap.StickySession || cap.FixedEgress) {
		return unusable("candidate identity changed", cap.FixedEgress)
	}
	// Two independent connections must agree; never infer stickiness from one
	// keep-alive socket. Neither candidate gateway changes the serving route.
	for i := 0; i < 2 && v.Target != ""; i++ {
		fresh, e := gatewayFor(probeCtx, s.Config.Account, proxy, network.ProxyCredentials{Username: user, Password: string(password)})
		if e != nil {
			return unknown("identity gateway unavailable")
		}
		ip, e := parallelGatewayExitIP(probeCtx, fresh)
		fresh.CloseIdleConnections()
		if e != nil {
			if ctx.Err() != nil {
				return unknown("probe interrupted")
			}
			return unknown("reconnect observation unavailable")
		}
		if ip != geo.IP {
			return unusable("candidate is not stable across connections", cap.FixedEgress)
		}
	}
	var collision bool
	err = query.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1
 AND (exit_ip=$2::inet OR subnet_key=host(network(set_masklen($2::inet,24)))||'/24'))`, s.Config.Account, geo.IP).Scan(&collision)
	if err != nil {
		return unknown("isolation check unavailable")
	}
	if collision {
		return unusable("candidate conflicts with another account", cap.FixedEgress)
	}
	v.Geo = geo
	v.ObservedAt = time.Now().UTC()
	if v.FirstSeen.IsZero() {
		v.FirstSeen = time.Now().UTC()
	}
	return v, proxyselection.Evaluation{Health: proxyselection.Usable, Candidate: v}
}

type countryProbe func(context.Context, countrySnapshot, countryProxy, countryCandidate) (countryCandidate, proxyselection.Evaluation)

func (c Container) maintainOrderedCountry(ctx context.Context, account string) error {
	return c.maintainOrderedCountryWithProbe(ctx, account, c.verifyCountryCandidate)
}
func (c Container) maintainOrderedCountryWithProbe(ctx context.Context, account string, probe countryProbe) error {
	s, err := c.claimCountryWork(ctx, account)
	if err != nil {
		return err
	}
	if s.Config.Mode != "proxy_required" {
		return nil
	}
	if proxyselection.Legacy(s.Config.Countries) {
		return c.maintainLegacyProxyControlPlane(ctx, account)
	}
	originalProbe := probe
	probe = func(ctx context.Context, s countrySnapshot, p countryProxy, v countryCandidate) (countryCandidate, proxyselection.Evaluation) {
		v, e := originalProbe(ctx, s, p, v)
		if e.Health == proxyselection.Usable {
			v.ObservedAt = time.Now().UTC()
			e.Candidate = v
		}
		return v, e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 22*time.Second)
	defer cancel()
	var incumbent *countryCandidate
	var incumbentUnknown bool
	if s.Health && s.IP != "" && s.Applied == s.Config.Revision {
		if p, ok := s.proxy(s.Active); ok {
			port := p.Port
			if ProxyAdapterCapabilities(p.Adapter).DedicatedPort {
				port = s.Port
			}
			v, e := probe(probeCtx, s, p, countryCandidate{ProxyID: p.ID, Session: s.Session, Target: s.Target, Port: port, Geo: stickyGeo{IP: s.IP}})
			if e.Health == proxyselection.Usable {
				incumbent = &v
			} else {
				incumbentUnknown = e.Health == proxyselection.Unknown
			}
		}
	}
	providers := make([]proxyselection.Provider, 0, len(s.Config.Pool))
	for _, p := range s.Config.Pool {
		attempts := 1
		if ProxyAdapterCapabilities(p.Adapter).StickySession {
			attempts = 6
		}
		providers = append(providers, proxyselection.Provider{ID: p.ID, Attempts: attempts})
	}
	// Retain the exact healthy incumbent within its country before considering
	// another provider of equal priority. Higher countries still come first.
	sort.SliceStable(providers, func(i, j int) bool { return providers[i].ID == s.Active && providers[j].ID != s.Active })
	savedCandidateUsed := false
	decision := (proxyselection.Selector{}).Select(probeCtx, s.Scope, s.Config.Countries, providers, &s.Checkpoint, func(ctx context.Context, r proxyselection.Request) proxyselection.Evaluation {
		if incumbent != nil && incumbent.ProxyID == r.Provider && strings.EqualFold(incumbent.Geo.CountryCode, r.Country) && (incumbent.Target == r.Country || ProxyAdapterCapabilities(func() string { p, _ := s.proxy(r.Provider); return p.Adapter }()).FixedEgress) {
			v := *incumbent
			v.Target = r.Country
			return proxyselection.Evaluation{Health: proxyselection.Usable, Candidate: v}
		}
		p, ok := s.proxy(r.Provider)
		if !ok {
			return proxyselection.Evaluation{Health: proxyselection.Unknown}
		}
		v := countryCandidate{ProxyID: p.ID, Target: r.Country, Port: p.Port}
		if !savedCandidateUsed && s.Candidate != nil && s.Candidate.ProxyID == p.ID && s.Candidate.Target == r.Country && time.Since(s.Candidate.FirstSeen) < 2*time.Minute {
			v = *s.Candidate
			savedCandidateUsed = true
		} else {
			if ProxyAdapterCapabilities(p.Adapter).StickySession {
				var e error
				v.Session, e = countrySession()
				if e != nil {
					return proxyselection.Evaluation{Health: proxyselection.Unknown}
				}
			}
			if ProxyAdapterCapabilities(p.Adapter).DedicatedPort {
				var e error
				v.Port, e = c.countryCandidatePort(ctx, s)
				if e != nil {
					return proxyselection.Evaluation{Health: proxyselection.Unknown}
				}
			}
		}
		_, result := probe(ctx, s, p, v)
		return result
	})
	if decision.Kind == "selected" {
		v := decision.Candidate.(countryCandidate)
		same := incumbent != nil && v.ProxyID == incumbent.ProxyID && v.Session == incumbent.Session && v.Port == incumbent.Port && v.Geo.IP == incumbent.Geo.IP && v.Target == incumbent.Target
		if incumbent != nil && !same && time.Since(v.FirstSeen) < 10*time.Second {
			return c.finishCountryWork(ctx, s, incumbent, &v, "stabilizing", "Preferred-country candidate is being rechecked before switching", false)
		}
		return c.finishCountryWork(ctx, s, &v, nil, "ready", "Country and isolated proxy identity verified", true)
	}
	if decision.Kind == "legacy" {
		return c.maintainCountryAuto(ctx, s)
	}
	status := "searching"
	detail := decision.Reason
	if incumbentUnknown {
		detail = "Current identity observation unavailable; country search remains fail-closed"
	}
	return c.finishCountryWork(ctx, s, incumbent, nil, status, detail, false)
}

func (c Container) finishCountryWork(ctx context.Context, s countrySnapshot, active, candidate *countryCandidate, status, detail string, selected bool) error {
	tx, err := countryRouteTx(ctx, c.DB, s.Config.Account)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedFence int64
	if err = tx.QueryRowContext(ctx, `SELECT fence FROM account_proxy_selection WHERE account_id=$1 FOR UPDATE NOWAIT`, s.Config.Account).Scan(&lockedFence); err != nil {
		return err
	}
	current, err := loadCountrySnapshot(ctx, tx, s.Config.Account)
	if err != nil {
		return err
	}
	if current.Fence != s.Fence || current.Scope != s.Scope || current.Epoch != s.Epoch || current.Active != s.Active {
		return ErrStaleAccountNetwork
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('dob.country_writer',$1,true)`, fmt.Sprintf("%s:%d", s.Config.Account, s.Fence)); err != nil {
		return err
	}
	if active != nil {
		at := active.ObservedAt
		if at.IsZero() {
			at = active.FirstSeen
		}
		if at.IsZero() || at.After(time.Now()) || time.Since(at) > 30*time.Second {
			return ErrNetworkNotReady
		}
	}
	if selected && active != nil && !countryDecisionCurrent(s, active.Target, time.Now()) {
		return ErrNetworkNotReady
	}
	stage := s.Stage
	applied := s.Applied
	if active != nil {
		if selected {
			stage = active.Target
			if stage == "" {
				stage = proxyselection.Auto
			}
			applied = s.Config.Revision
		}
		if err = c.publishCountryIdentityTx(ctx, tx, s, *active); err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE account_network_identities SET last_health_ok=false,last_health_at=now() WHERE account_id=$1`, s.Config.Account); err != nil {
			return err
		}
	}
	checkpoint, _ := json.Marshal(s.Checkpoint)
	var rawCandidate any
	if candidate != nil {
		b, _ := json.Marshal(candidate)
		rawCandidate = string(b)
	}
	_, err = tx.ExecContext(ctx, `UPDATE account_proxy_selection SET checkpoint=$2,candidate=$3,active_stage=$4,applied_revision=$5,
 status=$6,detail=$7,checked_at=now(),legacy_lease_until=NULL,updated_at=now() WHERE account_id=$1`, s.Config.Account, checkpoint, rawCandidate, stage, applied, status, detail)
	if err != nil {
		return err
	}
	// Allocation, constraints and row updates may have waited after initial checks.
	if active != nil {
		at := active.ObservedAt
		if at.IsZero() {
			at = active.FirstSeen
		}
		if at.IsZero() || at.After(time.Now()) || time.Since(at) > 30*time.Second {
			return ErrNetworkNotReady
		}
	}
	if selected && active != nil && !countryDecisionCurrent(s, active.Target, time.Now()) {
		return ErrNetworkNotReady
	}
	return tx.Commit()
}

func (c Container) publishCountryIdentityTx(ctx context.Context, tx *sql.Tx, s countrySnapshot, v countryCandidate) error {
	p, ok := s.proxy(v.ProxyID)
	if !ok {
		return ErrStaleAccountNetwork
	}
	if p.Status != "healthy" {
		return ErrNetworkNotReady
	}
	if ProxyAdapterCapabilities(p.Adapter).DedicatedPort {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('sticky-port-allocation',0))`); err != nil {
			return err
		}
		var occupied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM account_network_identities WHERE account_id<>$1 AND sticky_port=$2)`, s.Config.Account, v.Port).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return ErrStickyPortUnavailable
		}
	}
	// Lock definitions before final publication. NOWAIT avoids inversion with an
	// administrator updating credentials/pool then invalidating this account.
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT p.id::text FROM proxies p JOIN account_proxy_pool ap ON ap.proxy_id=p.id
 WHERE p.id=$2 AND ap.account_id=$1 AND ap.enabled AND p.status='healthy' FOR SHARE OF p,ap NOWAIT`, s.Config.Account, v.ProxyID).Scan(&id); err != nil {
		return err
	}
	if p.SecretRef != "" {
		var secretVersion string
		err := tx.QueryRowContext(ctx, `SELECT xmin::text||':'||updated_at::text FROM secrets WHERE proxy_id=$1 AND id=$2 FOR SHARE NOWAIT`, p.ID, p.SecretRef).Scan(&secretVersion)
		if err != nil {
			return err
		}
		if secretVersion != p.SecretVersion {
			return ErrStaleAccountNetwork
		}
	}
	// A credential trigger serializes on the selection row; compare snapshot
	// again after taking the route's row locks, before touching the live identity.
	latest, err := loadCountrySnapshot(ctx, tx, s.Config.Account)
	if err != nil {
		return err
	}
	if latest.Scope != s.Scope || latest.Fence != s.Fence {
		return ErrStaleAccountNetwork
	}
	portChanged := ProxyAdapterCapabilities(p.Adapter).DedicatedPort && s.Port != v.Port
	changed := s.Active != v.ProxyID || s.Session != v.Session || s.IP != v.Geo.IP || portChanged || s.Target != v.Target
	if !changed {
		// The incumbent probe predates the higher-country search. Lock and compare
		// its observation version before publishing any part of that older proof.
		var version string
		if err = tx.QueryRowContext(ctx, `SELECT xmin::text FROM proxy_runtime_state
 WHERE account_id=$1 AND proxy_id=$2 AND provider=$3 FOR UPDATE`, s.Config.Account, v.ProxyID, s.Config.Provider).Scan(&version); err != nil {
			return err
		}
		if s.HealthVersion == "" || version != s.HealthVersion {
			return proxycontrol.ErrStateConflict
		}
	}
	// A new route is not available to provider requests until this transaction
	// commits. Its fresh proof may replace old-route health while bumping the
	// generation, so delayed observations of that old route can no longer apply.
	observedAt := v.ObservedAt
	if observedAt.IsZero() {
		observedAt = v.FirstSeen
	}
	if _, err = tx.ExecContext(ctx, `UPDATE network_profiles SET proxy_id=$2,updated_at=now() WHERE account_id=$1 AND mode='proxy_required'`, s.Config.Account, v.ProxyID); err != nil {
		return err
	}
	var stickyPort any
	if ProxyAdapterCapabilities(p.Adapter).DedicatedPort {
		stickyPort = v.Port
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_network_identities(account_id,sticky_session,sticky_port,selected_target_country,
 exit_ip,subnet_key,country,country_code,timezone,locale,asn,last_health_at,last_health_ok,fallback_active,rotation_started_at,updated_at)
 VALUES($1,NULLIF($2,''),$3,NULLIF($4,''),$5::inet,host(network(set_masklen($5::inet,24)))||'/24',$6,lower($7),$8,$9,$10,$12,true,false,NULL,now())
 ON CONFLICT(account_id) DO UPDATE SET sticky_session=EXCLUDED.sticky_session,sticky_port=EXCLUDED.sticky_port,
 selected_target_country=EXCLUDED.selected_target_country,exit_ip=EXCLUDED.exit_ip,subnet_key=EXCLUDED.subnet_key,country=EXCLUDED.country,
 country_code=EXCLUDED.country_code,timezone=EXCLUDED.timezone,locale=EXCLUDED.locale,asn=EXCLUDED.asn,last_health_at=EXCLUDED.last_health_at,
 last_health_ok=true,fallback_active=$11,rotation_started_at=NULL,updated_at=now()`, s.Config.Account, v.Session, stickyPort, v.Target, v.Geo.IP, v.Geo.Country, v.Geo.CountryCode, v.Geo.Timezone, geoctx.LocaleForCountry(v.Geo.CountryCode), v.Geo.ASN, v.Target == "" && s.LegacyFallback, observedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO proxy_runtime_state(account_id,proxy_id,provider,health_state,circuit_state,generation,
 consecutive_successes,last_checked_at,last_success_at) VALUES($1,$2,$3,'healthy','closed',1,1,$5,$5)
 ON CONFLICT(account_id,proxy_id,provider) DO UPDATE SET health_state='healthy',circuit_state='closed',
 generation=proxy_runtime_state.generation+CASE WHEN $4 THEN 1 ELSE 0 END,consecutive_failures=0,consecutive_successes=proxy_runtime_state.consecutive_successes+1,
 retry_after=NULL,last_error_class=NULL,last_error_detail=NULL,last_checked_at=$5,last_success_at=$5,
 half_open_probe_in_flight=false,half_open_probe_lease_until=NULL,updated_at=now()`, s.Config.Account, v.ProxyID, s.Config.Provider, changed, observedAt)
	return err
}

// AUTO calls the existing selection/recovery implementation only after the
// configured explicit tiers have current exhaustion evidence. A short lease
// permits its established writers; no new direct route is introduced.
func (c Container) maintainCountryAuto(ctx context.Context, s countrySnapshot) error {
	return c.maintainCountryAutoWithLegacy(ctx, s, c.maintainLegacyProxyControlPlane)
}
func (c Container) maintainCountryAutoWithLegacy(ctx context.Context, s countrySnapshot, legacy func(context.Context, string) error) error {
	tx, err := countryRouteTx(ctx, c.DB, s.Config.Account)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedFence int64
	if err = tx.QueryRowContext(ctx, `SELECT fence FROM account_proxy_selection WHERE account_id=$1 FOR UPDATE NOWAIT`, s.Config.Account).Scan(&lockedFence); err != nil {
		return err
	}
	current, err := loadCountrySnapshot(ctx, tx, s.Config.Account)
	if err != nil {
		return err
	}
	if current.Scope != s.Scope || current.Fence != s.Fence || current.Epoch != s.Epoch {
		return ErrStaleAccountNetwork
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('dob.country_writer',$1,true)`, fmt.Sprintf("%s:%d", s.Config.Account, s.Fence)); err != nil {
		return err
	}
	if !countryDecisionCurrent(s, "", time.Now()) {
		return ErrNetworkNotReady
	}
	checkpoint, _ := json.Marshal(s.Checkpoint)
	if _, err = tx.ExecContext(ctx, `UPDATE account_proxy_selection SET active_stage='AUTO',applied_revision=0,checkpoint=$2,
 candidate=NULL,status='recovering',detail='Higher countries exhausted; legacy AUTO recovery',legacy_lease_until=now()+interval '25 seconds' WHERE account_id=$1`, s.Config.Account, checkpoint); err != nil {
		return err
	}
	if s.Target != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE account_network_identities SET selected_target_country=NULL,sticky_session=NULL,exit_ip=NULL,subnet_key=NULL,
 last_health_ok=false,last_health_at=NULL,rotation_started_at=NULL,fallback_active=false WHERE account_id=$1`, s.Config.Account); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	legacyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	legacyErr := legacy(context.WithValue(legacyCtx, countryAutoRecoveryKey{}, true), s.Config.Account)
	cancel()
	// Legacy rotation can change the active endpoint and epoch. Configuration
	// scope/fence must still match; its own profile switch is lease-authorized.
	tx, err = countryRouteTx(ctx, c.DB, s.Config.Account)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT fence FROM account_proxy_selection WHERE account_id=$1 FOR UPDATE NOWAIT`, s.Config.Account).Scan(&lockedFence); err != nil {
		return err
	}
	current, err = loadCountrySnapshot(ctx, tx, s.Config.Account)
	if err != nil {
		return err
	}
	if current.Scope != s.Scope || current.Fence != s.Fence {
		return ErrStaleAccountNetwork
	}
	var verified bool
	verified = legacyErr == nil && current.Health && current.IP != "" && current.Target == "" && countryDecisionCurrent(s, "", time.Now())
	status := "searching"
	detail := "Legacy AUTO recovery pending"
	applied := int64(0)
	if verified {
		status = "ready"
		detail = "Legacy AUTO identity verified after higher-country exhaustion"
		applied = s.Config.Revision
	}
	if _, err = tx.ExecContext(ctx, `UPDATE account_proxy_selection SET legacy_lease_until=NULL,applied_revision=$2,status=$3,detail=$4,checked_at=now(),updated_at=now() WHERE account_id=$1`, s.Config.Account, applied, status, detail); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if legacyErr == nil && !verified {
		return ErrNetworkNotReady
	}
	return legacyErr
}

// A route-lock wait must not turn expired negative evidence into permission to
// descend. Retaining an already verified incumbent needs no new descent proof.
func countryDecisionCurrent(s countrySnapshot, target string, now time.Time) bool {
	if target == "" {
		target = proxyselection.Auto
	}
	for _, country := range s.Config.Countries {
		if country == target {
			return true
		}
		sweep := s.Checkpoint.Countries[country]
		if sweep == nil || !sweep.ExhaustedUntil.After(now) {
			return false
		}
	}
	return false
}
