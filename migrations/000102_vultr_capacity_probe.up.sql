ALTER TABLE provider_capacity_observations
  ADD COLUMN IF NOT EXISTS probe_after TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS probe_in_flight BOOLEAN NOT NULL DEFAULT false;
