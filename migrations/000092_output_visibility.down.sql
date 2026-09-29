DROP INDEX IF EXISTS output_config_snapshots_visible_idx;
ALTER TABLE output_config_snapshots DROP COLUMN IF EXISTS visible_until;
