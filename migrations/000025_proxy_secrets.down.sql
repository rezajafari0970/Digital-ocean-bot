DELETE FROM secrets WHERE proxy_id IS NOT NULL;
DROP INDEX IF EXISTS secrets_proxy_id_id_idx;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_owner_check;
ALTER TABLE secrets DROP COLUMN IF EXISTS proxy_id;
ALTER TABLE secrets ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE secrets ADD CONSTRAINT secrets_account_id_fkey FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE;
