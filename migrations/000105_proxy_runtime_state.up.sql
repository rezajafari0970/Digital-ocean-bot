CREATE TABLE proxy_runtime_state (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    proxy_id UUID NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    health_state TEXT NOT NULL DEFAULT 'unknown'
        CHECK (health_state IN ('unknown','healthy','degraded','down')),
    circuit_state TEXT NOT NULL DEFAULT 'closed'
        CHECK (circuit_state IN ('closed','open','half_open')),
    consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    consecutive_successes INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_successes >= 0),
    retry_after TIMESTAMPTZ,
    generation BIGINT NOT NULL DEFAULT 1 CHECK (generation >= 1),
    last_error_class TEXT,
    last_error_detail TEXT,
    last_checked_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, proxy_id, provider)
);

CREATE INDEX proxy_runtime_state_provider_health_idx
    ON proxy_runtime_state(provider, health_state, circuit_state);
