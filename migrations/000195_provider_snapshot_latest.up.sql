-- Account summaries must seek the latest snapshot before decoding its JSON.
-- The canonical-only partial index cannot cover legacy or null snapshots.
CREATE INDEX IF NOT EXISTS idx_provider_snapshots_account_latest
 ON provider_snapshots(account_id,created_at DESC);
