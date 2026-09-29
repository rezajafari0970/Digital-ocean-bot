ALTER TABLE provider_snapshots ADD COLUMN IF NOT EXISTS canonical jsonb;
CREATE INDEX IF NOT EXISTS idx_provider_snapshots_account_canonical ON provider_snapshots(account_id,created_at DESC) WHERE canonical IS NOT NULL;
