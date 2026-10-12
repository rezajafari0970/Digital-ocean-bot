-- Deliberately irreversible: older runtimes must not bypass pending setup.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM admin_users WHERE must_change_password) THEN
  RAISE EXCEPTION 'Complete initial password setup before downgrading';
 END IF;
END $$;
ALTER TABLE admin_users DROP COLUMN bootstrap_token_hash;
ALTER TABLE admin_users DROP COLUMN must_change_password;
