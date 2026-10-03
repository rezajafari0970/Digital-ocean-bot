DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM bulk_lifecycle_scopes) OR EXISTS(SELECT 1 FROM client_mutation_jobs WHERE payload->>'Lifecycle'='true') THEN RAISE EXCEPTION 'lifecycle audit exists; use forward recovery'; END IF;
END $$;
DROP INDEX lifecycle_unresolved_idx;
DROP TABLE bulk_lifecycle_scopes;
DROP TABLE bulk_lifecycle_control;
