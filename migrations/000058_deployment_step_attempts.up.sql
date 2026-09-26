CREATE TABLE IF NOT EXISTS deployment_step_attempts (
  deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  step text NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_started_at timestamptz,
  last_finished_at timestamptz,
  last_error text,
  last_error_class text,
  next_retry_at timestamptz,
  PRIMARY KEY (deployment_id, step)
);
ALTER TABLE deployment_step_attempts ADD COLUMN IF NOT EXISTS last_error_class text;
ALTER TABLE deployment_step_attempts ADD COLUMN IF NOT EXISTS next_retry_at timestamptz;
CREATE INDEX IF NOT EXISTS deployment_step_attempts_step_idx ON deployment_step_attempts(step, attempts);
CREATE INDEX IF NOT EXISTS deployment_step_attempts_retry_idx ON deployment_step_attempts(next_retry_at) WHERE next_retry_at IS NOT NULL;
