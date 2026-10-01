ALTER TABLE provider_capacity_observations
  DROP COLUMN IF EXISTS probe_in_flight,
  DROP COLUMN IF EXISTS probe_after;
