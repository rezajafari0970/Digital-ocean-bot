DROP INDEX IF EXISTS idx_provider_snapshots_account_canonical;
ALTER TABLE provider_snapshots DROP COLUMN IF EXISTS canonical;
