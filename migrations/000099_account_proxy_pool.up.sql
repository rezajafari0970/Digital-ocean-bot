CREATE TABLE IF NOT EXISTS account_proxy_pool (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 proxy_id uuid NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
 priority integer NOT NULL DEFAULT 100 CHECK(priority>=0),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,proxy_id)
);
CREATE INDEX IF NOT EXISTS account_proxy_pool_active_idx ON account_proxy_pool(account_id,priority,proxy_id) WHERE enabled=true;
INSERT INTO account_proxy_pool(account_id,proxy_id,priority,enabled)
SELECT account_id,proxy_id,0,true FROM network_profiles WHERE mode='proxy_required' AND proxy_id IS NOT NULL
ON CONFLICT(account_id,proxy_id) DO NOTHING;
