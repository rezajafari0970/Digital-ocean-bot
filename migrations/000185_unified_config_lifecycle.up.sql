-- A single desired-state lifecycle. Existing installations opt in through the
-- durable retirement operation; migration alone never deletes live listeners.
CREATE TABLE config_control_authority (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 unified boolean NOT NULL DEFAULT false,
 reset_job_id uuid REFERENCES panel_cleanup_jobs(id),
 activated_at timestamptz
);
INSERT INTO config_control_authority(singleton) VALUES(true);
ALTER TABLE config_driver_profiles ADD COLUMN delete_requested_at timestamptz,
 ADD COLUMN deleted_at timestamptz,
 ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE config_driver_listeners ADD COLUMN mutation_profile jsonb,
 ADD COLUMN mutation_credential_ref text,
 ADD COLUMN mutation_operation text NOT NULL DEFAULT 'UPSERT' CHECK(mutation_operation IN('UPSERT','DELETE')),
 ADD COLUMN failures integer NOT NULL DEFAULT 0,
 ADD COLUMN native_observed_at timestamptz,
 ADD COLUMN retired_at timestamptz;
CREATE INDEX config_driver_due ON config_driver_listeners(next_attempt_at,attempted_at) WHERE state<>'RETIRED';
-- A sent legacy operation must retain its exact desired membership on recovery.
UPDATE config_driver_listeners l SET mutation_profile=jsonb_build_object(
 'id',p.id,'name',p.name,'spec',l.mutation_spec,'enabled',true,'fleet',p.fleet,
 'panel_ids',p.panel_ids,'classes',p.classes,'clients_per_class',p.clients_per_class,
 'revision',l.mutation_revision,'legacy_key',p.legacy_key),mutation_credential_ref=l.credential_ref
 FROM config_driver_profiles p WHERE p.id=l.profile_id AND l.mutation_state IN('PREPARED','SENT');

CREATE OR REPLACE FUNCTION guard_config_driver_ports() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(628341902732);
 IF EXISTS(SELECT 1 FROM config_driver_profiles p JOIN config_driver_profiles q ON p.id<q.id AND (p.spec->>'port')::integer=(q.spec->>'port')::integer
 WHERE p.enabled AND q.enabled AND p.delete_requested_at IS NULL AND q.delete_requested_at IS NULL AND (p.fleet OR q.fleet OR p.panel_ids&&q.panel_ids)) THEN
  RAISE EXCEPTION 'Profile port is reserved by another profile' USING ERRCODE='23514';
 END IF;
 IF NOT (SELECT unified FROM config_control_authority WHERE singleton) THEN
  IF EXISTS(SELECT 1 FROM config_driver_profiles p CROSS JOIN LATERAL (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') r CROSS JOIN LATERAL jsonb_array_elements_text(r.ports) port WHERE p.enabled AND p.delete_requested_at IS NULL AND (p.spec->>'port')::integer=port::integer) THEN
   RAISE EXCEPTION 'Profile port conflicts with legacy Reality' USING ERRCODE='23514';
  END IF;
  IF EXISTS(SELECT 1 FROM config_driver_profiles p CROSS JOIN trojan_tcp_policy t WHERE p.enabled AND p.delete_requested_at IS NULL AND p.legacy_key IS DISTINCT FROM 'trojan' AND t.enabled AND (p.spec->>'port')::integer=t.port) THEN
   RAISE EXCEPTION 'Profile port conflicts with legacy Trojan' USING ERRCODE='23514';
  END IF;
  IF EXISTS(SELECT 1 FROM config_driver_listeners l CROSS JOIN LATERAL (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') r CROSS JOIN LATERAL jsonb_array_elements_text(r.ports) port WHERE l.state<>'RETIRED' AND l.remote_id IS NOT NULL AND (l.applied_spec->>'port')::integer=port::integer) THEN
   RAISE EXCEPTION 'Reality port is still used by a profile listener' USING ERRCODE='23514';
  END IF;
 END IF;
 IF EXISTS(SELECT 1 FROM config_driver_profiles p JOIN config_driver_listeners l ON l.profile_id<>p.id AND l.state<>'RETIRED'
 AND ((l.remote_id IS NOT NULL AND (p.spec->>'port')::integer=(l.applied_spec->>'port')::integer)
 OR (l.mutation_state IN('PREPARED','SENT') AND (p.spec->>'port')::integer=(l.mutation_spec->>'port')::integer))
 WHERE p.enabled AND p.delete_requested_at IS NULL AND (p.fleet OR l.panel_id=ANY(p.panel_ids))) THEN
  RAISE EXCEPTION 'Port is still owned by an applied or pending listener' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;

ALTER TABLE config_driver_profiles ADD COLUMN request_id uuid UNIQUE;
CREATE TABLE config_driver_client_retirements(
 profile_id uuid NOT NULL,panel_id uuid NOT NULL,client_id text NOT NULL,email text NOT NULL,
 confirmed_absent_at timestamptz,PRIMARY KEY(profile_id,panel_id,client_id),
 FOREIGN KEY(profile_id,panel_id) REFERENCES config_driver_listeners(profile_id,panel_id) ON DELETE CASCADE
);
CREATE FUNCTION guard_legacy_configuration_authority() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.enabled AND EXISTS(SELECT 1 FROM config_control_authority WHERE unified) THEN
  RAISE EXCEPTION 'Legacy creation is retired; use unified configuration profiles' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER legacy_reality_authority BEFORE INSERT OR UPDATE ON reality_config_profiles FOR EACH ROW EXECUTE FUNCTION guard_legacy_configuration_authority();
CREATE TRIGGER legacy_global_authority BEFORE INSERT OR UPDATE ON global_config_policies FOR EACH ROW EXECUTE FUNCTION guard_legacy_configuration_authority();
CREATE TRIGGER legacy_trojan_authority BEFORE INSERT OR UPDATE ON trojan_tcp_policy FOR EACH ROW EXECUTE FUNCTION guard_legacy_configuration_authority();
CREATE TRIGGER legacy_bulk_authority BEFORE INSERT OR UPDATE ON bulk_lifecycle_control FOR EACH ROW EXECUTE FUNCTION guard_legacy_configuration_authority();
