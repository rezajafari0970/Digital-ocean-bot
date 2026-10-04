CREATE TABLE reality_config_profiles (
 route_class text PRIMARY KEY CHECK(route_class IN ('DIRECT','RESIDENTIAL')),
 enabled boolean NOT NULL DEFAULT true,
 ports jsonb NOT NULL CHECK(jsonb_typeof(ports)='array' AND jsonb_array_length(ports)>0),
 target_users_per_inbound integer NOT NULL CHECK(target_users_per_inbound BETWEEN 0 AND 10000),
 user_quota_bytes bigint NOT NULL DEFAULT 0 CHECK(user_quota_bytes>=0),
 user_lifetime_seconds integer NOT NULL DEFAULT 0 CHECK(user_lifetime_seconds BETWEEN 0 AND 315360000),
 device_limit integer NOT NULL DEFAULT 0 CHECK(device_limit BETWEEN 0 AND 10000),
 users_per_second integer NOT NULL DEFAULT 1 CHECK(users_per_second BETWEEN 1 AND 100),
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now()
);
-- Preserve the existing allocation and identities. This migration performs no
-- panel mutation, gate rearm, quota change or client deletion.
INSERT INTO reality_config_profiles(route_class,enabled,ports,target_users_per_inbound,user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second)
SELECT cls,enabled,ports,
 CASE WHEN generate_direct AND generate_residential THEN
   CASE WHEN cls='DIRECT' THEN target_users_per_inbound/2 ELSE target_users_per_inbound-target_users_per_inbound/2 END
 ELSE target_users_per_inbound END,
 user_quota_bytes,user_lifetime_seconds,device_limit,users_per_second
FROM global_config_policies CROSS JOIN (VALUES('DIRECT'),('RESIDENTIAL')) c(cls)
WHERE policy_key='reality' AND ((cls='DIRECT' AND generate_direct) OR (cls='RESIDENTIAL' AND generate_residential));

ALTER TABLE bulk_user_ownership ADD COLUMN route_class text NOT NULL DEFAULT '' CHECK(route_class IN ('','DIRECT','RESIDENTIAL'));
UPDATE bulk_user_ownership o SET route_class=r.route_class
FROM bulk_user_generations g JOIN panel_client_routes r ON r.panel_id=g.panel_id
WHERE o.generation_id=g.id AND o.client_id=r.client_id AND o.email=r.email;
CREATE FUNCTION protect_owned_route_class() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.route_class<>'' AND NEW.route_class<>OLD.route_class THEN
  RAISE EXCEPTION 'owned client route class is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_owned_route_class BEFORE UPDATE OF route_class ON bulk_user_ownership FOR EACH ROW EXECUTE FUNCTION protect_owned_route_class();

ALTER TABLE bulk_user_rate_state ADD COLUMN route_class text NOT NULL DEFAULT '' CHECK(route_class IN ('','DIRECT','RESIDENTIAL'));
ALTER TABLE bulk_user_rate_state DROP CONSTRAINT bulk_user_rate_state_pkey;
ALTER TABLE bulk_user_rate_state ADD PRIMARY KEY(panel_id,inbound_id,route_class);

-- Compatibility summary for shared listener provisioning and the existing
-- explicit cleanup/restore master switch. Per-client settings live above.
CREATE FUNCTION summarize_reality_profiles() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ps jsonb; target integer;
BEGIN
 SELECT COALESCE(jsonb_agg(port ORDER BY port),'[]'::jsonb) INTO ps FROM (
  SELECT DISTINCT v::integer port FROM reality_config_profiles p CROSS JOIN LATERAL jsonb_array_elements_text(p.ports) v
  WHERE p.enabled OR NOT EXISTS(SELECT 1 FROM reality_config_profiles WHERE enabled)
 ) q;
 SELECT COALESCE(max(n),0) INTO target FROM (
  SELECT v,sum(target_users_per_inbound) n FROM reality_config_profiles p CROSS JOIN LATERAL jsonb_array_elements_text(p.ports) v WHERE enabled GROUP BY v
 ) q;
 IF target>10000 THEN RAISE EXCEPTION 'combined profile target exceeds inbound safety limit'; END IF;
 UPDATE global_config_policies SET
  enabled=EXISTS(SELECT 1 FROM reality_config_profiles WHERE enabled),
  ports=ps,target_users_per_inbound=target,
  generate_direct=EXISTS(SELECT 1 FROM reality_config_profiles WHERE route_class='DIRECT'),
  generate_residential=EXISTS(SELECT 1 FROM reality_config_profiles WHERE route_class='RESIDENTIAL'),
  revision=revision+1,updated_at=now()
 WHERE policy_key='reality';
 RETURN NULL;
END $$;
CREATE TRIGGER reality_profiles_summary AFTER INSERT OR UPDATE OR DELETE ON reality_config_profiles FOR EACH STATEMENT EXECUTE FUNCTION summarize_reality_profiles();
