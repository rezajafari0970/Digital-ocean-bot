CREATE TABLE reality_transport_apply_jobs (
 id uuid PRIMARY KEY,
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 managed_key text NOT NULL,
 policy_revision bigint NOT NULL,
 before_config jsonb NOT NULL,
 target_config jsonb NOT NULL,
 source_config jsonb NOT NULL,
 before_secret_ref text NOT NULL DEFAULT '',
 remote_id bigint,
 restart_sent_at timestamptz,
 restart_before_process text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN ('PENDING','RUNNING','SUCCEEDED','FAILED','BLOCKED','OBSOLETE')),
 phase text NOT NULL DEFAULT 'QUEUED',
 attempts integer NOT NULL DEFAULT 0,
 next_retry_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 completed_at timestamptz,
 rollback_of uuid REFERENCES reality_transport_apply_jobs(id)
);
CREATE UNIQUE INDEX reality_transport_apply_active ON reality_transport_apply_jobs(panel_id,managed_key)
 WHERE state IN ('PENDING','RUNNING','BLOCKED');
CREATE INDEX reality_transport_apply_queue ON reality_transport_apply_jobs(next_retry_at,created_at)
 WHERE state IN ('PENDING','RUNNING');

-- Admission is serialized with apply requests. Existing queued mutations drain
-- before transport work starts; no new client intent may capture the old flow.
CREATE FUNCTION guard_client_transport_application() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(180061015);
 IF EXISTS(SELECT 1 FROM reality_transport_apply_jobs WHERE panel_id=NEW.panel_id AND state IN('PENDING','RUNNING','BLOCKED')) THEN
  RAISE EXCEPTION 'listener transport application pending' USING ERRCODE='55P03';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER client_transport_application_admission BEFORE INSERT ON client_mutation_jobs
FOR EACH ROW EXECUTE FUNCTION guard_client_transport_application();
