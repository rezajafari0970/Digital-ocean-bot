CREATE TABLE IF NOT EXISTS deployment_installer_selections (
 deployment_id uuid PRIMARY KEY REFERENCES deployments(id) ON DELETE CASCADE,
 installer_name text NOT NULL,
 installer_version integer NOT NULL CHECK(installer_version>0),
 source text NOT NULL DEFAULT 'operator',
 selected_at timestamptz NOT NULL DEFAULT now()
);
