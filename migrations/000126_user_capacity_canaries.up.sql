CREATE TABLE user_capacity_canaries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_id uuid NOT NULL UNIQUE REFERENCES bulk_user_generations(id) ON DELETE CASCADE,
    panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
    inbound_id bigint NOT NULL CHECK (inbound_id > 0),
    target_users integer NOT NULL CHECK (target_users BETWEEN 2 AND 10000),
    users_per_second integer NOT NULL CHECK (users_per_second BETWEEN 1 AND 100),
    state text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','ROLLBACK_PENDING','ROLLING_BACK','COMPLETED','FAILED')),
    expires_at timestamptz NOT NULL,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    CHECK (expires_at > created_at),
    UNIQUE(panel_id,inbound_id)
);

CREATE INDEX user_capacity_canaries_due_idx
ON user_capacity_canaries(state,expires_at)
WHERE state IN ('ACTIVE','ROLLBACK_PENDING','ROLLING_BACK');
