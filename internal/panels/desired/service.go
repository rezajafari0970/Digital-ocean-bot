package desired

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
)

var ErrMutationDisabled = errors.New("desired mutation disabled")

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
}
type Service struct {
	DB         *sql.DB
	Secrets    Secrets
	ManagedKey string
	Port       int
}

func (s Service) ReconcilePanel(ctx context.Context, p readyworker.Panel, dryRun bool) error {
	if !dryRun {
		return ErrMutationDisabled
	}
	if s.DB == nil || s.Secrets == nil || p.ID == "" || s.ManagedKey == "" || s.Port < 1 {
		return errors.New("desired config")
	}
	var acc, target, sni, ur, pr, sid string
	var tport int
	err := s.DB.QueryRowContext(ctx, `
SELECT pi.account_id::text,rs.target,rs.server_name,rs.port,
rc.uuid_secret_ref,rc.private_key_secret_ref,rc.short_id
FROM panel_instances pi
JOIN droplets d ON d.id=pi.droplet_id
JOIN reality_target_selections rs ON rs.panel_id=pi.id
JOIN reality_credentials rc ON rc.panel_id=pi.id AND rc.managed_key=$2
WHERE pi.id=$1 AND pi.enabled=true AND d.state='READY'
`, p.ID, s.ManagedKey).Scan(&acc, &target, &sni, &tport, &ur, &pr, &sid)
	if err != nil {
		return err
	}
	uuid, err := s.Secrets.Get(ctx, acc, ur)
	if err != nil {
		return err
	}
	defer credentials.Wipe(uuid)
	priv, err := s.Secrets.Get(ctx, acc, pr)
	if err != nil {
		return err
	}
	defer credentials.Wipe(priv)
	_, err = realityconfig.Build(realityconfig.Input{Remark: s.ManagedKey, Port: s.Port, UUID: string(uuid), Email: "managed", Target: target, TargetPort: tport, ServerName: sni, PrivateKey: string(priv), ShortID: sid})
	return err
}
