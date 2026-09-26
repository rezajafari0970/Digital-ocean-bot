CREATE TABLE IF NOT EXISTS install_scripts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  version integer NOT NULL CHECK(version > 0),
  category text NOT NULL DEFAULT 'script',
  precheck text NOT NULL DEFAULT '',
  execute text NOT NULL DEFAULT '',
  verify text NOT NULL DEFAULT '',
  timeout_seconds integer NOT NULL DEFAULT 600 CHECK(timeout_seconds > 0),
  max_attempts integer NOT NULL DEFAULT 3 CHECK(max_attempts > 0),
  sha256 text NOT NULL CHECK(length(sha256)=64),
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(name,version),
  UNIQUE(sha256)
);
CREATE OR REPLACE FUNCTION protect_install_script_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.name<>OLD.name OR NEW.version<>OLD.version OR NEW.category<>OLD.category OR NEW.precheck<>OLD.precheck OR NEW."execute"<>OLD."execute" OR NEW.verify<>OLD.verify OR NEW.timeout_seconds<>OLD.timeout_seconds OR NEW.max_attempts<>OLD.max_attempts OR NEW.sha256<>OLD.sha256 THEN
    RAISE EXCEPTION 'install script versions are immutable; create a new version';
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS install_scripts_immutable ON install_scripts;
CREATE TRIGGER install_scripts_immutable BEFORE UPDATE ON install_scripts FOR EACH ROW EXECUTE FUNCTION protect_install_script_version();
