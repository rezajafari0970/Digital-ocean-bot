DO $$
BEGIN
 IF EXISTS (SELECT 1 FROM schedules WHERE lease_token IS NOT NULL) THEN
  RAISE EXCEPTION 'scheduler claims must be settled before removing ownership fencing';
 END IF;
END $$;
ALTER TABLE schedules DROP COLUMN lease_token;
