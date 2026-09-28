CREATE TABLE user_capacity_snapshots (
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 inbound_id bigint NOT NULL,
 port integer NOT NULL,
 target_users integer NOT NULL DEFAULT 0,
 active_users integer NOT NULL DEFAULT 0,
 expired_users integer NOT NULL DEFAULT 0,
 quota_exhausted_users integer NOT NULL DEFAULT 0,
 deficit integer NOT NULL DEFAULT 0,
 created_last_cycle integer NOT NULL DEFAULT 0,
 deleted_last_cycle integer NOT NULL DEFAULT 0,
 last_error text NOT NULL DEFAULT '',
 observed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,inbound_id)
);
CREATE INDEX user_capacity_snapshots_observed_idx ON user_capacity_snapshots(observed_at DESC);
