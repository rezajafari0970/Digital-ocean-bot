CREATE TABLE xui_database_templates (
 id UUID PRIMARY KEY,
 name TEXT NOT NULL,
 version INTEGER NOT NULL,
 storage_path TEXT NOT NULL,
 sha256 TEXT NOT NULL,
 size_bytes BIGINT NOT NULL,
 active BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(name,version)
);

CREATE TABLE xui_database_deployments (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 droplet_id UUID NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 template_id UUID NOT NULL REFERENCES xui_database_templates(id),
 state TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX xui_database_deployments_recovery_idx ON xui_database_deployments(state,updated_at) WHERE state NOT IN ('COMPLETED');
