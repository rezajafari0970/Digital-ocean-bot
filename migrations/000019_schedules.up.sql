CREATE TABLE schedules (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 profile_id UUID NOT NULL REFERENCES deployment_profiles(id) ON DELETE CASCADE,
 enabled BOOLEAN NOT NULL DEFAULT true,
 interval_seconds INTEGER NOT NULL CHECK(interval_seconds>=60),
 batch_size INTEGER NOT NULL DEFAULT 1 CHECK(batch_size BETWEEN 1 AND 100),
 max_concurrent INTEGER NOT NULL DEFAULT 1 CHECK(max_concurrent BETWEEN 1 AND 1000),
 next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_run_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(account_id,profile_id)
);
CREATE INDEX schedules_due_idx ON schedules(next_run_at) WHERE enabled=true;
