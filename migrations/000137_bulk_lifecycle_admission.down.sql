DO $$ BEGIN IF EXISTS(SELECT 1 FROM bulk_lifecycle_control WHERE auto_enroll) THEN RAISE EXCEPTION 'close automatic admission first'; END IF; END $$;
ALTER TABLE bulk_lifecycle_control DROP COLUMN auto_enroll,DROP COLUMN max_active_scopes,DROP COLUMN operation_budget,DROP COLUMN max_batch_size;
