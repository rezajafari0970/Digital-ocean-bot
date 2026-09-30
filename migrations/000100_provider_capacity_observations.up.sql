CREATE TABLE IF NOT EXISTS provider_capacity_observations (
  account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  compute_limit INTEGER NOT NULL CHECK (compute_limit >= 0),
  source TEXT NOT NULL,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS provider_capacity_observations_observed_idx ON provider_capacity_observations(observed_at DESC);
