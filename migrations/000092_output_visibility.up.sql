ALTER TABLE output_config_snapshots
ADD COLUMN IF NOT EXISTS visible_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS output_config_snapshots_visible_idx
ON output_config_snapshots(visible_until)
WHERE visible_until IS NOT NULL;
