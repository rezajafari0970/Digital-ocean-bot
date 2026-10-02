CREATE TABLE panel_runtime_repairs (
 panel_id UUID PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_success_at TIMESTAMPTZ,
 attempts BIGINT NOT NULL DEFAULT 1,
 last_error TEXT NOT NULL DEFAULT ''
);
