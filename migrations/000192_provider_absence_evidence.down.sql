DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM accounts WHERE provider IN ('aws','ovh')) THEN
  RAISE EXCEPTION 'remove AWS/OVH accounts through verified cleanup before downgrading';
 END IF;
END $$;
DROP TABLE provider_absence_evidence;
