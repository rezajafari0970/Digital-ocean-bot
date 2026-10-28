DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM provider_authorizations WHERE proxy_country<>'AUTO') THEN
  RAISE EXCEPTION 'explicit authorization country must not be erased by rollback';
 END IF;
END $$;
ALTER TABLE provider_authorizations DROP COLUMN proxy_country;
