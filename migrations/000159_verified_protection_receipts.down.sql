DROP INDEX IF EXISTS panel_inventory_syncs_recent_idx;
ALTER TABLE server_protection_nodes DROP COLUMN verified_at,DROP COLUMN verified_status;
DROP INDEX IF EXISTS panel_inventory_syncs_retention_idx;
