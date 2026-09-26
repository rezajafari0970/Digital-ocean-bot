CREATE TABLE IF NOT EXISTS provision_step_attempts (
  run_id uuid NOT NULL REFERENCES provision_runs(id) ON DELETE CASCADE,
  step text NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_started_at timestamptz,
  last_finished_at timestamptz,
  last_error text,
  next_retry_at timestamptz,
  terminal boolean NOT NULL DEFAULT false,
  PRIMARY KEY (run_id, step)
);
CREATE INDEX IF NOT EXISTS provision_step_attempts_retry_idx ON provision_step_attempts(next_retry_at) WHERE next_retry_at IS NOT NULL;
