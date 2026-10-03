DO $$ BEGIN RAISE EXCEPTION 'Residential isolation requires an explicit data migration to roll back'; END $$;
