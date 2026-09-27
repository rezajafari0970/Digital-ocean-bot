package desired

import (
	"context"
	"database/sql"
	"errors"

	createflow "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/create"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/listeners"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/reality/credentials"
)

var ErrMutationDisabled = errors.New("desired mutation disabled")

type Secrets interface {
	Get(
		context.Context,
		string,
		string,
	) ([]byte, error)
}

type Service struct {
	DB *sql.DB

	Secrets Secrets

	SSH provisioning.SSHClient

	ManagedKey string

	Port int

	// Explicit production allowlist.
	//
	// DryRun=false is still unable to mutate a panel
	// unless its exact panel ID exists here with true.
	MutationPanels map[string]bool
}

type panelDeps struct {
	db *sql.DB

	secrets Secrets

	ssh provisioning.SSHClient

	exec sanaei.SessionExecutor

	target provisioning.Target

	sshKey []byte

	panelID string

	accountID string

	managedKey string

	port int

	targetHost string

	sni string

	uuidRef string

	privateRef string

	shortID string

	targetPort int
}

func (d *panelDeps) Ready(
	ctx context.Context,
) (bool, error) {

	var (
		state   string
		enabled bool
	)

	err := d.db.QueryRowContext(
		ctx,
		`
SELECT
r.state,
pi.enabled
FROM panel_instances pi
JOIN droplets r
  ON r.id=pi.droplet_id
WHERE pi.id=$1
`,
		d.panelID,
	).Scan(
		&state,
		&enabled,
	)

	if err != nil {
		return false, err
	}

	return state == "READY" &&
			enabled,
		nil
}

func (d *panelDeps) RefreshInventory(
	ctx context.Context,
) ([]inventory.InboundRecord, error) {

	snapshot, err :=
		sanaei.ReadInventory(
			ctx,
			d.exec,
			d.panelID,
		)

	if err != nil {
		return nil, err
	}

	_, err =
		(inventory.SQLStore{
			DB: d.db,
		}).Sync(
			ctx,
			snapshot,
		)

	if err != nil {
		return nil, err
	}

	return snapshot.Records, nil
}

func (d *panelDeps) OccupiedPorts(
	ctx context.Context,
) ([]int, error) {

	run := func(
		ctx context.Context,
		command string,
	) (string, error) {

		return d.ssh.Run(
			ctx,
			d.target,
			d.sshKey,
			command,
		)
	}

	return (listeners.SSHCollector{
		Run: run,
	}).Ports(
		ctx,
	)
}

func (d *panelDeps) Build(
	ctx context.Context,
	_ createflow.Request,
) (realityconfig.Payload, error) {

	uuid, err :=
		d.secrets.Get(
			ctx,
			d.accountID,
			d.uuidRef,
		)

	if err != nil {
		return realityconfig.Payload{}, err
	}

	defer credentials.Wipe(
		uuid,
	)

	privateKey, err :=
		d.secrets.Get(
			ctx,
			d.accountID,
			d.privateRef,
		)

	if err != nil {
		return realityconfig.Payload{}, err
	}

	defer credentials.Wipe(
		privateKey,
	)

	return realityconfig.Build(
		realityconfig.Input{
			Remark: d.managedKey,

			Port: d.port,

			UUID: string(uuid),

			Email: "managed",

			Target: d.targetHost,

			TargetPort: d.targetPort,

			ServerName: d.sni,

			PrivateKey: string(privateKey),

			ShortID: d.shortID,
		},
	)
}

func (d *panelDeps) Add(
	ctx context.Context,
	payload realityconfig.Payload,
) error {

	_, err :=
		sanaei.AddInbound(
			ctx,
			d.exec,
			payload,
		)

	return err
}

