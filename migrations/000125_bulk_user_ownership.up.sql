CREATE TABLE bulk_user_generations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
    inbound_id bigint NOT NULL CHECK (inbound_id > 0),
    purpose text NOT NULL CHECK (purpose IN ('POLICY','CANARY')),
    state text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','ROLLING_BACK','CLOSED')),
    marker text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz
);

CREATE TABLE bulk_user_ownership (
    generation_id uuid NOT NULL REFERENCES bulk_user_generations(id) ON DELETE CASCADE,
    client_id text NOT NULL,
    email text NOT NULL,
    state text NOT NULL DEFAULT 'PLANNED' CHECK (state IN ('PLANNED','ACTIVE','DELETE_PENDING','DELETED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    PRIMARY KEY(generation_id,client_id),
    UNIQUE(client_id)
);

CREATE INDEX bulk_user_ownership_active_idx
ON bulk_user_ownership(generation_id,state)
WHERE state <> 'DELETED';
