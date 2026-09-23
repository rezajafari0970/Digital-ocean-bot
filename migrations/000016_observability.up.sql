CREATE TABLE audit_events (
 id BIGSERIAL PRIMARY KEY,
 account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
 actor TEXT NOT NULL,
 action TEXT NOT NULL,
 resource_type TEXT NOT NULL,
 resource_id TEXT,
 result TEXT NOT NULL,
 message TEXT NOT NULL DEFAULT '',
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_account_time_idx ON audit_events(account_id,created_at DESC);
CREATE INDEX audit_events_action_time_idx ON audit_events(action,created_at DESC);

CREATE TABLE worker_heartbeats (
 worker_id TEXT PRIMARY KEY,
 kind TEXT NOT NULL,
 last_seen_at TIMESTAMPTZ NOT NULL,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
