DROP INDEX IF EXISTS droplets_expiration_idx;
ALTER TABLE droplets DROP COLUMN IF EXISTS expires_at;
ALTER TABLE droplets DROP COLUMN IF EXISTS ready_at;
