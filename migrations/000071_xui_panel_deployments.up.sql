CREATE TABLE xui_panel_deployments (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 droplet_id UUID NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 username TEXT NOT NULL,
 password_secret_ref TEXT NOT NULL,
 port INTEGER NOT NULL CHECK(port BETWEEN 1024 AND 65535),
 web_path TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('CONFIGURING','FAILED','COMPLETED')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(droplet_id)
);
CREATE INDEX xui_panel_deployments_recovery_idx ON xui_panel_deployments(state,updated_at) WHERE state<>'COMPLETED';
