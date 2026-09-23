ALTER TABLE accounts ADD COLUMN IF NOT EXISTS server_lifetime_seconds INTEGER NOT NULL DEFAULT 7200;
ALTER TABLE accounts ADD CONSTRAINT accounts_server_lifetime_seconds_check CHECK (server_lifetime_seconds BETWEEN 1800 AND 604800);
