package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

var ErrPostInstallCapabilityMismatch = errors.New("installer lacks required post-install capabilities")

func (c Container) RequirePostInstallCapabilities(ctx context.Context, deploymentID string) error {
	var raw []byte
	err := c.DB.QueryRowContext(ctx, `SELECT ir.manifest_snapshot FROM installer_runs ir JOIN deployments d ON d.id=ir.deployment_id WHERE ir.deployment_id=$1 AND ir.generation=d.installer_generation AND ir.state='INSTALL_COMPLETE'`, deploymentID).Scan(&raw)
	if err != nil {
		return err
	}
	var m provisioning.InstallerManifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return err
	}
	have := map[string]bool{}
	for _, x := range m.Capabilities {
		have[x] = true
	}
	if !have["xui_database"] || !have["xui_panel"] {
		return ErrPostInstallCapabilityMismatch
	}
	return nil
}
