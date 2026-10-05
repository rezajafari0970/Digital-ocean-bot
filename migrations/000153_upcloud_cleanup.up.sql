CREATE TABLE provider_cleanup_manifests (
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 provider text NOT NULL CHECK (provider='upcloud'),
 server_id text NOT NULL,
 identity text NOT NULL CHECK (identity<>''),
 storage_ids jsonb NOT NULL CHECK (jsonb_typeof(storage_ids)='array'),
 created_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 PRIMARY KEY(account_id,provider,server_id)
);
CREATE INDEX provider_cleanup_pending_idx ON provider_cleanup_manifests(account_id) WHERE completed_at IS NULL;
GRANT SELECT,INSERT,UPDATE,DELETE ON provider_cleanup_manifests TO digitaloceanbot;
