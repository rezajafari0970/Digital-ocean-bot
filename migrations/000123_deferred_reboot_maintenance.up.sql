ALTER TABLE rolling_reboot_jobs DROP CONSTRAINT IF EXISTS rolling_reboot_jobs_state_check;
ALTER TABLE rolling_reboot_jobs ADD CONSTRAINT rolling_reboot_jobs_state_check
CHECK(state IN ('PENDING','DEFERRED','REBOOT_SENT','WAITING_SSH','VERIFYING','COMPLETED','FAILED','OBSOLETE'));

UPDATE rolling_reboot_jobs
SET state='DEFERRED',
    next_retry_at=NULL,
    last_error='maintenance deferred; lifecycle rotation may obsolete reboot',
    updated_at=now()
WHERE state='PENDING';
