package app

import (
	"context"
	"testing"
)

func TestProvisionHandoffUsesCurrentInstallerGeneration(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	c := Container{DB: db}
	check := func(want bool) {
		t.Helper()
		cfg, _, err := c.DeploymentConfigFromSnapshot(context.Background(), f.d.ID)
		if err != nil || cfg.Provision.InstallerHandoff != want {
			t.Fatal(cfg.Provision.InstallerHandoff, err)
		}
	}
	check(true)
	execBootstrap(t, db, `UPDATE deployments SET installer_generation=2 WHERE id=$1`, f.d.ID)
	check(false)
	execBootstrap(t, db, `INSERT INTO deployment_installer_selections(deployment_id,generation,installer_name,installer_version) VALUES($1,2,'selected-generation',1)`, f.d.ID)
	check(true)
}
