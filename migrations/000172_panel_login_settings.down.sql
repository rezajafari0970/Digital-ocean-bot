-- Credential restrictions are not recreated during rollback.
ALTER TABLE panel_settings DROP CONSTRAINT IF EXISTS panel_listen_host_fixed;
