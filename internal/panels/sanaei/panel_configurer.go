package sanaei

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

type SecretWriterReader interface {
	Put(context.Context, string, string, string, []byte) error
	Get(context.Context, string, string) ([]byte, error)
}
type PanelConfigurer struct {
	DB       *sql.DB
	Secrets  SecretWriterReader
	Runner   Runner
	Uploader Uploader
}

func NewPanelPassword() ([]byte, error) {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	out := make([]byte, base64.RawURLEncoding.EncodedLen(len(b)))
	base64.RawURLEncoding.Encode(out, b)
	return out, nil
}
func panelDefaults(id string) (string, int, string, string) {
	s := strings.ReplaceAll(id, "-", "")
	if len(s) > 12 {
		s = s[:12]
	}
	return "dob_" + s, 2053, "/" + s + "/", "xui-panel-" + id
}

func (p PanelConfigurer) Configure(ctx context.Context, accountID, dropletID string, target provisioning.Target) error {
	if p.DB == nil || p.Secrets == nil || p.Runner == nil || p.Uploader == nil || accountID == "" || dropletID == "" || target.Host == "" {
		return errors.New("panel config missing")
	}
	user, port, path, ref := panelDefaults(dropletID)
	var state string
	var generation int
	if err := p.DB.QueryRowContext(ctx, `SELECT postinstall_generation FROM deployments WHERE account_id=$1 AND droplet_id=$2 ORDER BY created_at DESC LIMIT 1`, accountID, dropletID).Scan(&generation); err != nil {
		return err
	}
	err := p.DB.QueryRowContext(ctx, `SELECT username,password_secret_ref,port,web_path,state FROM xui_panel_deployments WHERE droplet_id=$1 AND generation=$2`, dropletID, generation).Scan(&user, &ref, &port, &path, &state)
	if errors.Is(err, sql.ErrNoRows) {
		pass, e := NewPanelPassword()
		if e != nil {
			return e
		}
		defer wipe(pass)
		if e = p.Secrets.Put(ctx, accountID, ref, "xui_panel_password", pass); e != nil {
			return e
		}
		err = p.DB.QueryRowContext(ctx, `INSERT INTO xui_panel_deployments(id,account_id,droplet_id,username,password_secret_ref,port,web_path,state,generation) VALUES(gen_random_uuid(),$1,$2,$3,$4,$5,$6,'CONFIGURING',$7) RETURNING state`, accountID, dropletID, user, ref, port, path, generation).Scan(&state)
	}
	if err != nil {
		return err
	}
	if state == "COMPLETED" {
		return nil
	}
	pass, err := p.Secrets.Get(ctx, accountID, ref)
	if err != nil {
		return err
	}
	defer wipe(pass)
	key, err := p.Secrets.Get(ctx, accountID, target.KeySecretRef)
	if err != nil {
		return err
	}
	defer wipe(key)
	payload := []byte("XUI_USER=" + shellQuote(user) + "\nXUI_PASS=" + shellQuote(string(pass)) + "\nXUI_PORT=" + fmt.Sprint(port) + "\nXUI_PATH=" + shellQuote(path) + "\n")
	local, err := os.CreateTemp("", "dob-panel-*")
	if err != nil {
		return err
	}
	localPath := local.Name()
	defer os.Remove(localPath)
	if err = os.Chmod(localPath, 0600); err != nil {
		local.Close()
		return err
	}
	if _, err = local.Write(payload); err != nil {
		local.Close()
		return err
	}
	if err = local.Close(); err != nil {
		return err
	}
	remote := "/root/.dob-xui-panel-" + dropletID + ".env"
	// A previous process may have crashed after upload but before command execution.
	// Remove that deterministic root-only staging file before replacing it.
	_, _ = p.Runner.Run(ctx, target, key, "rm -f -- "+shellQuote(remote))
	if err = p.Uploader.Upload(ctx, target, key, localPath, remote, 0600); err != nil {
		return err
	}
	cmd := panelConfigureCommand(remote)
	if _, err = p.Runner.Run(ctx, target, key, cmd); err != nil {
		_, _ = p.DB.ExecContext(ctx, `UPDATE xui_panel_deployments SET state='FAILED',updated_at=now() WHERE droplet_id=$1 AND generation=$2`, dropletID, generation)
		return err
	}
	_, err = p.DB.ExecContext(ctx, `UPDATE xui_panel_deployments SET state='COMPLETED',updated_at=now() WHERE droplet_id=$1 AND generation=$2`, dropletID, generation)
	return err
}

