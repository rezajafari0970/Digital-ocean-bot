package residentialsync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportcore"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/protocolprobe"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/residentialperf"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
)

func protocolScope() (bool, []string) {
	value := strings.TrimSpace(os.Getenv("DOB_UDP_CAPABILITY_PANELS"))
	if value == "all" {
		return true, nil
	}
	if value == "" || value == "none" {
		return false, nil
	}
	ids := []string{}
	for _, id := range strings.Split(value, ",") {
		id = strings.ToLower(strings.TrimSpace(id))
		if !residentialperf.UUID.MatchString(id) {
			return false, nil
		}
		ids = append(ids, id)
	}
	return false, ids
}
func protocolEnabled(panel string) bool {
	all, ids := protocolScope()
	if all {
		return true
	}
	for _, id := range ids {
		if strings.EqualFold(panel, id) {
			return true
		}
	}
	return false
}

// Origin includes the deployment generation and SSH identity reference. Evidence
// from a replacement node or another receiver never authorizes this receiver.
func (s Service) protocolTarget(ctx context.Context, panel string) (provisioning.Target, string, error) {
	var target provisioning.Target
	var generation int64
	err := s.DB.QueryRowContext(ctx, `SELECT p.account_id::text,p.droplet_id::text,d.host,
 COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref',''),d.postinstall_generation
 FROM panel_instances p JOIN deployments d ON d.droplet_id=p.droplet_id
 JOIN droplets dr ON dr.id=p.droplet_id
 WHERE p.id=$1 AND p.enabled AND d.state='PANEL_COMPLETE'
 AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now())`, panel).
		Scan(&target.AccountID, &target.DropletID, &target.Host, &target.User, &target.KeySecretRef, &generation)
	if err != nil {
		return target, "", err
	}
	target.Port = 22
	raw, _ := json.Marshal(struct {
		Panel      string
		Target     provisioning.Target
		Generation int64
	}{panel, target, generation})
	digest := sha256.Sum256(raw)
	return target, hex.EncodeToString(digest[:]), nil
}

func protocolDriver(proxy rp) string {
	if proxy.Relay != nil {
		return protocolprobe.DriverManagedRelay
	}
	return protocolprobe.DriverSOCKS5
}

func protocolCandidates(proxies []rp) ([]protocolprobe.Candidate, error) {
	candidates := []protocolprobe.Candidate{}
	for _, p := range proxies {
		if p.Type != "socks5" {
			continue
		}
		outbound, err := proxyOutbound(p)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, protocolprobe.Candidate{ID: p.ID, Driver: protocolDriver(p), Digest: protocolprobe.Fingerprint(protocolDriver(p), outbound, p.DonorAccount, p.DonorDroplet, p.DonorPlan), Outbound: outbound})
	}
	if len(candidates) > protocolprobe.MaxCandidates {
		return nil, errors.New("protocol probe candidate bound exceeded")
	}
	return candidates, nil
}

func (s Service) hydrateProtocolHealth(ctx context.Context, panel string, p *routePolicy) error {
	if !protocolEnabled(panel) {
		return nil
	}
	for i := range p.Proxies {
		p.Proxies[i].UDPRequired = true
		p.Proxies[i].UDPVerified = false
		p.Proxies[i].QUICVerified = false
	}
	_, origin, err := s.protocolTarget(ctx, panel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT proxy_id,transport_hash,code='ok' FROM panel_protocol_health
 WHERE panel_id=$1 AND origin_hash=$2 AND quic_ok
 AND checked_at>now()-interval '10 minutes' AND checked_at<=now()`, panel, origin)
	if err != nil {
		return err
	}
	type proof struct {
		digest string
		full   bool
	}
	evidence := map[string]proof{}
	for rows.Next() {
		var id, digest string
		var full bool
		if err = rows.Scan(&id, &digest, &full); err != nil {
			rows.Close()
			return err
		}
		evidence[id] = proof{digest, full}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i := range p.Proxies {
		proxy := &p.Proxies[i]
		outbound, e := proxyOutbound(*proxy)
		if e != nil {
			return e
		}
		proof := evidence[proxy.ID]
		proxy.QUICVerified = proxy.Type == "socks5" && proof.digest == protocolprobe.Fingerprint(protocolDriver(*proxy), outbound, proxy.DonorAccount, proxy.DonorDroplet, proxy.DonorPlan)
		proxy.UDPVerified = proxy.QUICVerified && proof.full
	}
	return nil
}

type protocolLease struct{ Panel, Token string }

func (s Service) claimProtocolProbe(ctx context.Context) (protocolLease, error) {
	all, ids := protocolScope()
	if !all && len(ids) == 0 {
		return protocolLease{}, sql.ErrNoRows
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO panel_protocol_probe_schedule(panel_id)
 SELECT p.id FROM panel_instances p JOIN droplets dr ON dr.id=p.droplet_id
 CROSS JOIN residential_routing_control c
 WHERE c.singleton AND c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids))
 AND p.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now())
 AND ($1 OR p.id::text=ANY($2::text[]))
 AND EXISTS(SELECT 1 FROM deployments d WHERE d.droplet_id=p.droplet_id AND d.state='PANEL_COMPLETE')
 ON CONFLICT DO NOTHING`, all, pq.Array(ids))
	if err != nil {
		return protocolLease{}, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return protocolLease{}, err
	}
	lease := protocolLease{Token: hex.EncodeToString(nonce[:])}
	err = s.DB.QueryRowContext(ctx, `WITH candidate AS (
 SELECT s.panel_id FROM panel_protocol_probe_schedule s
 JOIN panel_instances p ON p.id=s.panel_id JOIN droplets dr ON dr.id=p.droplet_id
 CROSS JOIN residential_routing_control c
 WHERE c.singleton AND c.enabled AND (c.fleet OR p.id=ANY(c.panel_ids))
 AND p.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR dr.expires_at>now())
 AND ($1 OR p.id::text=ANY($2::text[])) AND s.next_probe_at<=now() AND s.lease_until<now()
 ORDER BY s.next_probe_at,s.panel_id FOR UPDATE OF s SKIP LOCKED LIMIT 1
 ) UPDATE panel_protocol_probe_schedule s SET lease_token=$3,lease_until=now()+interval '100 seconds'
 FROM candidate c WHERE s.panel_id=c.panel_id RETURNING s.panel_id::text`, all, pq.Array(ids), lease.Token).Scan(&lease.Panel)
	return lease, err
}

