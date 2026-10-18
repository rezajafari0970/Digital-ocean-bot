DROP TRIGGER legacy_reality_authority ON reality_config_profiles;
DROP TRIGGER legacy_global_authority ON global_config_policies;
DROP TRIGGER legacy_trojan_authority ON trojan_tcp_policy;
DROP TRIGGER legacy_bulk_authority ON bulk_lifecycle_control;
DROP FUNCTION guard_legacy_configuration_authority();
DROP TABLE config_driver_client_retirements;
CREATE OR REPLACE FUNCTION guard_config_driver_ports() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(628341902732);
 IF EXISTS(SELECT 1 FROM config_driver_profiles p JOIN config_driver_profiles q ON p.id<q.id AND (p.spec->>'port')::integer=(q.spec->>'port')::integer WHERE p.enabled AND q.enabled AND (p.fleet OR q.fleet OR p.panel_ids&&q.panel_ids)) THEN
  RAISE EXCEPTION 'Profile port is reserved by another enabled profile' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM config_driver_profiles p CROSS JOIN LATERAL (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') r CROSS JOIN LATERAL jsonb_array_elements_text(r.ports) port WHERE p.enabled AND (p.spec->>'port')::integer=port::integer) THEN
  RAISE EXCEPTION 'Profile port conflicts with Reality' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM config_driver_profiles p CROSS JOIN trojan_tcp_policy t WHERE p.enabled AND p.legacy_key IS DISTINCT FROM 'trojan' AND t.enabled AND (p.spec->>'port')::integer=t.port) THEN
  RAISE EXCEPTION 'Profile port conflicts with legacy Trojan' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM config_driver_profiles p JOIN config_driver_listeners l ON l.profile_id<>p.id AND l.state<>'RETIRED' AND l.remote_id IS NOT NULL AND (p.spec->>'port')::integer=(l.applied_spec->>'port')::integer WHERE p.enabled AND (p.fleet OR l.panel_id=ANY(p.panel_ids))) THEN
  RAISE EXCEPTION 'Profile port is still used by an applied listener' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM config_driver_listeners l CROSS JOIN LATERAL (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') r CROSS JOIN LATERAL jsonb_array_elements_text(r.ports) port WHERE l.state<>'RETIRED' AND l.remote_id IS NOT NULL AND (l.applied_spec->>'port')::integer=port::integer) THEN
  RAISE EXCEPTION 'Reality port is still used by a profile listener' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
DROP INDEX config_driver_due;
ALTER TABLE config_driver_listeners DROP COLUMN mutation_profile,DROP COLUMN mutation_credential_ref,DROP COLUMN mutation_operation,DROP COLUMN failures,DROP COLUMN native_observed_at,DROP COLUMN retired_at;
ALTER TABLE config_driver_profiles DROP COLUMN delete_requested_at,DROP COLUMN deleted_at,DROP COLUMN created_at,DROP COLUMN request_id;
DROP TABLE config_control_authority;