func panelConfigureCommand(remote string) string {
	return fmt.Sprintf("set -euo pipefail; f=%s; trap 'rm -f -- \"$f\"' EXIT; . \"$f\"; /usr/local/x-ui/x-ui setting -username \"$XUI_USER\" -password \"$XUI_PASS\" -port \"$XUI_PORT\" -webBasePath \"$XUI_PATH\" >/dev/null; systemctl restart x-ui; systemctl is-active x-ui >/dev/null", shellQuote(remote))
}

func (p PanelConfigurer) RepairCompleted(
	ctx context.Context,
	accountID string,
	dropletID string,
	target provisioning.Target,
) error {

	if p.DB == nil ||
		p.Secrets == nil ||
		p.Runner == nil ||
		p.Uploader == nil ||
		accountID == "" ||
		dropletID == "" ||
		target.Host == "" {

		return errors.New(
			"panel repair missing",
		)
	}

	var (
		generation int

		username  string
		secretRef string
		port      int
		webPath   string
		state     string
	)

	err := p.DB.QueryRowContext(
		ctx,
		`
SELECT
d.postinstall_generation,
p.username,
p.password_secret_ref,
p.port,
p.web_path,
p.state
FROM deployments d
JOIN xui_panel_deployments p
  ON p.droplet_id=d.droplet_id
 AND p.generation=d.postinstall_generation
WHERE d.account_id=$1
  AND d.droplet_id=$2
ORDER BY d.created_at DESC
LIMIT 1
`,
		accountID,
		dropletID,
	).Scan(
		&generation,
		&username,
		&secretRef,
		&port,
		&webPath,
		&state,
	)

	if err != nil {
		return err
	}

	_ = generation

	if state != "COMPLETED" {
		return errors.New(
			"panel repair requires completed ledger",
		)
	}

	password, err := p.Secrets.Get(
		ctx,
		accountID,
		secretRef,
	)

	if err != nil {
		return err
	}

	defer wipe(password)

	privateKey, err := p.Secrets.Get(
		ctx,
		accountID,
		target.KeySecretRef,
	)

	if err != nil {
		return err
	}

	defer wipe(privateKey)

	payload := []byte(
		"XUI_USER=" +
			shellQuote(username) +
			"\n" +

			"XUI_PASS=" +
			shellQuote(
				string(password),
			) +
			"\n" +

			"XUI_PORT=" +
			fmt.Sprint(port) +
			"\n" +

			"XUI_PATH=" +
			shellQuote(webPath) +
			"\n",
	)

	local, err := os.CreateTemp(
		"",
		"dob-panel-repair-*",
	)

	if err != nil {
		return err
	}

	localPath := local.Name()

	defer os.Remove(
		localPath,
	)

	if err = os.Chmod(
		localPath,
		0600,
	); err != nil {

		local.Close()
		return err
	}

	if _, err = local.Write(
		payload,
	); err != nil {

		local.Close()
		return err
	}

	if err = local.Close(); err != nil {
		return err
	}

	remote :=
		"/root/.dob-xui-panel-repair-" +
			dropletID +
			".env"

	// Clean up a stale staging file left by
	// an interrupted previous repair.
	_, _ = p.Runner.Run(
		ctx,
		target,
		privateKey,
		"rm -f -- "+shellQuote(remote),
	)

	if err = p.Uploader.Upload(
		ctx,
		target,
		privateKey,
		localPath,
		remote,
		0600,
	); err != nil {
		return err
	}

	_, err = p.Runner.Run(
		ctx,
		target,
		privateKey,
		panelConfigureCommand(
			remote,
		),
	)

	return err
}