func (s Service) ReconcilePanel(
	ctx context.Context,
	panel readyworker.Panel,
	dryRun bool,
) error {

	if s.DB == nil ||
		s.Secrets == nil ||
		panel.ID == "" ||
		s.ManagedKey == "" ||
		s.Port < 1 ||
		s.Port > 65535 {

		return errors.New(
			"desired config",
		)
	}

	var (
		accountID string

		dropletID string

		host string

		sshUser string

		sshKeyRef string

		panelUser string

		panelPasswordRef string

		panelPath string

		lifecycleState string

		targetHost string

		sni string

		uuidRef string

		privateRef string

		shortID string

		panelPort int

		targetPort int
	)

	err := s.DB.QueryRowContext(
		ctx,
		`
SELECT
pi.account_id::text,
pi.droplet_id::text,

d.host,

d.profile_snapshot->>'ssh_user',
d.profile_snapshot->>'ssh_key_secret_ref',

x.username,
x.password_secret_ref,
x.port,
x.web_path,

r.state,

rs.target,
rs.server_name,
rs.port,

rc.uuid_secret_ref,
rc.private_key_secret_ref,
rc.short_id

FROM panel_instances pi

JOIN deployments d
  ON d.droplet_id=pi.droplet_id

JOIN droplets r
  ON r.id=pi.droplet_id

JOIN xui_panel_deployments x
  ON x.droplet_id=pi.droplet_id
 AND x.generation=d.postinstall_generation

JOIN reality_target_selections rs
  ON rs.panel_id=pi.id

JOIN reality_credentials rc
  ON rc.panel_id=pi.id
 AND rc.managed_key=$2

WHERE pi.id=$1
  AND pi.enabled=true
`,
		panel.ID,
		s.ManagedKey,
	).Scan(
		&accountID,
		&dropletID,
		&host,
		&sshUser,
		&sshKeyRef,
		&panelUser,
		&panelPasswordRef,
		&panelPort,
		&panelPath,
		&lifecycleState,
		&targetHost,
		&sni,
		&targetPort,
		&uuidRef,
		&privateRef,
		&shortID,
	)

	if err != nil {
		return err
	}

	if lifecycleState != "READY" {
		return createflow.ErrNotReady
	}

	// Dry-run deliberately never creates an SSH mutation
	// executor and never calls AddInbound.
	if dryRun {

		uuid, err :=
			s.Secrets.Get(
				ctx,
				accountID,
				uuidRef,
			)

		if err != nil {
			return err
		}

		defer credentials.Wipe(
			uuid,
		)

		privateKey, err :=
			s.Secrets.Get(
				ctx,
				accountID,
				privateRef,
			)

		if err != nil {
			return err
		}

		defer credentials.Wipe(
			privateKey,
		)

		_, err =
			realityconfig.Build(
				realityconfig.Input{
					Remark: s.ManagedKey,

					Port: s.Port,

					UUID: string(uuid),

					Email: "managed",

					Target: targetHost,

					TargetPort: targetPort,

					ServerName: sni,

					PrivateKey: string(privateKey),

					ShortID: shortID,
				},
			)

		return err
	}

	// Fail closed unless this exact panel has been
	// explicitly enabled for mutation.
	if !s.MutationPanels[panel.ID] {
		return ErrMutationDisabled
	}

	sshKey, err :=
		s.Secrets.Get(
			ctx,
			accountID,
			sshKeyRef,
		)

	if err != nil {
		return err
	}

	defer credentials.Wipe(
		sshKey,
	)

	target :=
		provisioning.Target{
			AccountID: accountID,

			DropletID: dropletID,

			Host: host,

			User: sshUser,

			KeySecretRef: sshKeyRef,
		}

	executor :=
		sanaei.SSHSessionExecutor{
			SSH: s.SSH,

			Target: target,

			PrivateKeySecretRef: sshKeyRef,

			PanelPasswordSecretRef: panelPasswordRef,

			AccountID: accountID,

			Username: panelUser,

			Port: panelPort,

			BasePath: panelPath,

			DialHost: "127.0.0.1",

			Secrets: s.Secrets,
		}

	dependencies :=
		&panelDeps{
			db: s.DB,

			secrets: s.Secrets,

			ssh: s.SSH,

			exec: executor,

			target: target,

			sshKey: sshKey,

			panelID: panel.ID,

			accountID: accountID,

			managedKey: s.ManagedKey,

			port: s.Port,

			targetHost: targetHost,

			sni: sni,

			uuidRef: uuidRef,

			privateRef: privateRef,

			shortID: shortID,

			targetPort: targetPort,
		}

	_, err =
		createflow.Run(
			ctx,
			dependencies,

			createflow.Request{
				ManagedKey: s.ManagedKey,

				Port: s.Port,
			},
		)

	return err
}
