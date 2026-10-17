-- Trojan is independently opt-in. No existing listener, client, policy or port is changed.
CREATE TABLE trojan_tcp_policy (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 port integer NOT NULL DEFAULT 8443 CHECK(port BETWEEN 1024 AND 65535 AND port<>18080),
 direct_enabled boolean NOT NULL DEFAULT true,
 residential_enabled boolean NOT NULL DEFAULT true,
 fleet boolean NOT NULL DEFAULT false,
 panel_ids uuid[] NOT NULL DEFAULT '{}',
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO trojan_tcp_policy DEFAULT VALUES;
CREATE TABLE trojan_tcp_listeners (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 port integer NOT NULL CHECK(port BETWEEN 1024 AND 65535),
 remote_id bigint,
 managed_key text NOT NULL,
 credential_ref text NOT NULL,
 policy_revision bigint NOT NULL,
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN('PENDING','TLS_READY','VERIFIED','BLOCKED','RETIRED')),
 stage text NOT NULL DEFAULT 'prepare',
 error_code text NOT NULL DEFAULT '',
 certificate_not_after timestamptz,
 certificate_sha256 text NOT NULL DEFAULT '',
 verified_at timestamptz,
 verify_attempted_at timestamptz,
 attempted_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE trojan_tcp_clients (
 panel_id uuid NOT NULL REFERENCES trojan_tcp_listeners(panel_id) ON DELETE CASCADE,
 route_class text NOT NULL CHECK(route_class IN('DIRECT','RESIDENTIAL')),
 client_id text NOT NULL,
 email text NOT NULL,
 PRIMARY KEY(panel_id,route_class),
 UNIQUE(panel_id,client_id), UNIQUE(panel_id,email)
);
-- Reserve an enabled Trojan policy against all saved Reality ports. Existing Trojan
-- listeners also retain their ports until a verified cleanup retires them.
CREATE FUNCTION guard_trojan_tcp_ports() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tp integer; conflicts boolean;
BEGIN
 PERFORM pg_advisory_xact_lock(628341902732);
 SELECT port INTO tp FROM trojan_tcp_policy WHERE singleton AND enabled;
 SELECT EXISTS(SELECT 1 FROM (SELECT ports FROM reality_config_profiles UNION ALL SELECT ports FROM global_config_policies WHERE policy_key='reality') r CROSS JOIN LATERAL jsonb_array_elements_text(r.ports) p
  WHERE p::integer=tp OR EXISTS(SELECT 1 FROM trojan_tcp_listeners t WHERE t.port=p::integer AND t.state<>'RETIRED')) INTO conflicts;
 IF conflicts THEN RAISE EXCEPTION 'Trojan port conflicts with reserved Reality port' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER trojan_tcp_port_guard AFTER INSERT OR UPDATE ON trojan_tcp_policy FOR EACH STATEMENT EXECUTE FUNCTION guard_trojan_tcp_ports();
CREATE TRIGGER reality_trojan_port_guard AFTER INSERT OR UPDATE ON reality_config_profiles FOR EACH STATEMENT EXECUTE FUNCTION guard_trojan_tcp_ports();

CREATE TRIGGER global_reality_trojan_port_guard AFTER INSERT OR UPDATE ON global_config_policies FOR EACH STATEMENT EXECUTE FUNCTION guard_trojan_tcp_ports();
