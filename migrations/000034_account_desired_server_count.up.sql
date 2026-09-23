ALTER TABLE accounts ADD COLUMN IF NOT EXISTS desired_server_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE accounts ADD CONSTRAINT accounts_desired_server_count_check CHECK (desired_server_count BETWEEN 0 AND 1000);
