ALTER TABLE operations DROP COLUMN IF EXISTS lock_version;
ALTER TABLE deployments DROP COLUMN IF EXISTS lock_version;
DROP INDEX IF EXISTS schedules_lease_idx;
ALTER TABLE schedules DROP COLUMN IF EXISTS lease_until;
