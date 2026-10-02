CREATE TABLE IF NOT EXISTS reality_rollout_panels (
    panel_id UUID PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    stage TEXT NOT NULL DEFAULT 'canary',
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_reality_rollout_enabled ON reality_rollout_panels(enabled) WHERE enabled=true;
