DROP INDEX IF EXISTS droplets_replacement_idx;
ALTER TABLE droplets DROP COLUMN IF EXISTS replacement_deployment_id;
ALTER TABLE droplets DROP COLUMN IF EXISTS profile_id;
