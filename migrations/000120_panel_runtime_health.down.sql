DROP INDEX IF EXISTS panel_instances_health_idx;
ALTER TABLE panel_instances
  DROP CONSTRAINT IF EXISTS panel_instances_health_state_check,
  DROP COLUMN IF EXISTS health_last_error,
  DROP COLUMN IF EXISTS health_last_ok_at,
  DROP COLUMN IF EXISTS health_checked_at,
  DROP COLUMN IF EXISTS health_successes,
  DROP COLUMN IF EXISTS health_failures,
  DROP COLUMN IF EXISTS health_state;
