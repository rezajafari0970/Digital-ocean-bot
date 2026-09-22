ALTER TABLE droplets ADD COLUMN ready_at TIMESTAMPTZ;
ALTER TABLE droplets ADD COLUMN expires_at TIMESTAMPTZ;
CREATE INDEX droplets_expiration_idx ON droplets(expires_at,state) WHERE expires_at IS NOT NULL AND state IN ('READY','EXPIRING','RETIRING','DELETING');
