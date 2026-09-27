package panelbootstrap

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/runtimecap"
)

var ErrRealityWarming = errors.New("reality stability warming")

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}

type Service struct {
	DB         *sql.DB
	Secrets    Secrets
	SSH        provisioning.SSHClient
	ManagedKey string
}

func (s Service) BootstrapPanel(ctx context.Context, p readyworker.Panel) error {
	if s.DB == nil || s.Secrets == nil || p.ID == "" || s.ManagedKey == "" {
		return errors.New("bootstrap config")
	}
	var acc, did, host, user, keyref, puser, pref, path, state string
	var pport int
	err := s.DB.QueryRowContext(ctx, `
SELECT pi.account_id::text,pi.droplet_id::text,d.host,
d.profile_snapshot->>'ssh_user',d.profile_snapshot->>'ssh_key_secret_ref',
x.username,x.password_secret_ref,x.port,x.web_path,r.state
FROM panel_instances pi
JOIN deployments d ON d.droplet_id=pi.droplet_id
JOIN droplets r ON r.id=pi.droplet_id
JOIN xui_panel_deployments x ON x.droplet_id=pi.droplet_id AND x.generation=d.postinstall_generation
WHERE pi.id=$1 AND pi.enabled=true
`, p.ID).Scan(&acc, &did, &host, &user, &keyref, &puser, &pref, &pport, &path, &state)
	if err != nil {
		return err
	}
	if state != "READY" {
		return errors.New("panel not ready")
	}
	target := provisioning.Target{AccountID: acc, DropletID: did, Host: host, User: user, KeySecretRef: keyref}
	exec := sanaei.SSHSessionExecutor{SSH: s.SSH, Target: target, PrivateKeySecretRef: keyref, PanelPasswordSecretRef: pref, AccountID: acc, Username: puser, Port: pport, BasePath: path, DialHost: "127.0.0.1", Secrets: s.Secrets}
	disc, err := sanaei.DiscoverWithExecutor(ctx, exec, "")
	if err != nil {
		return err
	}
	if err = (panels.SQLStore{DB: s.DB}).SaveDiscovery(ctx, p.ID, disc); err != nil {
		return err
	}
	snap, err := sanaei.ReadInventory(ctx, exec, p.ID)
	if err != nil {
		return err
	}
	if _, err = (inventory.SQLStore{DB: s.DB}).Sync(ctx, snap); err != nil {
		return err
	}
	key, err := s.Secrets.Get(ctx, acc, keyref)
	if err != nil {
		return err
	}
	defer credentials.Wipe(key)
	run := func(ctx context.Context, cmd string) (string, error) { return s.SSH.Run(ctx, target, key, cmd) }
	candidates := []reality.Candidate{{Target: "www.cloudflare.com", ServerName: "www.cloudflare.com", Port: 443}, {Target: "www.google.com", ServerName: "www.google.com", Port: 443}, {Target: "www.microsoft.com", ServerName: "www.microsoft.com", Port: 443}, {Target: "www.apple.com", ServerName: "www.apple.com", Port: 443}}
	policy := reality.StabilityPolicy{MinObservations: 3, MinEligibleRatio: .8, SwitchMargin: 5}
	result, err := reality.RunPanel(ctx, p.ID, reality.SSHProber{Run: run}, candidates, 3, 5, policy, reality.SQLStore{DB: s.DB}, nil)
	if err != nil {
		return err
	}
	if !result.Stable {
		return ErrRealityWarming
	}
	xrayPath, err := (runtimecap.XrayResolver{Run: run}).Resolve(ctx)
	if err != nil {
		return err
	}
	_, err = (credentials.Registry{DB: s.DB, Secrets: s.Secrets, Keys: credentials.XrayGenerator{Run: run, Binary: xrayPath}}).Ensure(ctx, acc, p.ID, s.ManagedKey)
	return err
}
