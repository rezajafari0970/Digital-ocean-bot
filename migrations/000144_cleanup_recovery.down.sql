DO $$ BEGIN RAISE EXCEPTION 'Cancelled cleanup audit records require an explicit rollback migration'; END $$;
