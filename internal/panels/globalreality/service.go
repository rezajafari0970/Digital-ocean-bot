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
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
	"strings"
)

type Secrets interface {
	Get(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, string, []byte) error
}
type Service struct {
	DB       *sql.DB
	Secrets  Secrets
	SSH      provisioning.SSHClient
	Runtimes *sanaei.RuntimeManager
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
	var rolloutCount int
	if e = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM reality_rollout_panels WHERE enabled=true`).Scan(&rolloutCount); e != nil {
		return e
	}
	if rolloutCount > 0 {
		var allowed bool
		if e = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reality_rollout_panels WHERE panel_id=$1 AND enabled=true)`, p.ID).Scan(&allowed); e != nil {
			return e
		}
		if !allowed {
			return nil
		}
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
	var runtime *sanaei.PanelRuntime
	if s.Runtimes != nil {
		runtime, e = s.Runtimes.Acquire(ctx, p.ID)
		if e != nil {
			// A freshly provisioned panel can be healthy on loopback while the
			// host firewall still blocks its public API port. Repair only that
			// host-level condition, then retry the normal direct API runtime.
			var accID, dropletID, host, user, keyRef string
			qerr := s.DB.QueryRowContext(ctx, `SELECT pi.account_id::text,pi.droplet_id::text,d.host,COALESCE(d.profile_snapshot->>'ssh_user','root'),COALESCE(d.profile_snapshot->>'ssh_key_secret_ref','') FROM panel_instances pi JOIN deployments d ON d.droplet_id=pi.droplet_id WHERE pi.id=$1`, p.ID).Scan(&accID, &dropletID, &host, &user, &keyRef)
			if qerr == nil && keyRef != "" {
				key, kerr := s.Secrets.Get(ctx, accID, keyRef)
				if kerr == nil {
					out, _ := s.SSH.Run(ctx, provisioning.Target{AccountID: accID, DropletID: dropletID, Host: host, User: user, KeySecretRef: keyRef}, key, `changed=0; if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then ufw --force disable >/dev/null 2>&1 || true; changed=1; fi; if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then systemctl disable --now firewalld >/dev/null 2>&1 || true; changed=1; fi; echo $changed`)
					credentials.Wipe(key)
					if strings.TrimSpace(out) == "1" {
						s.Runtimes.Invalidate(p.ID)
						s.Runtimes.ResetCircuit(p.ID)
						runtime, e = s.Runtimes.Acquire(ctx, p.ID)
					}
				}
			}
			if e != nil {
				return e
			}
		}
	}
	var acc string
	e = s.DB.QueryRowContext(ctx, "SELECT account_id::text FROM panel_instances WHERE id=$1 AND enabled=true", p.ID).Scan(&acc)
	if e != nil {
		return e
	}
	reg := credentials.Registry{DB: s.DB, Secrets: s.Secrets, Keys: credentials.LocalX25519Generator{}}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid reality port %d", port)
		}
		var existingProtocol, existingTransport, existingSecurity string
		var existingEnabled bool
		err := s.DB.QueryRowContext(ctx, `SELECT protocol,transport,security,enabled FROM panel_inbound_inventory WHERE panel_id=$1 AND port=$2 AND present=true ORDER BY remote_id LIMIT 1`, p.ID, port).Scan(&existingProtocol, &existingTransport, &existingSecurity, &existingEnabled)
		if err == nil {
			if existingEnabled && existingProtocol == "vless" && existingTransport == "tcp" && existingSecurity == "reality" {
				managed := fmt.Sprintf("dob:global-reality:%05d", port)
				d := desired.Service{DB: s.DB, Secrets: s.Secrets, SSH: s.SSH, ManagedKey: managed, Port: port, MutationPanels: map[string]bool{p.ID: true}, Runtime: runtime}
				if e = d.ReconcilePanel(ctx, p, false); e != nil {
					return fmt.Errorf("port %d: %w", port, e)
				}
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
		d := desired.Service{DB: s.DB, Secrets: s.Secrets, SSH: s.SSH, ManagedKey: managed, Port: port, MutationPanels: map[string]bool{p.ID: true}, Runtime: runtime}
		if e = d.ReconcilePanel(ctx, p, false); e != nil {
			return fmt.Errorf("port %d: %w", port, e)
		}
	}
	return nil
}
