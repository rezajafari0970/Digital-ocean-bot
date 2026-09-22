CREATE TABLE droplets (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 provider_resource_id TEXT,
 state TEXT NOT NULL,
 profile JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE lifecycle_events (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 resource_id UUID,
 state TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
