-- Keep durable operator acknowledgements until explicitly completed/resolved.
-- Lock, check and drop are one statement, including under an autocommit client.
DO $$ BEGIN
 LOCK TABLE account_purge_requests IN ACCESS EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM account_purge_requests) THEN
  RAISE EXCEPTION 'account purge requests remain; rollback must preserve durable intent';
 END IF;
 DROP TABLE account_purge_requests;
END $$;
ALTER TABLE accounts DROP COLUMN provider_inventory_interval_seconds;
DROP INDEX IF EXISTS account_cleanup_relay_account_idx;
DROP INDEX IF EXISTS account_cleanup_relay_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_relay_panel_idx;
DROP INDEX IF EXISTS account_cleanup_relay_endpoint_idx;
DROP INDEX IF EXISTS account_cleanup_panel_job_idx;
DROP INDEX IF EXISTS account_cleanup_waiting_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_targets_idx;
DROP INDEX IF EXISTS account_cleanup_scope_generation_idx;
DROP INDEX IF EXISTS account_cleanup_ownership_job_idx;
DROP INDEX IF EXISTS account_cleanup_generations_idx;
DROP INDEX IF EXISTS account_cleanup_reboot_account_idx;
DROP INDEX IF EXISTS account_cleanup_reboot_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_panels_idx;
DROP INDEX IF EXISTS account_cleanup_xui_panel_idx;
DROP INDEX IF EXISTS account_cleanup_installer_run_idx;
DROP INDEX IF EXISTS account_cleanup_readiness_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_readiness_idx;
DROP INDEX IF EXISTS account_cleanup_failures_idx;
DROP INDEX IF EXISTS account_cleanup_deployment_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_deployments_idx;
DROP INDEX IF EXISTS account_cleanup_xui_db_idx;
DROP INDEX IF EXISTS account_cleanup_provision_droplet_idx;
DROP INDEX IF EXISTS account_cleanup_lifecycle_idx;
DROP INDEX IF EXISTS account_cleanup_snapshots_idx;
DROP INDEX IF EXISTS account_cleanup_panel_sync_idx;
