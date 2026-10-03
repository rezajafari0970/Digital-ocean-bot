CREATE TABLE client_mutation_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
    inbound_id bigint NOT NULL CHECK (inbound_id > 0),
    client_id text NOT NULL CHECK (client_id <> ''),
    kind text NOT NULL CHECK (kind IN ('CREATE','UPDATE','DELETE')),
    idempotency_key text NOT NULL CHECK (idempotency_key <> ''),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    state text NOT NULL DEFAULT 'PENDING'
        CHECK (state IN ('PENDING','RUNNING','SUCCEEDED','FAILED','OBSOLETE')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_retry_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(account_id,idempotency_key)
);

CREATE INDEX client_mutation_jobs_claim_idx
ON client_mutation_jobs(state,next_retry_at,created_at)
WHERE state='PENDING';

CREATE INDEX client_mutation_jobs_panel_idx
ON client_mutation_jobs(panel_id,state,created_at);
