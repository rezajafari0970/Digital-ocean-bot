ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_profile_fk;
ALTER TABLE deployments DROP COLUMN IF EXISTS profile_snapshot;
DROP TABLE IF EXISTS deployment_profiles;
