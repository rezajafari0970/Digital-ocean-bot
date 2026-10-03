-- Preserve journal audit: rollback is refused while the new kind/run data exists.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM client_mutation_jobs WHERE kind='BULK_DELETE') OR EXISTS(SELECT 1 FROM bulk_scale_runs) THEN
  RAISE EXCEPTION 'bulk scale audit exists; forward recovery required';
 END IF;
END $$;
DROP TABLE bulk_scale_runs;
ALTER TABLE client_mutation_jobs DROP CONSTRAINT client_mutation_jobs_kind_check;
ALTER TABLE client_mutation_jobs ADD CONSTRAINT client_mutation_jobs_kind_check CHECK(kind IN ('CREATE','UPDATE','DELETE','BULK_CREATE'));
DROP INDEX client_bulk_unresolved_idx;
CREATE UNIQUE INDEX client_bulk_unresolved_idx ON client_mutation_jobs(panel_id,inbound_id) WHERE kind='BULK_CREATE' AND state IN ('PENDING','RUNNING','FAILED');
