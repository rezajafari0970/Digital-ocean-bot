CREATE TABLE bulk_user_rate_state (
    panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
    inbound_id bigint NOT NULL CHECK (inbound_id > 0),
    tokens double precision NOT NULL DEFAULT 0 CHECK (tokens >= 0),
    last_refill_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(panel_id,inbound_id)
);
