ALTER TABLE client_mutation_jobs DROP CONSTRAINT client_mutation_jobs_kind_check;
ALTER TABLE client_mutation_jobs ADD CONSTRAINT client_mutation_jobs_kind_check CHECK (kind IN ('CREATE','UPDATE','DELETE','BULK_CREATE'));
ALTER TABLE client_mutation_jobs ADD COLUMN result jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE bulk_user_ownership ADD COLUMN mutation_job_id uuid REFERENCES client_mutation_jobs(id);
CREATE INDEX bulk_user_ownership_job_idx ON bulk_user_ownership(mutation_job_id) WHERE mutation_job_id IS NOT NULL;
CREATE UNIQUE INDEX client_bulk_unresolved_idx ON client_mutation_jobs(panel_id,inbound_id) WHERE kind='BULK_CREATE' AND state IN ('PENDING','RUNNING','FAILED');
CREATE TABLE bulk_client_execution_gate (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 kill_switch boolean NOT NULL DEFAULT true,
 panel_id uuid REFERENCES panel_instances(id) ON DELETE CASCADE,
 inbound_id bigint CHECK(inbound_id>0),
 max_batch_size integer NOT NULL DEFAULT 10 CHECK(max_batch_size BETWEEN 1 AND 250),
 remaining_batches integer NOT NULL DEFAULT 0 CHECK(remaining_batches>=0),
 expires_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (NOT enabled OR (NOT kill_switch AND panel_id IS NOT NULL AND inbound_id IS NOT NULL AND expires_at IS NOT NULL))
);
INSERT INTO bulk_client_execution_gate(singleton) VALUES(true);
CREATE FUNCTION protect_bulk_payload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.kind='BULK_CREATE' AND (NEW.payload IS DISTINCT FROM OLD.payload OR NEW.kind<>OLD.kind OR NEW.panel_id<>OLD.panel_id OR NEW.inbound_id<>OLD.inbound_id OR NEW.client_id<>OLD.client_id OR NEW.account_id<>OLD.account_id OR NEW.idempotency_key<>OLD.idempotency_key) THEN
  RAISE EXCEPTION 'bulk plan identity and payload are immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER protect_bulk_payload BEFORE UPDATE ON client_mutation_jobs FOR EACH ROW EXECUTE FUNCTION protect_bulk_payload();
