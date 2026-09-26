DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM installer_runs WHERE generation>1) OR EXISTS(SELECT 1 FROM deployment_installer_selections WHERE generation>1) THEN
  RAISE EXCEPTION 'cannot downgrade installer generations while generation > 1 history exists';
 END IF;
END $$;
DROP INDEX IF EXISTS installer_selections_deployment_generation_idx;
DROP INDEX IF EXISTS installer_runs_deployment_generation_idx;
ALTER TABLE deployment_installer_selections DROP CONSTRAINT IF EXISTS deployment_installer_selections_pkey;
ALTER TABLE deployment_installer_selections ADD PRIMARY KEY(deployment_id);
ALTER TABLE installer_runs DROP CONSTRAINT IF EXISTS installer_runs_deployment_generation_key;
ALTER TABLE installer_runs ADD CONSTRAINT installer_runs_deployment_id_key UNIQUE(deployment_id);
ALTER TABLE deployment_installer_selections DROP COLUMN IF EXISTS generation;
ALTER TABLE installer_runs DROP COLUMN IF EXISTS generation;
ALTER TABLE deployments DROP COLUMN IF EXISTS installer_generation;
