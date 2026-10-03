CREATE TABLE IF NOT EXISTS rolling_reboot_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
  account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  droplet_id uuid NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
  state text NOT NULL DEFAULT 'PENDING',
  attempts integer NOT NULL DEFAULT 0,
  started_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  next_retry_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  UNIQUE(panel_id)
);
ALTER TABLE rolling_reboot_jobs DROP CONSTRAINT IF EXISTS rolling_reboot_jobs_state_check;
ALTER TABLE rolling_reboot_jobs ADD CONSTRAINT rolling_reboot_jobs_state_check
CHECK(state IN ('PENDING','REBOOT_SENT','WAITING_SSH','VERIFYING','COMPLETED','FAILED'));
CREATE INDEX IF NOT EXISTS rolling_reboot_jobs_due_idx ON rolling_reboot_jobs(state,next_retry_at,updated_at);
