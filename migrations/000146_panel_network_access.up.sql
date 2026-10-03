CREATE TABLE panel_network_access(
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 plan_hash text NOT NULL, verified_at timestamptz NOT NULL DEFAULT now()
);
