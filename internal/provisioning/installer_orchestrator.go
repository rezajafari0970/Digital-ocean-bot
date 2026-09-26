package provisioning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrInstallerRunConflict = errors.New("installer run manifest conflict")

type InstallerRun struct {
	ID, DeploymentID, ProvisionRunID, InstallerID, ManifestSHA256, State, LastError string
	Manifest                                                                        InstallerManifest
	Generation                                                                      int
}

type InstallerOrchestrator struct {
	DB       *sql.DB
	Registry InstallerRegistry
}

func (o InstallerOrchestrator) Prepare(ctx context.Context, deploymentID, provisionRunID string, generation int, ref InstallerRef, ready ReadinessSnapshot) (ResolvedInstaller, InstallerRun, error) {
	resolved, err := o.Registry.Resolve(ctx, ref, ready)
	if err != nil {
		return resolved, InstallerRun{}, err
	}
	raw, _ := json.Marshal(resolved.Manifest)
	if generation < 1 {
		return resolved, InstallerRun{}, ErrInvalidPlan
	}
	run := InstallerRun{DeploymentID: deploymentID, ProvisionRunID: provisionRunID, InstallerID: resolved.RegistryID, ManifestSHA256: resolved.SHA256, Manifest: resolved.Manifest, State: "PREPARING", Generation: generation}
	err = o.DB.QueryRowContext(ctx, `INSERT INTO installer_runs(deployment_id,provision_run_id,installer_id,manifest_snapshot,manifest_sha256,state,generation) VALUES($1,$2,$3,$4,$5,'PREPARING',$6) ON CONFLICT(deployment_id,generation) DO NOTHING RETURNING id::text`, deploymentID, provisionRunID, resolved.RegistryID, raw, resolved.SHA256, generation).Scan(&run.ID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingHash string
		err = o.DB.QueryRowContext(ctx, `SELECT id::text,manifest_sha256,state,last_error FROM installer_runs WHERE deployment_id=$1 AND generation=$2`, deploymentID, generation).Scan(&run.ID, &existingHash, &run.State, &run.LastError)
		if err != nil {
			return resolved, run, err
		}
		if existingHash != resolved.SHA256 {
			return resolved, run, ErrInstallerRunConflict
		}
		run.ManifestSHA256 = existingHash
		return resolved, run, nil
	}
	return resolved, run, err
}
func (o InstallerOrchestrator) SetState(ctx context.Context, runID, state, lastError string) error {
	switch state {
	case "PREPARING", "INSTALLING", "VERIFYING", "INSTALL_COMPLETE", "ROLLBACK_REQUIRED", "ROLLED_BACK", "FAILED":
	default:
		return ErrInvalidPlan
	}
	_, err := o.DB.ExecContext(ctx, `UPDATE installer_runs SET state=$2,last_error=$3,updated_at=now() WHERE id=$1`, runID, state, lastError)
	return err
}
