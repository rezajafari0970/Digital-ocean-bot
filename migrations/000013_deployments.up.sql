CREATE TABLE deployments (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 profile_id UUID NOT NULL,
 droplet_id UUID REFERENCES droplets(id) ON DELETE SET NULL,
 provider_id TEXT,
 state TEXT NOT NULL,
 current_step TEXT NOT NULL,
 attempt INTEGER NOT NULL DEFAULT 0,
 last_error TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX deployments_active_identity_idx ON deployments(account_id,profile_id) WHERE state NOT IN ('READY','FAILED');
CREATE INDEX deployments_recovery_idx ON deployments(state,updated_at) WHERE state NOT IN ('READY','FAILED');

CREATE TABLE deployment_events (
 id BIGSERIAL PRIMARY KEY,
 deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
 step TEXT NOT NULL,
 state TEXT NOT NULL,
 message TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deployment_events_deployment_time_idx ON deployment_events(deployment_id,created_at);
