DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM provider_cleanup_manifests WHERE completed_at IS NULL)
 OR EXISTS(SELECT 1 FROM accounts WHERE provider='upcloud' AND deleted_at IS NULL) THEN
  RAISE EXCEPTION 'Remove UpCloud accounts and complete owned storage cleanup before downgrading';
 END IF;
END $$;
DROP TABLE provider_cleanup_manifests;