func (s Service) finishProtocolProbe(ctx context.Context, lease protocolLease, origin string, observed []protocolprobe.Observation, available bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var panel string
	err = tx.QueryRowContext(ctx, `SELECT panel_id::text FROM panel_protocol_probe_schedule
 WHERE panel_id=$1 AND lease_token=$2 AND lease_until>now() FOR UPDATE`, lease.Panel, lease.Token).Scan(&panel)
	if err != nil {
		return err
	}
	if available {
		if !protocolEnabled(panel) {
			return errors.New("protocol probe scope changed")
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM panel_protocol_health WHERE panel_id=$1", panel)
		if err != nil {
			return err
		}
		for _, o := range observed {
			_, err = tx.ExecContext(ctx, `INSERT INTO panel_protocol_health(panel_id,proxy_id,origin_hash,transport_hash,dns_ok,quic_ok,code,checked_at,driver)
 VALUES($1,$2,$3,$4,$5,$6,$7,now(),$8)`, panel, o.ID, origin, o.Digest, o.DNS, o.QUIC, o.Code, o.Driver)
			if err != nil {
				return err
			}
		}
		// Only this hint is changed; the established writer owns configuration,
		// verifies the current policy again, and proves the complete native plan.
		_, err = tx.ExecContext(ctx, "UPDATE panel_routing_state SET next_check_at=LEAST(next_check_at,now()) WHERE panel_id=$1", panel)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE panel_protocol_probe_schedule SET lease_token='',lease_until='-infinity',
 next_probe_at=now()+CASE WHEN $3 THEN interval '2 minutes' ELSE interval '30 seconds' END,
 last_error=CASE WHEN $3 THEN '' ELSE 'probe_unavailable' END WHERE panel_id=$1 AND lease_token=$2`, panel, lease.Token, available)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s Service) observeProtocols(ctx context.Context, lease protocolLease) (err error) {
	// No network probing under the configuration fence or on the controller path.
	defer func() {
		if err != nil {
			c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_ = s.finishProtocolProbe(c, lease, "", nil, false)
		}
	}()
	p, _, err := s.policy(ctx, lease.Panel)
	if err != nil {
		return err
	}
	candidates, err := protocolCandidates(p.Proxies)
	if err != nil {
		return err
	}
	target, origin, err := s.protocolTarget(ctx, lease.Panel)
	if err != nil {
		return err
	}
	observations := []protocolprobe.Observation{}
	if len(candidates) > 0 {
		if target.KeySecretRef == "" {
			return errors.New("protocol probe key unavailable")
		}
		key, e := s.Secrets.Get(ctx, target.AccountID, target.KeySecretRef)
		if e != nil {
			return errors.New("protocol probe key unavailable")
		}
		defer func() {
			for i := range key {
				key[i] = 0
			}
		}()
		input, e := json.Marshal(protocolprobe.Request{Candidates: candidates, CoreHashes: transportcore.CoreHashes()})
		if e != nil {
			return e
		}
		defer func() {
			for i := range input {
				input[i] = 0
			}
		}()
		if len(input) > 256*1024 {
			return errors.New("protocol probe input bound exceeded")
		}
		command := "timeout --signal=TERM --kill-after=3s 65s python3 -c 'import base64;exec(base64.b64decode(\"" + base64.StdEncoding.EncodeToString([]byte(protocolprobe.Script)) + "\"))'"
		ssh := provisioning.SSHClient{Timeout: 8 * time.Second, OutputLimit: 65536, HostKeys: provisioning.SQLHostKeyPins{DB: s.DB}}
		result, e := ssh.RunInput(ctx, target, key, command, input)
		if e != nil {
			return errors.New("protocol probe transport unavailable")
		}
		observations, e = protocolprobe.Decode(result.Stdout, candidates)
		if e != nil {
			return e
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	_, currentOrigin, e := s.protocolTarget(ctx, lease.Panel)
	if e != nil || currentOrigin != origin {
		return errors.New("protocol probe origin changed")
	}
	// Publication also matches the transport digest, so an endpoint/credential
	// edit during this read or after commit cannot admit the stale observation.
	return s.finishProtocolProbe(ctx, lease, origin, observations, true)
}

func (s Service) RunProtocolHealth(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			timer := time.NewTicker(5 * time.Second)
			defer timer.Stop()
			for {
				supervision.Pulse(ctx)
				c, cancel := context.WithTimeout(ctx, 5*time.Second)
				lease, err := s.claimProtocolProbe(c)
				cancel()
				if err == nil {
					c, cancel = context.WithTimeout(ctx, 75*time.Second)
					err = supervision.Work(c, func(c context.Context) error { return s.observeProtocols(c, lease) })
					cancel()
				}
				if err != nil && !errors.Is(err, sql.ErrNoRows) && ctx.Err() == nil {
					log.Printf("protocol health evidence unavailable panel=%s", lease.Panel)
				}
				supervision.Idle(ctx)
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
			}
		}()
	}
	wg.Wait()
}
