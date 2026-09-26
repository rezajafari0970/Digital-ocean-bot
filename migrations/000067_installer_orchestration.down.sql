DROP TABLE IF EXISTS installer_runs;
DROP TRIGGER IF EXISTS installers_immutable ON installers;
DROP FUNCTION IF EXISTS protect_installer_version();
DROP TABLE IF EXISTS installers;
