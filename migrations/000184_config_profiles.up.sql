-- Independent desired profiles; existing Reality ownership/pins are untouched.
CREATE TABLE config_driver_profiles (
 id uuid PRIMARY KEY,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 spec jsonb NOT NULL CHECK(jsonb_typeof(spec)='object'),
 enabled boolean NOT NULL DEFAULT false,
 fleet boolean NOT NULL DEFAULT false,
 panel_ids uuid[] NOT NULL DEFAULT '{}',
 classes text[] NOT NULL CHECK(cardinality(classes) BETWEEN 1 AND 2 AND classes <@ ARRAY['DIRECT','RESIDENTIAL']::text[]),
 clients_per_class integer NOT NULL DEFAULT 1 CHECK(clients_per_class BETWEEN 1 AND 100),
 revision bigint NOT NULL DEFAULT 1,
 legacy_key text UNIQUE,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE config_driver_listeners (
 profile_id uuid NOT NULL REFERENCES config_driver_profiles(id),
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 managed_key text NOT NULL,
 remote_id bigint,
 credential_ref text NOT NULL,
 applied_spec jsonb NOT NULL,
 applied_revision bigint NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','APPLYING','VERIFIED','BLOCKED','RETIRED')),
 stage text NOT NULL DEFAULT 'prepare',
 error_code text NOT NULL DEFAULT '',
 mutation_state text NOT NULL DEFAULT '' CHECK(mutation_state IN('','PREPARED','SENT','CONFIRMED')),
 mutation_spec jsonb,
 mutation_revision bigint,
 certificate_not_after timestamptz,
 verified_at timestamptz,
 attempted_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(profile_id,panel_id),
 UNIQUE(panel_id,managed_key)
);
CREATE UNIQUE INDEX config_driver_native_identity ON config_driver_listeners(panel_id,remote_id) WHERE remote_id IS NOT NULL AND state<>'RETIRED';
CREATE TABLE config_driver_clients (
 profile_id uuid NOT NULL,
 panel_id uuid NOT NULL,
 client_id text NOT NULL,
 email text NOT NULL,
 route_class text NOT NULL CHECK(route_class IN('DIRECT','RESIDENTIAL')),
 PRIMARY KEY(panel_id,client_id),
 UNIQUE(panel_id,email),
 FOREIGN KEY(profile_id,panel_id) REFERENCES config_driver_listeners(profile_id,panel_id) ON DELETE CASCADE
);
CREATE TABLE config_driver_events (
 id bigserial PRIMARY KEY,
 profile_id uuid NOT NULL REFERENCES config_driver_profiles(id),
 panel_id uuid,
 revision bigint NOT NULL,
 stage text NOT NULL,
 code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
-- Adopt only the already-owned Trojan policy; passwords stay in the secret store.
INSERT INTO config_driver_profiles(id,name,spec,enabled,fleet,panel_ids,classes,revision,legacy_key)
 SELECT '00000000-0000-4000-8000-000000000001','Trojan',
 jsonb_build_object('protocol','trojan','network','tcp','security','tls','port',port),
 enabled,fleet,panel_ids,array_remove(ARRAY[CASE WHEN direct_enabled THEN 'DIRECT' END,CASE WHEN residential_enabled THEN 'RESIDENTIAL' END],NULL),
 revision,'trojan'
 FROM trojan_tcp_policy WHERE (enabled OR EXISTS(SELECT 1 FROM trojan_tcp_listeners)) AND (direct_enabled OR residential_enabled);
INSERT INTO config_driver_listeners(profile_id,panel_id,managed_key,remote_id,credential_ref,applied_spec,applied_revision,state,stage,error_code,certificate_not_after,verified_at)
 SELECT p.id,t.panel_id,t.managed_key,t.remote_id,t.credential_ref,p.spec,t.policy_revision,
 CASE WHEN t.state='TLS_READY' THEN 'PENDING' ELSE t.state END,t.stage,t.error_code,t.certificate_not_after,t.verified_at
 FROM trojan_tcp_listeners t CROSS JOIN config_driver_profiles p WHERE p.legacy_key='trojan';
INSERT INTO config_driver_clients(profile_id,panel_id,client_id,email,route_class)
 SELECT p.id,c.panel_id,c.client_id,c.email,c.route_class FROM trojan_tcp_clients c CROSS JOIN config_driver_profiles p WHERE p.legacy_key='trojan';
-- One authority after adoption. Legacy records remain as migration evidence.
UPDATE trojan_tcp_policy SET enabled=false WHERE EXISTS(SELECT 1 FROM config_driver_profiles WHERE legacy_key='trojan');
-- All creation APIs serialize port reservations with the existing admission lock.
CREATE FUNCTION guard_config_driver_ports() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER config_driver_port_guard AFTER INSERT OR UPDATE ON config_driver_profiles FOR EACH STATEMENT EXECUTE FUNCTION guard_config_driver_ports();
CREATE TRIGGER reality_driver_port_guard AFTER INSERT OR UPDATE ON reality_config_profiles FOR EACH STATEMENT EXECUTE FUNCTION guard_config_driver_ports();
CREATE TRIGGER global_driver_port_guard AFTER INSERT OR UPDATE ON global_config_policies FOR EACH STATEMENT EXECUTE FUNCTION guard_config_driver_ports();
CREATE TRIGGER trojan_driver_port_guard AFTER INSERT OR UPDATE ON trojan_tcp_policy FOR EACH STATEMENT EXECUTE FUNCTION guard_config_driver_ports();

-- Bind every published URI to the desired revision that generated it.
ALTER TABLE output_config_snapshots ADD COLUMN config_profile_id uuid, ADD COLUMN config_profile_revision bigint;
UPDATE output_config_snapshots o SET config_profile_id=dc.profile_id,config_profile_revision=dp.revision
 FROM config_driver_clients dc JOIN config_driver_profiles dp ON dp.id=dc.profile_id
 WHERE o.panel_id=dc.panel_id AND o.client_id=dc.client_id AND o.uri LIKE 'trojan://%';
