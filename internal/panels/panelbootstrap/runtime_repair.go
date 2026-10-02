package panelbootstrap

import (
	"context"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

func (s Service) RepairRuntimePanel(ctx context.Context, p readyworker.Panel) error {
	if s.DB == nil || s.Secrets == nil || p.ID == "" {
		return errors.New("runtime repair config")
	}
	var acc, did, host, user, keyref string
	err := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,
COALESCE(d.profile_snapshot->>'ssh_user','root'),
COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','')
FROM panel_instances pi
JOIN deployments d ON d.droplet_id=pi.droplet_id
WHERE pi.id=$1 AND pi.enabled=true`, p.ID).
		Scan(&acc, &did, &host, &user, &keyref)
	if err != nil {
		return err
	}
	if keyref == "" || host == "" {
		return errors.New("runtime repair identity incomplete")
	}
	claimed, err := s.claimRuntimeRepair(ctx, p.ID)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrRuntimeRepairCooldown
	}
	target := provisioning.Target{
		AccountID:    acc,
		DropletID:    did,
		Host:         host,
		User:         user,
		KeySecretRef: keyref,
	}
	repair := sanaei.PanelConfigurer{
		DB:       s.DB,
		Secrets:  s.Secrets,
		Runner:   s.SSH,
		Uploader: s.SSH,
	}
	repairErr := repair.RepairCompleted(ctx, acc, did, target)
	s.recordRuntimeRepair(ctx, p.ID, repairErr)
	return repairErr
}
