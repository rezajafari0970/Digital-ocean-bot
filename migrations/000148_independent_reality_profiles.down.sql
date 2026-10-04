-- Deliberately fail closed: merging independent limits back into one policy
-- requires an explicit data migration, not a lossy automatic rollback.
DO $$ BEGIN RAISE EXCEPTION 'independent Reality profiles require explicit rollback reconciliation'; END $$;
