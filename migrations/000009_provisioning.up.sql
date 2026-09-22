CREATE TABLE provision_runs (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 droplet_id UUID NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 state TEXT NOT NULL,
 current_step TEXT NOT NULL,
 attempt INTEGER NOT NULL DEFAULT 0,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(account_id,droplet_id)
);

CREATE TABLE provision_events (
 id UUID PRIMARY KEY,
 run_id UUID NOT NULL REFERENCES provision_runs(id) ON DELETE CASCADE,
 step TEXT NOT NULL,
 state TEXT NOT NULL,
 error_code TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX provision_runs_recovery_idx ON provision_runs(state,updated_at) WHERE state NOT IN ('COMPLETED');
CREATE INDEX provision_events_run_idx ON provision_events(run_id,created_at);
