DROP INDEX IF EXISTS deployments_active_account_profile_idx;
CREATE UNIQUE INDEX IF NOT EXISTS deployments_active_identity_idx ON deployments(account_id,profile_id) WHERE state NOT IN ('READY','FAILED');
