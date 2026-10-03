ALTER TABLE panel_instances
  ADD COLUMN IF NOT EXISTS health_state text NOT NULL DEFAULT 'UNKNOWN',
  ADD COLUMN IF NOT EXISTS health_failures integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS health_successes integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS health_checked_at timestamptz,
  ADD COLUMN IF NOT EXISTS health_last_ok_at timestamptz,
  ADD COLUMN IF NOT EXISTS health_last_error text;

ALTER TABLE panel_instances
  DROP CONSTRAINT IF EXISTS panel_instances_health_state_check;
ALTER TABLE panel_instances
  ADD CONSTRAINT panel_instances_health_state_check
  CHECK (health_state IN ('UNKNOWN','HEALTHY','DEGRADED','UNHEALTHY'));

CREATE INDEX IF NOT EXISTS panel_instances_health_idx
  ON panel_instances(enabled,health_state,health_checked_at);
