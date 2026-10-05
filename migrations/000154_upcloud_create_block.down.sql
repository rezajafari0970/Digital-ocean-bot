DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM account_create_blocks) THEN
  RAISE EXCEPTION 'Resolve account create blocks before downgrade; older workers cannot enforce them';
 END IF;
END $$;
DROP TABLE account_create_blocks;
