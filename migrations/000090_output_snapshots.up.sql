CREATE TABLE IF NOT EXISTS output_config_snapshots (
 panel_id UUID NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 uri TEXT NOT NULL,
 first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,uri)
);
CREATE INDEX IF NOT EXISTS output_config_snapshots_first_seen_idx ON output_config_snapshots(first_seen_at DESC);
CREATE TABLE IF NOT EXISTS output_share_tokens (
 token TEXT PRIMARY KEY,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 revoked_at TIMESTAMPTZ
);
