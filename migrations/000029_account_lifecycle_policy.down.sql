ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_server_lifetime_seconds_check;
ALTER TABLE accounts DROP COLUMN IF EXISTS server_lifetime_seconds;
