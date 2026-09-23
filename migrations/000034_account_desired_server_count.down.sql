ALTER TABLE accounts DROP CONSTRAINT IF EXISTS accounts_desired_server_count_check;
ALTER TABLE accounts DROP COLUMN IF EXISTS desired_server_count;
