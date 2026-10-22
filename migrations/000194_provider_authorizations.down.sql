-- Preserve pending/verified encrypted credentials on rollback.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM provider_authorizations WHERE ciphertext IS NOT NULL) THEN
  RAISE EXCEPTION 'Pending provider authorization credentials must be settled before rollback';
 END IF;
END $$;
DROP TABLE provider_authorizations;
