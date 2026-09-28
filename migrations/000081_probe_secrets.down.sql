DELETE FROM secrets WHERE probe_id IS NOT NULL;
ALTER TABLE reality_probe_nodes ADD COLUMN account_id uuid REFERENCES accounts(id) ON DELETE CASCADE;
DROP INDEX IF EXISTS secrets_probe_id_id_idx;
ALTER TABLE secrets DROP CONSTRAINT secrets_owner_check;
ALTER TABLE secrets DROP COLUMN owner_key;
ALTER TABLE secrets DROP COLUMN probe_id;
ALTER TABLE secrets ADD COLUMN owner_key text GENERATED ALWAYS AS (COALESCE(account_id::text,proxy_id::text)) STORED;
ALTER TABLE secrets ADD CONSTRAINT secrets_owner_check CHECK ((account_id IS NOT NULL)::int+(proxy_id IS NOT NULL)::int=1);
