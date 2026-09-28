package globalreality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/desired"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/panelbootstrap"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/runtimecap"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}
type Service struct {
	DB      *sql.DB
	Secrets Secrets
	SSH     provisioning.SSHClient
}

func (s Service) ReconcilePanel(ctx context.Context, p readyworker.Panel, dry bool) error {
	if s.DB == nil || s.Secrets == nil || p.ID == "" {
		return errors.New("global reality config")
	}
	var enabled bool
	var portsRaw []byte
	e := s.DB.QueryRowContext(ctx, `SELECT enabled,ports FROM global_config_policies WHERE policy_key='reality'`).Scan(&enabled, &portsRaw)
	if errors.Is(e, sql.ErrNoRows) || !enabled {
		return nil
	}
	if e != nil {
		return e
	}
	var ports []int
	if json.Unmarshal(portsRaw, &ports) != nil || len(ports) == 0 {
		return errors.New("global reality ports")
	}
	if dry {
		return nil
	}
	var selectionExists bool
	if e = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reality_target_selections WHERE panel_id=$1)`, p.ID).Scan(&selectionExists); e != nil {
		return e
	}
	if !selectionExists {
		return (panelbootstrap.Service{DB: s.DB, Secrets: s.Secrets, SSH: s.SSH, ManagedKey: "dob:reality-primary:000001"}).BootstrapPanel(ctx, p)
	}
	var acc, did, host, user, keyref string
	e = s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1 AND pi.enabled=true`, p.ID).Scan(&acc, &did, &host, &user, &keyref)
	if e != nil {
		return e
	}
	key, e := s.Secrets.Get(ctx, acc, keyref)
	if e != nil {
		return e
	}
	defer credentials.Wipe(key)
	target := provisioning.Target{AccountID: acc, DropletID: did, Host: host, Port: 22, User: user, KeySecretRef: keyref}
	run := func(ctx context.Context, cmd string) (string, error) { return s.SSH.Run(ctx, target, key, cmd) }
	xray, e := (runtimecap.XrayResolver{Run: run}).Resolve(ctx)
	if e != nil {
		return e
	}
	reg := credentials.Registry{DB: s.DB, Secrets: s.Secrets, Keys: credentials.XrayGenerator{Run: run, Binary: xray}}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid reality port %d", port)
		}
		var existingProtocol, existingTransport, existingSecurity string
		var existingEnabled bool
		err := s.DB.QueryRowContext(ctx, `SELECT protocol,transport,security,enabled FROM panel_inbound_inventory WHERE panel_id=$1 AND port=$2 AND present=true ORDER BY remote_id LIMIT 1`, p.ID, port).Scan(&existingProtocol, &existingTransport, &existingSecurity, &existingEnabled)
		if err == nil {
			if existingEnabled && existingProtocol == "vless" && existingTransport == "tcp" && existingSecurity == "reality" {
				continue
			}
			return fmt.Errorf("port %d occupied by incompatible inbound", port)
		}
		if err != sql.ErrNoRows {
			return err
		}
		managed := fmt.Sprintf("dob:global-reality:%05d", port)
		if _, e = reg.Ensure(ctx, acc, p.ID, managed); e != nil {
			return e
		}
		d := desired.Service{DB: s.DB, Secrets: s.Secrets, SSH: s.SSH, ManagedKey: managed, Port: port, MutationPanels: map[string]bool{p.ID: true}}
		if e = d.ReconcilePanel(ctx, p, false); e != nil {
			return fmt.Errorf("port %d: %w", port, e)
		}
	}
	return nil
}
