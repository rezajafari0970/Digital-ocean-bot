ALTER TABLE deployments ADD COLUMN IF NOT EXISTS installer_generation integer NOT NULL DEFAULT 1 CHECK(installer_generation>0);
ALTER TABLE installer_runs ADD COLUMN IF NOT EXISTS generation integer NOT NULL DEFAULT 1 CHECK(generation>0);
ALTER TABLE deployment_installer_selections ADD COLUMN IF NOT EXISTS generation integer NOT NULL DEFAULT 1 CHECK(generation>0);

ALTER TABLE installer_runs DROP CONSTRAINT IF EXISTS installer_runs_deployment_id_key;
ALTER TABLE installer_runs ADD CONSTRAINT installer_runs_deployment_generation_key UNIQUE(deployment_id,generation);

ALTER TABLE deployment_installer_selections DROP CONSTRAINT IF EXISTS deployment_installer_selections_pkey;
ALTER TABLE deployment_installer_selections ADD PRIMARY KEY(deployment_id,generation);

CREATE INDEX IF NOT EXISTS installer_runs_deployment_generation_idx ON installer_runs(deployment_id,generation DESC);
CREATE INDEX IF NOT EXISTS installer_selections_deployment_generation_idx ON deployment_installer_selections(deployment_id,generation DESC);
