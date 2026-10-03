ALTER TABLE client_mutation_jobs DROP CONSTRAINT client_mutation_jobs_kind_check;
ALTER TABLE client_mutation_jobs ADD CONSTRAINT client_mutation_jobs_kind_check CHECK(kind IN ('CREATE','UPDATE','DELETE','BULK_CREATE','BULK_DELETE'));
DROP INDEX client_bulk_unresolved_idx;
CREATE UNIQUE INDEX client_bulk_unresolved_idx ON client_mutation_jobs(panel_id,inbound_id) WHERE kind IN ('BULK_CREATE','BULK_DELETE') AND state IN ('PENDING','RUNNING','FAILED');
CREATE OR REPLACE FUNCTION protect_bulk_payload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.kind IN ('BULK_CREATE','BULK_DELETE') AND (NEW.payload IS DISTINCT FROM OLD.payload OR NEW.kind<>OLD.kind OR NEW.panel_id<>OLD.panel_id OR NEW.inbound_id<>OLD.inbound_id OR NEW.client_id<>OLD.client_id OR NEW.account_id<>OLD.account_id OR NEW.idempotency_key<>OLD.idempotency_key) THEN
  RAISE EXCEPTION 'bulk plan identity and payload are immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TABLE bulk_scale_runs (
 generation_id uuid PRIMARY KEY REFERENCES bulk_user_generations(id),
 target_users integer NOT NULL CHECK(target_users BETWEEN 2 AND 10000),
 baseline jsonb NOT NULL,
 phase text NOT NULL CHECK(phase IN ('CREATING','VERIFYING','CLEANING','SUCCEEDED','FAILED')),
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 last_error text NOT NULL DEFAULT '',
 evidence jsonb NOT NULL DEFAULT '{}'::jsonb
);
