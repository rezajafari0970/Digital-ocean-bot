-- Backfill one internal automation profile and schedule per existing account.
WITH inserted AS (
  INSERT INTO deployment_profiles(id,account_id,name,version,config,enabled)
  SELECT gen_random_uuid(),a.id,'account-auto',1,
    jsonb_build_object(
      'name','account-auto',
      'region',COALESCE(a.preferred_region,''),
      'size',COALESCE(a.preferred_sizes->>0,''),
      'image',COALESCE(a.preferred_image,''),
      'lifetime',(a.server_lifetime_seconds::bigint * 1000000000),
      'ssh_user','root',
      'inbound_id',1,
      'client_count',1,
      'email_prefix','client'
    ),true
  FROM accounts a
  WHERE NOT EXISTS (
    SELECT 1 FROM deployment_profiles p
    WHERE p.account_id=a.id AND p.name='account-auto' AND p.version=1
  )
  RETURNING id,account_id
)
INSERT INTO schedules(id,account_id,profile_id,enabled,interval_seconds,batch_size,max_concurrent,next_run_at)
SELECT gen_random_uuid(),a.id,p.id,true,a.auto_interval_seconds,a.auto_batch_size,a.auto_max_concurrent,
       now()+make_interval(secs=>a.auto_interval_seconds)
FROM accounts a
JOIN deployment_profiles p ON p.account_id=a.id AND p.name='account-auto' AND p.version=1
WHERE NOT EXISTS (
  SELECT 1 FROM schedules s WHERE s.account_id=a.id AND s.profile_id=p.id
);
