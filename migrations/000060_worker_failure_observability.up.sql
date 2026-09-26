CREATE TABLE IF NOT EXISTS worker_item_failures (
  kind text NOT NULL,
  item_id text NOT NULL,
  account_id uuid REFERENCES accounts(id) ON DELETE CASCADE,
  failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0),
  last_error text NOT NULL DEFAULT '',
  first_failed_at timestamptz NOT NULL DEFAULT now(),
  last_failed_at timestamptz NOT NULL DEFAULT now(),
  next_retry_at timestamptz,
  PRIMARY KEY (kind, item_id)
);
CREATE INDEX IF NOT EXISTS worker_item_failures_retry_idx ON worker_item_failures(next_retry_at) WHERE next_retry_at IS NOT NULL;
