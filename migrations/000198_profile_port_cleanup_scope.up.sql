-- Released desired ports may be reused while only the affected panels wait
-- for confirmed remote cleanup. Active desired and legacy reservations remain.
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
 JOIN config_driver_profiles owner ON owner.id=l.profile_id
 WHERE p.enabled AND p.delete_requested_at IS NULL
 AND (owner.delete_requested_at IS NULL OR NOT (SELECT unified FROM config_control_authority WHERE singleton))
 AND (p.fleet OR l.panel_id=ANY(p.panel_ids))) THEN
  RAISE EXCEPTION 'Port is still owned by an applied or pending listener' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
