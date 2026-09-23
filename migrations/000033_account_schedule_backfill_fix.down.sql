DELETE FROM schedules s USING deployment_profiles p
WHERE s.profile_id=p.id AND p.name='account-auto' AND p.version=1 AND s.last_run_at IS NULL;
