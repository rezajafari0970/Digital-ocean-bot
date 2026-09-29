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
			return e
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
