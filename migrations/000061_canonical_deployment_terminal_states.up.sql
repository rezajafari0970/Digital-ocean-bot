DROP INDEX IF EXISTS deployments_active_account_profile_idx;
CREATE INDEX deployments_active_account_profile_idx
ON deployments(account_id,profile_id,created_at)
WHERE state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE');
