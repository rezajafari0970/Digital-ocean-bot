CREATE TABLE IF NOT EXISTS installers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL,
 version integer NOT NULL CHECK(version>0),
 manifest jsonb NOT NULL,
 sha256 text NOT NULL CHECK(length(sha256)=64),
 active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(name,version),
 UNIQUE(sha256)
);
CREATE OR REPLACE FUNCTION protect_installer_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.name<>OLD.name OR NEW.version<>OLD.version OR NEW.manifest<>OLD.manifest OR NEW.sha256<>OLD.sha256 THEN
  RAISE EXCEPTION 'installer versions are immutable; create a new version';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS installers_immutable ON installers;
CREATE TRIGGER installers_immutable BEFORE UPDATE ON installers FOR EACH ROW EXECUTE FUNCTION protect_installer_version();

CREATE TABLE IF NOT EXISTS installer_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
 provision_run_id uuid NOT NULL REFERENCES provision_runs(id) ON DELETE CASCADE,
 installer_id uuid NOT NULL REFERENCES installers(id),
 manifest_snapshot jsonb NOT NULL,
 manifest_sha256 text NOT NULL,
 state text NOT NULL CHECK(state IN ('PREPARING','INSTALLING','VERIFYING','INSTALL_COMPLETE','ROLLBACK_REQUIRED','ROLLED_BACK','FAILED')),
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(deployment_id)
);
CREATE INDEX IF NOT EXISTS installer_runs_state_idx ON installer_runs(state,updated_at);
