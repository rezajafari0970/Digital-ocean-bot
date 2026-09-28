CREATE TABLE inbound_export_metadata (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 remote_id bigint NOT NULL,
 public_key text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,remote_id)
);
