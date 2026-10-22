CREATE TABLE provider_account_resources (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 provider text NOT NULL,
 scope text NOT NULL,
 name text NOT NULL,
 nonce uuid NOT NULL DEFAULT gen_random_uuid(),
 resource_id text NOT NULL DEFAULT '',
 last_attempt_at timestamptz,
 missing_since timestamptz,
 last_missing_at timestamptz,
 missing_count integer NOT NULL DEFAULT 0,
 completed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,provider,scope),
 UNIQUE(account_id,provider,name)
);
