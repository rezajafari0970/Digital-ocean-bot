CREATE TABLE IF NOT EXISTS inbound_structural_snapshots (
 panel_id UUID NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 remote_id BIGINT NOT NULL,
 port INTEGER NOT NULL,
 payload JSONB NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,remote_id)
);
CREATE INDEX IF NOT EXISTS inbound_structural_snapshots_panel_idx ON inbound_structural_snapshots(panel_id,port);
