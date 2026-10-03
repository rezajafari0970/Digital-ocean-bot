
ALTER TABLE panel_cleanup_jobs DROP CONSTRAINT panel_cleanup_jobs_state_check;
ALTER TABLE panel_cleanup_jobs ADD CHECK(state IN('QUEUED','RUNNING','PAUSED','SUCCEEDED','CANCELLED'));
DROP INDEX one_active_panel_cleanup;
CREATE UNIQUE INDEX one_active_panel_cleanup ON panel_cleanup_jobs((true)) WHERE state NOT IN('SUCCEEDED','CANCELLED');
