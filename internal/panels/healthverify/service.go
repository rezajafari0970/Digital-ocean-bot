package healthverify

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}

type Service struct {
	DB         *sql.DB
	Secrets    Secrets
	Runner     Runner
	ManagedKey string
	Repair     func(context.Context, string) error
}

func (s Service) Run(ctx context.Context) error {
	if s.DB == nil || s.Secrets == nil || s.ManagedKey == "" {
		return ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT pi.id::text
FROM panel_instances pi
JOIN droplets dr ON dr.id=pi.droplet_id
JOIN deployments d ON d.droplet_id=dr.id
JOIN xui_panel_deployments x ON x.droplet_id=dr.id AND x.generation=d.postinstall_generation
WHERE pi.enabled=true
  AND dr.state IN ('READY','EXPIRING','RETIRING')
  AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
  AND d.state='PANEL_COMPLETE' AND x.state='COMPLETED'
ORDER BY COALESCE(pi.health_checked_at,'epoch'),pi.id
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	runner := s.Runner
	if runner == nil {
		runner = LocalRunner{}
	}
	s.Runner = runner
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			cctx, cancel := context.WithTimeout(ctx, 35*time.Second)
			probeErr := s.runOne(cctx, id)
			cancel()
			if probeErr != nil {
				failures := s.recordFailure(ctx, id, probeErr)
				if failures >= UnhealthyFailureThreshold && s.Repair != nil {
					rctx, rcancel := context.WithTimeout(ctx, 45*time.Second)
					_ = s.Repair(rctx, id)
					rcancel()
				}
			} else {
				s.recordSuccess(ctx, id)
			}
		}()
	}
	wg.Wait()
	return ctx.Err()
}

func (s Service) runOne(ctx context.Context, panelID string) error {
	var accountID, host, uuidRef, publicKey, shortID, sni string
	var port int
	err := s.DB.QueryRowContext(ctx, `
SELECT pi.account_id::text,d.host,rc.uuid_secret_ref,rc.public_key,rc.short_id,rs.server_name,
(SELECT pii.port FROM panel_inbound_inventory pii WHERE pii.panel_id=pi.id AND pii.present=true AND pii.enabled=true AND pii.protocol='vless' AND pii.security='reality' ORDER BY pii.port LIMIT 1)
FROM panel_instances pi
JOIN deployments d ON d.droplet_id=pi.droplet_id
JOIN reality_credentials rc ON rc.panel_id=pi.id AND rc.managed_key=$2
JOIN reality_target_selections rs ON rs.panel_id=pi.id
WHERE pi.id=$1 AND pi.enabled=true
`, panelID, s.ManagedKey).Scan(&accountID, &host, &uuidRef, &publicKey, &shortID, &sni, &port)
	if err != nil {
		return err
	}
	if uuidRef == "" || host == "" || port < 1 {
		return ErrInvalid
	}
	uuidRaw, err := s.Secrets.Get(ctx, accountID, uuidRef)
	if err != nil {
		return err
	}
	defer credentials.Wipe(uuidRaw)
	uuid := strings.TrimSpace(string(uuidRaw))
	_, err = Probe(ctx, s.Runner, RealityClient{UUID: uuid, Host: host, Port: port, SNI: sni, PublicKey: publicKey, ShortID: shortID})
	return err
}

func (s Service) recordFailure(ctx context.Context, id string, probeErr error) int {
	msg := probeErr.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	var failures int
	err := s.DB.QueryRowContext(ctx, `
UPDATE panel_instances SET
 health_failures=health_failures+1,
 health_successes=0,
 health_checked_at=now(),
 health_last_error=$2,
 health_state=CASE WHEN health_failures+1>=3 THEN 'UNHEALTHY' ELSE 'DEGRADED' END,
 updated_at=now()
WHERE id=$1 RETURNING health_failures
`, id, msg).Scan(&failures)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	return failures
}

func (s Service) recordSuccess(ctx context.Context, id string) {
	_, _ = s.DB.ExecContext(ctx, `
UPDATE panel_instances SET
 health_state='HEALTHY',health_failures=0,health_successes=health_successes+1,
 health_checked_at=now(),health_last_ok_at=now(),health_last_error=NULL,updated_at=now()
WHERE id=$1
`, id)
}
