ALTER TABLE global_config_policies ADD COLUMN generate_residential boolean NOT NULL DEFAULT true;
ALTER TABLE global_config_policies ADD COLUMN generate_direct boolean NOT NULL DEFAULT false;
CREATE TABLE residential_routing_control(
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 fleet boolean NOT NULL DEFAULT false,
 panel_ids uuid[] NOT NULL DEFAULT '{}',
 revision bigint NOT NULL DEFAULT 1
);
INSERT INTO residential_routing_control(singleton) VALUES(true);
CREATE TABLE panel_routing_state(
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 revision bigint NOT NULL DEFAULT 0,
 plan_hash text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'PENDING' CHECK(state IN ('PENDING','APPLYING','APPLIED','FAILED')),
 selected_proxy_id uuid REFERENCES proxies(id) ON DELETE SET NULL,
 configured_count integer NOT NULL DEFAULT 0,
 healthy_count integer NOT NULL DEFAULT 0,
 next_check_at timestamptz NOT NULL DEFAULT now(),
 verified_at timestamptz,
 last_error text NOT NULL DEFAULT ''
);
CREATE TABLE panel_client_routes(
 panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 client_id text NOT NULL,
 email text NOT NULL,
 route_class text NOT NULL CHECK(route_class IN ('RESIDENTIAL','DIRECT')),
 effective_class text NOT NULL CHECK(effective_class IN ('RESIDENTIAL','DIRECT','BLOCKED')),
 revision bigint NOT NULL,
 PRIMARY KEY(panel_id,client_id),
 UNIQUE(panel_id,email)
);
ALTER TABLE output_config_snapshots ADD COLUMN client_id text NOT NULL DEFAULT '';
ALTER TABLE output_share_tokens ADD COLUMN route_class text NOT NULL DEFAULT 'ALL' CHECK(route_class IN ('ALL','RESIDENTIAL','DIRECT'));

CREATE FUNCTION bump_residential_routing_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='secrets' THEN
  IF TG_OP='DELETE' THEN
   IF OLD.proxy_id IS NULL THEN RETURN NULL; END IF;
  ELSE
   IF NEW.proxy_id IS NULL THEN RETURN NULL; END IF;
  END IF;
 END IF;
 IF TG_TABLE_NAME='proxies' THEN
  IF NOT EXISTS(SELECT 1 FROM residential_proxies WHERE proxy_id=NEW.id) THEN RETURN NULL; END IF;
 END IF;
 UPDATE residential_routing_control SET revision=revision+1 WHERE singleton;
 UPDATE panel_routing_state SET next_check_at=now();
 RETURN NULL;
END $$;
CREATE TRIGGER residential_route_change AFTER INSERT OR UPDATE OR DELETE ON residential_proxies FOR EACH STATEMENT EXECUTE FUNCTION bump_residential_routing_revision();
CREATE TRIGGER residential_proxy_change AFTER UPDATE ON proxies FOR EACH ROW WHEN
 ((OLD.type,OLD.host,OLD.port,OLD.username,OLD.secret_ref,OLD.status) IS DISTINCT FROM (NEW.type,NEW.host,NEW.port,NEW.username,NEW.secret_ref,NEW.status))
 EXECUTE FUNCTION bump_residential_routing_revision();
CREATE TRIGGER residential_secret_change AFTER INSERT OR UPDATE OR DELETE ON secrets FOR EACH ROW EXECUTE FUNCTION bump_residential_routing_revision();
CREATE TRIGGER residential_policy_change AFTER UPDATE ON global_config_policies FOR EACH ROW WHEN
 ((OLD.generate_residential,OLD.generate_direct) IS DISTINCT FROM (NEW.generate_residential,NEW.generate_direct))
 EXECUTE FUNCTION bump_residential_routing_revision();
CREATE FUNCTION wake_panel_routing() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE panel_routing_state SET next_check_at=now() WHERE panel_id=NEW.panel_id;
 RETURN NULL;
END $$;
CREATE TRIGGER client_routing_refresh AFTER UPDATE ON client_mutation_jobs FOR EACH ROW WHEN (NEW.state='SUCCEEDED' AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION wake_panel_routing();

CREATE TRIGGER residential_control_change AFTER UPDATE ON residential_routing_control FOR EACH ROW WHEN
 ((OLD.enabled,OLD.fleet,OLD.panel_ids) IS DISTINCT FROM (NEW.enabled,NEW.fleet,NEW.panel_ids))
 EXECUTE FUNCTION bump_residential_routing_revision();
