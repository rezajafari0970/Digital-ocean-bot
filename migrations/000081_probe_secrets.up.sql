ALTER TABLE secrets DROP CONSTRAINT secrets_owner_check;
ALTER TABLE secrets DROP COLUMN owner_key;
ALTER TABLE secrets ADD COLUMN probe_id uuid REFERENCES reality_probe_nodes(id) ON DELETE CASCADE;
ALTER TABLE secrets ADD COLUMN owner_key text GENERATED ALWAYS AS (COALESCE(account_id::text,proxy_id::text,probe_id::text)) STORED;
ALTER TABLE secrets ADD CONSTRAINT secrets_owner_check CHECK ((account_id IS NOT NULL)::int+(proxy_id IS NOT NULL)::int+(probe_id IS NOT NULL)::int=1);
CREATE UNIQUE INDEX secrets_probe_id_id_idx ON secrets(probe_id,id) WHERE probe_id IS NOT NULL;
ALTER TABLE reality_probe_nodes DROP COLUMN account_id;
