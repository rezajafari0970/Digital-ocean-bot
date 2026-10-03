CREATE TABLE bulk_lifecycle_control (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO bulk_lifecycle_control(singleton) VALUES(true);
CREATE TABLE bulk_lifecycle_scopes (
 panel_id uuid NOT NULL REFERENCES panel_instances(id),
 inbound_id bigint NOT NULL CHECK(inbound_id>0),
 generation_id uuid NOT NULL REFERENCES bulk_user_generations(id),
 enabled boolean NOT NULL DEFAULT false,
 use_global_policy boolean NOT NULL DEFAULT true,
 allow_create boolean NOT NULL DEFAULT false,
 target_users integer NOT NULL DEFAULT 1 CHECK(target_users BETWEEN 0 AND 10000),
 quota_bytes bigint NOT NULL DEFAULT 0 CHECK(quota_bytes>=0),
 lifetime_seconds integer NOT NULL DEFAULT 0 CHECK(lifetime_seconds BETWEEN 0 AND 315360000),
 device_limit integer NOT NULL DEFAULT 0 CHECK(device_limit BETWEEN 0 AND 10000),
 users_per_second integer NOT NULL DEFAULT 1 CHECK(users_per_second BETWEEN 1 AND 100),
 max_batch_size integer NOT NULL DEFAULT 10 CHECK(max_batch_size BETWEEN 1 AND 100),
 remaining_operations integer NOT NULL DEFAULT 0 CHECK(remaining_operations>=0),
 expires_at timestamptz NOT NULL,
 last_observed_at timestamptz,
 active_users integer NOT NULL DEFAULT 0,
 last_error text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(panel_id,inbound_id)
);
CREATE UNIQUE INDEX lifecycle_unresolved_idx ON client_mutation_jobs(panel_id,inbound_id) WHERE payload->>'Lifecycle'='true' AND state IN ('PENDING','RUNNING','FAILED');
CREATE OR REPLACE FUNCTION protect_bulk_payload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.kind IN ('BULK_CREATE','BULK_DELETE') OR OLD.payload->>'Lifecycle'='true') AND (NEW.payload IS DISTINCT FROM OLD.payload OR NEW.kind<>OLD.kind OR NEW.panel_id<>OLD.panel_id OR NEW.inbound_id<>OLD.inbound_id OR NEW.client_id<>OLD.client_id OR NEW.account_id<>OLD.account_id OR NEW.idempotency_key<>OLD.idempotency_key) THEN
  RAISE EXCEPTION 'durable plan identity and payload are immutable';
 END IF;
 RETURN NEW;
END $$;
