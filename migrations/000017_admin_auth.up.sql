CREATE TABLE admin_users (
 id UUID PRIMARY KEY,
 username TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE admin_sessions (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
 token_hash BYTEA NOT NULL UNIQUE,
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_sessions_expiry_idx ON admin_sessions(expires_at);
