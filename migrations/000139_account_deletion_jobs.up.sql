CREATE TABLE account_deletion_jobs (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 requested_at timestamptz NOT NULL DEFAULT now(),
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 attempts integer NOT NULL DEFAULT 0,
 last_error text NOT NULL DEFAULT '',
 verified_at timestamptz
);
CREATE TABLE account_deletion_keys (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 provider_key_id text NOT NULL,
 state text NOT NULL DEFAULT 'PLANNED' CHECK(state IN ('PLANNED','RUNNING','SUCCEEDED')),
 attempts integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,provider_key_id)
);
