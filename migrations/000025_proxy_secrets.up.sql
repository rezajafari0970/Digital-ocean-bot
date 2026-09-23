ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_pkey;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_account_id_fkey;
ALTER TABLE secrets ADD COLUMN proxy_id UUID REFERENCES proxies(id) ON DELETE CASCADE;
ALTER TABLE secrets ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE secrets ADD COLUMN owner_key TEXT GENERATED ALWAYS AS (COALESCE(account_id::text,proxy_id::text)) STORED;
ALTER TABLE secrets ADD CONSTRAINT secrets_pkey PRIMARY KEY(owner_key,id);
ALTER TABLE secrets ADD CONSTRAINT secrets_owner_check CHECK ((account_id IS NOT NULL)::int + (proxy_id IS NOT NULL)::int = 1);
CREATE UNIQUE INDEX secrets_proxy_id_id_idx ON secrets(proxy_id,id) WHERE proxy_id IS NOT NULL;
