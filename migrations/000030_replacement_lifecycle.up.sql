ALTER TABLE droplets ADD COLUMN IF NOT EXISTS profile_id UUID;
ALTER TABLE droplets ADD COLUMN IF NOT EXISTS replacement_deployment_id UUID;
CREATE INDEX IF NOT EXISTS droplets_replacement_idx ON droplets(account_id,state,expires_at);
