CREATE TABLE operations (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 kind TEXT NOT NULL,
 idempotency_key TEXT NOT NULL,
 state TEXT NOT NULL CHECK (state IN ('planned','running','verifying','succeeded','failed','unknown')),
 provider_action_id TEXT,
 resource_id TEXT,
 attempt INTEGER NOT NULL DEFAULT 0,
 error_code TEXT,
 error_message TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (account_id, idempotency_key)
);

CREATE INDEX operations_account_state_idx ON operations(account_id, state);

CREATE TABLE account_runtime_state (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 circuit_state TEXT NOT NULL DEFAULT 'closed',
 consecutive_failures INTEGER NOT NULL DEFAULT 0,
 retry_after TIMESTAMPTZ,
 rate_remaining INTEGER,
 rate_reset_at TIMESTAMPTZ,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
