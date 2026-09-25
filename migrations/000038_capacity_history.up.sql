CREATE TABLE IF NOT EXISTS account_capacity_events (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 old_limit INTEGER NOT NULL,
 new_limit INTEGER NOT NULL,
 delta INTEGER NOT NULL,
 detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 snapshot_id UUID REFERENCES provider_snapshots(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS account_capacity_events_account_time_idx ON account_capacity_events(account_id,detected_at DESC);
