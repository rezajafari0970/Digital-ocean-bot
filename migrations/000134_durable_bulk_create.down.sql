DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM client_mutation_jobs WHERE kind='BULK_CREATE') THEN
  RAISE EXCEPTION 'bulk audit history exists; rollback requires explicit archival';
 END IF;
END $$;
DROP TRIGGER protect_bulk_payload ON client_mutation_jobs;
DROP FUNCTION protect_bulk_payload();
DROP TABLE bulk_client_execution_gate;
DROP INDEX client_bulk_unresolved_idx;
ALTER TABLE bulk_user_ownership DROP COLUMN mutation_job_id;
ALTER TABLE client_mutation_jobs DROP COLUMN result;
ALTER TABLE client_mutation_jobs DROP CONSTRAINT client_mutation_jobs_kind_check;
ALTER TABLE client_mutation_jobs ADD CONSTRAINT client_mutation_jobs_kind_check CHECK(kind IN ('CREATE','UPDATE','DELETE'));
