CREATE UNIQUE INDEX IF NOT EXISTS droplets_provider_identity_idx ON droplets(account_id,provider_resource_id) WHERE provider_resource_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS operations_recovery_idx ON operations(state,updated_at) WHERE state IN ('running','verifying','unknown');
CREATE INDEX IF NOT EXISTS droplets_lifecycle_idx ON droplets(account_id,state,updated_at);
