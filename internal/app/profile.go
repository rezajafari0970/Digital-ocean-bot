package app

import (
	"context"
	"database/sql"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/droplets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
)

func (c Container) DeploymentConfigFromSnapshot(ctx context.Context, deploymentID string) (DeploymentConfig, workflow.ProfileSnapshot, error) {
	store := workflow.ProfileStore{DB: c.DB}
	snap, err := store.SnapshotForDeployment(ctx, deploymentID)
	if err != nil {
		return DeploymentConfig{}, snap, err
	}
	var template sanaei.DatabaseTemplate
	if snap.DatabaseTemplateID != "" {
		err = c.DB.QueryRowContext(ctx, `SELECT id::text,name,version,storage_path,sha256,size_bytes FROM xui_database_templates WHERE id=$1 AND active=true`, snap.DatabaseTemplateID).Scan(&template.ID, &template.Name, &template.Version, &template.Path, &template.SHA256, &template.Size)
		if err != nil && err != sql.ErrNoRows {
			return DeploymentConfig{}, snap, err
		}
	}
	cfg := DeploymentConfig{Profile: droplets.Profile{Name: snap.Name, Region: snap.Region, Regions: snap.Regions, Size: snap.Size, Image: snap.Image, Lifetime: snap.Lifetime, Provision: "sanaei", SSHKeyID: snap.SSHProviderKeyID}, Provision: provisioning.Plan{Bootstrap: "set -euo pipefail; apt-get update -y", InstallPanel: "true", Verify: sanaei.VerifyCommand()}, Target: provisioning.Target{User: snap.SSHUser, KeySecretRef: snap.SSHKeySecretRef}, Template: template, DatabasePaths: sanaei.DefaultDatabasePaths()}
	return cfg, snap, nil
}
