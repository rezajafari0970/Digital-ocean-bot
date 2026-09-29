DROP INDEX IF EXISTS accounts_active_idx;
ALTER TABLE accounts DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE accounts DROP COLUMN IF EXISTS deletion_requested_at;
