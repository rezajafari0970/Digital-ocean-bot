DROP INDEX IF EXISTS deployments_active_identity_idx;
CREATE INDEX IF NOT EXISTS deployments_active_account_profile_idx ON deployments(account_id,profile_id,created_at) WHERE state NOT IN ('READY','FAILED');
