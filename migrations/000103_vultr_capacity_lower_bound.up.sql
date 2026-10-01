ALTER TABLE provider_capacity_observations
  ADD COLUMN IF NOT EXISTS lower_bound INTEGER NOT NULL DEFAULT 0 CHECK (lower_bound >= 0);
