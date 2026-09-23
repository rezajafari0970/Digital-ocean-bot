INSERT INTO schedules(id,account_id,profile_id,enabled,interval_seconds,batch_size,max_concurrent,next_run_at)
SELECT gen_random_uuid(),a.id,p.id,true,a.auto_interval_seconds,a.auto_batch_size,a.auto_max_concurrent,
       now()+make_interval(secs=>a.auto_interval_seconds)
FROM accounts a
JOIN deployment_profiles p ON p.account_id=a.id AND p.name='account-auto' AND p.version=1
WHERE NOT EXISTS (SELECT 1 FROM schedules s WHERE s.account_id=a.id AND s.profile_id=p.id);
