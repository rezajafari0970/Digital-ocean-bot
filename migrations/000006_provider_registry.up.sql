CREATE TABLE provider_snapshots (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 provider TEXT NOT NULL,
 version INTEGER NOT NULL DEFAULT 1,
 data JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE resources (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 provider TEXT NOT NULL,
 provider_resource_id TEXT NOT NULL,
 type TEXT NOT NULL,
 state TEXT NOT NULL,
 managed BOOLEAN NOT NULL DEFAULT false,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
