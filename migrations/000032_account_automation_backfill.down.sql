DELETE FROM deployment_profiles p
WHERE p.name='account-auto' AND p.version=1
AND NOT EXISTS (SELECT 1 FROM deployments d WHERE d.profile_id=p.id);
