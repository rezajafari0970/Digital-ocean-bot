ALTER TABLE provision_runs DROP COLUMN IF EXISTS next_retry_at;
ALTER TABLE provision_runs DROP COLUMN IF EXISTS last_error;
