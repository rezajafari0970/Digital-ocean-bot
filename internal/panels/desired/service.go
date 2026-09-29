package desired

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	createflow "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/create"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/inventory"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/realitycontract"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/postflight"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	updateflow "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/update"
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

type updateDeps struct{ *panelDeps }

func (d updateDeps) Build(ctx context.Context, _ updateflow.Request) (realityconfig.Payload, error) {
	return d.panelDeps.Build(ctx, createflow.Request{})
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

	// Optional shared authenticated runtime for fast API-only reconciliation.
	Runtime *sanaei.PanelRuntime
}

func operationalLifecycle(state string) bool {
	switch state {
	case "READY", "EXPIRING", "RETIRING":
		return true
	default:
		return false
	}
}

type panelDeps struct {
	db *sql.DB

	secrets Secrets

	ssh provisioning.SSHClient

	exec sanaei.SessionExecutor

	runtime *sanaei.PanelRuntime

	target provisioning.Target

	sshKey []byte

	panelID string

	accountID string

	managedKey string

	port int

	targetHost string

	sni string

	serverNames []string

	uuidRef string

	privateRef string

	shortID string

	publicKey string

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

	return operationalLifecycle(state) &&
			enabled,
		nil
}

func (d *panelDeps) RefreshInventory(
	ctx context.Context,
) ([]inventory.InboundRecord, error) {

	var snapshot inventory.Snapshot
	var err error
	if d.runtime != nil && d.runtime.Session != nil {
		var raws []json.RawMessage
		raws, err = d.runtime.Session.Snapshot(ctx)
		if err == nil {
			snapshot, err = sanaei.InventoryFromRaw(d.panelID, raws)
		}
	} else {
		snapshot, err = sanaei.ReadInventory(ctx, d.exec, d.panelID)
	}

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

func (d *panelDeps) OccupiedPorts(ctx context.Context) ([]int, error) {
	records, err := d.RefreshInventory(ctx)
	if err != nil {
		return nil, err
	}
	ports := make([]int, 0, len(records))
	seen := make(map[int]bool, len(records))
	for _, r := range records {
		if r.Port > 0 && !seen[r.Port] {
			seen[r.Port] = true
			ports = append(ports, r.Port)
		}
	}
	return ports, nil
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

	managed := realitycontract.Managed{
		Remark:      d.managedKey,
		Port:        d.port,
		UUID:        string(uuid),
		Email:       "managed-" + strconv.Itoa(d.port),
		Target:      d.targetHost,
		TargetPort:  d.targetPort,
		SNI:         d.sni,
		ServerNames: d.serverNames,
		PrivateKey:  string(privateKey),
		ShortID:     d.shortID,
	}
	return managed.Payload()
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
	if err == nil && d.runtime != nil && d.runtime.Session != nil {
		d.runtime.Session.Invalidate()
	}

	return err
}

func (d *panelDeps) Update(ctx context.Context, remoteID int64, payload realityconfig.Payload) error {
	_, err := sanaei.UpdateInbound(ctx, d.exec, remoteID, payload)
	if err == nil && d.runtime != nil && d.runtime.Session != nil {
		d.runtime.Session.Invalidate()
	}
	return err
}

func (d *panelDeps) Verify(
	ctx context.Context,
	remoteID int64,
	payload realityconfig.Payload,
) error {
	var inbound postflight.Inbound
	var err error
	if d.runtime != nil && d.runtime.Session != nil {
		var raw json.RawMessage
		var ok bool
		raw, ok, err = d.runtime.Session.RawInbound(ctx, remoteID)
		if err == nil && !ok {
			// Cached snapshot may be stale after a concurrent panel mutation.
			d.runtime.Session.Invalidate()
			raw, ok, err = d.runtime.Session.RawInbound(ctx, remoteID)
		}
		if err == nil && ok {
			err = json.Unmarshal(raw, &inbound)
		}
		if err == nil && !ok {
			// Last-resort authoritative lookup. ResilientSession handles
			// transient network/5xx/session failures.
			inbound, err = sanaei.GetInbound(ctx, d.runtime.Session.Exec, remoteID)
		}
	} else {
		inbound, err = sanaei.GetInbound(ctx, d.exec, remoteID)
	}
	if err != nil {
		return err
	}

	if err := postflight.ValidatePayload(inbound, remoteID, payload); err != nil {
		return err
	}
	if d.publicKey == "" {
		return errors.New("reality public key unavailable")
	}
	_, err = d.db.ExecContext(ctx, `INSERT INTO inbound_export_metadata(panel_id,remote_id,public_key) VALUES($1,$2,$3) ON CONFLICT(panel_id,remote_id) DO UPDATE SET public_key=excluded.public_key,updated_at=now()`, d.panelID, remoteID, d.publicKey)
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

		publicKey string

		panelPort int

		targetPort int

		serverNamesRaw []byte
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
rs.server_names,

rc.uuid_secret_ref,
rc.private_key_secret_ref,
rc.short_id,
rc.public_key

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
		&serverNamesRaw,
		&uuidRef,
		&privateRef,
		&shortID,
		&publicKey,
	)

	if err != nil {
		return err
	}

	var serverNames []string
	if len(serverNamesRaw) > 0 {
		if err := json.Unmarshal(serverNamesRaw, &serverNames); err != nil {
			return err
		}
	}
	if len(serverNames) == 0 {
		serverNames = []string{sni}
	}

	if !operationalLifecycle(lifecycleState) {
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

		_, err = (realitycontract.Managed{
			Remark:      s.ManagedKey,
			Port:        s.Port,
			UUID:        string(uuid),
			Email:       "managed-" + strconv.Itoa(s.Port),
			Target:      targetHost,
			TargetPort:  targetPort,
			SNI:         sni,
			ServerNames: serverNames,
			PrivateKey:  string(privateKey),
			ShortID:     shortID,
		}).Payload()
		return err
	}

	// Fail closed unless this exact panel has been
	// explicitly enabled for mutation.
	if !s.MutationPanels[panel.ID] {
		return ErrMutationDisabled
	}

	target :=
		provisioning.Target{
			AccountID: accountID,

			DropletID: dropletID,

			Host: host,

			User: sshUser,

			KeySecretRef: sshKeyRef,
		}

	var executor sanaei.SessionExecutor
	if s.Runtime != nil && s.Runtime.Session != nil {
		executor = s.Runtime.Session.Exec
	} else {
		panelPassword, err := s.Secrets.Get(ctx, accountID, panelPasswordRef)
		if err != nil {
			return err
		}
		baseURL := fmt.Sprintf("http://%s:%d%s", host, panelPort, panelPath)
		apiClient, err := sanaei.NewAPIClient(baseURL, sanaei.Credentials{Username: panelUser, Password: string(panelPassword)}, nil)
		credentials.Wipe(panelPassword)
		if err != nil {
			return err
		}
		if err = apiClient.Login(ctx); err != nil {
			return err
		}
		executor = (&sanaei.ResilientSession{Client: apiClient, Policy: sanaei.DefaultRetryPolicy()})
	}

	dependencies :=
		&panelDeps{
			db: s.DB,

			secrets: s.Secrets,

			ssh: s.SSH,

			exec: executor,

			runtime: s.Runtime,

			target: target,

			panelID: panel.ID,

			accountID: accountID,

			managedKey: s.ManagedKey,

			port: s.Port,

			targetHost: targetHost,

			sni: sni,

			serverNames: serverNames,

			uuidRef: uuidRef,

			privateRef: privateRef,

			shortID: shortID,

			publicKey: publicKey,

			targetPort: targetPort,
		}

	run := func(runCtx context.Context) error {
		records, err := dependencies.RefreshInventory(runCtx)
		if err != nil {
			return err
		}
		owned := make([]inventory.InboundRecord, 0, 1)
		for _, rec := range records {
			if rec.Remark == s.ManagedKey {
				owned = append(owned, rec)
			}
		}
		if len(owned) == 1 && owned[0].RemoteID > 0 && owned[0].Port == s.Port {
			payload, err := dependencies.Build(runCtx, createflow.Request{ManagedKey: s.ManagedKey, Port: s.Port})
			if err != nil {
				return err
			}
			if err = dependencies.Verify(runCtx, owned[0].RemoteID, payload); err == nil {
				return nil
			}
			_, err = updateflow.Run(runCtx, updateDeps{dependencies}, updateflow.Request{ManagedKey: s.ManagedKey, Port: s.Port})
			return err
		}
		_, err = createflow.Run(runCtx, dependencies, createflow.Request{ManagedKey: s.ManagedKey, Port: s.Port})
		return err
	}
	if s.Runtime != nil {
		return s.Runtime.WithMutation(ctx, run)
	}
	return run(ctx)

}
