UPDATE rolling_reboot_jobs SET state='PENDING',last_error='',updated_at=now() WHERE state IN ('DEFERRED','OBSOLETE');
ALTER TABLE rolling_reboot_jobs DROP CONSTRAINT IF EXISTS rolling_reboot_jobs_state_check;
ALTER TABLE rolling_reboot_jobs ADD CONSTRAINT rolling_reboot_jobs_state_check
CHECK(state IN ('PENDING','REBOOT_SENT','WAITING_SSH','VERIFYING','COMPLETED','FAILED'));
