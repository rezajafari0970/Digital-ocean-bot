-- No existing account/session/learned country is modified by this migration.
-- An absent policy is exactly legacy AUTO.
CREATE TABLE account_proxy_country_policies (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 countries jsonb NOT NULL DEFAULT '["AUTO"]'::jsonb
  CHECK(jsonb_typeof(countries)='array' AND jsonb_array_length(countries) BETWEEN 1 AND 251),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE account_proxy_selection (
 account_id uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 fence bigint NOT NULL DEFAULT 0,
 work_revision bigint NOT NULL DEFAULT 0,
 applied_revision bigint NOT NULL DEFAULT 0,
 scope_hash text NOT NULL DEFAULT '',
 checkpoint jsonb NOT NULL DEFAULT '{}'::jsonb,
 candidate jsonb,
 active_stage text NOT NULL DEFAULT '',
 legacy_lease_until timestamptz,
 status text NOT NULL DEFAULT 'pending',
 detail text NOT NULL DEFAULT '',
 checked_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE account_network_identities ADD COLUMN selected_target_country text;
ALTER TABLE account_network_identities ADD CONSTRAINT account_selected_target_country_valid
 CHECK(selected_target_country IS NULL OR selected_target_country ~ '^[A-Z]{2}$');

-- Registered modules validate driver names at the API boundary. Unknown modules
-- are not eligible for ordered selection. No vendor is named by this constraint.
ALTER TABLE proxies DROP CONSTRAINT proxies_adapter_check;
ALTER TABLE proxies ADD CONSTRAINT proxies_adapter_check CHECK(adapter ~ '^[a-z][a-z0-9-]{0,63}$');

-- An old keeper must not overwrite a newly explicit policy during a rolling
-- update or unsupported binary rollback. The ordered writer supplies a fence
-- scoped to this account; legacy work gets only a bounded AUTO compatibility lease.
CREATE FUNCTION guard_explicit_proxy_country_writer() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE policy_revision bigint; f bigint; legacy_allowed boolean; changed boolean;
BEGIN
 SELECT revision INTO policy_revision FROM account_proxy_country_policies
 WHERE account_id=NEW.account_id AND countries<>'["AUTO"]'::jsonb;
 IF policy_revision IS NULL THEN RETURN NEW; END IF;
 SELECT fence,(active_stage='AUTO' AND work_revision=policy_revision AND legacy_lease_until>clock_timestamp())
 INTO f,legacy_allowed FROM account_proxy_selection WHERE account_id=NEW.account_id;
 IF current_setting('dob.country_writer',true)=NEW.account_id::text||':'||f::text
    OR COALESCE(legacy_allowed,false) THEN RETURN NEW; END IF;
 IF TG_OP='INSERT' THEN
  changed=NEW.exit_ip IS NOT NULL OR NEW.sticky_session IS NOT NULL OR NEW.sticky_port IS NOT NULL OR NEW.selected_target_country IS NOT NULL;
 ELSE
  changed=(OLD.exit_ip,OLD.sticky_session,OLD.sticky_port,OLD.selected_target_country,OLD.preferred_country_code,OLD.preferred_country,OLD.fallback_active,OLD.country_code,OLD.country,OLD.timezone,OLD.locale,OLD.asn)
       IS DISTINCT FROM (NEW.exit_ip,NEW.sticky_session,NEW.sticky_port,NEW.selected_target_country,NEW.preferred_country_code,NEW.preferred_country,NEW.fallback_active,NEW.country_code,NEW.country,NEW.timezone,NEW.locale,NEW.asn);
 END IF;
 IF NEW.last_health_ok OR changed THEN
  RAISE EXCEPTION 'explicit country policy requires a current compatible writer' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guard_explicit_proxy_country_writer BEFORE INSERT OR UPDATE
 ON account_network_identities FOR EACH ROW EXECUTE FUNCTION guard_explicit_proxy_country_writer();

CREATE FUNCTION invalidate_explicit_country_selection(account uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM account_proxy_country_policies WHERE account_id=account AND countries<>'["AUTO"]'::jsonb) THEN RETURN; END IF;
 UPDATE account_proxy_selection SET fence=fence+1,applied_revision=0,legacy_lease_until=NULL,
 checkpoint='{}',candidate=NULL,scope_hash='',status='pending',detail='Route configuration changed; verification pending',updated_at=now()
 WHERE account_id=account;
 UPDATE account_network_identities SET last_health_ok=false WHERE account_id=account;
END $$;

CREATE FUNCTION invalidate_country_pool_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND (OLD.proxy_id,OLD.priority,OLD.enabled) IS NOT DISTINCT FROM (NEW.proxy_id,NEW.priority,NEW.enabled) THEN RETURN NEW; END IF;
 IF TG_OP='DELETE' THEN PERFORM invalidate_explicit_country_selection(OLD.account_id);RETURN OLD; END IF;
 PERFORM invalidate_explicit_country_selection(NEW.account_id);RETURN NEW;
END $$;
CREATE TRIGGER invalidate_country_pool_change AFTER INSERT OR UPDATE OR DELETE ON account_proxy_pool
 FOR EACH ROW EXECUTE FUNCTION invalidate_country_pool_change();

CREATE FUNCTION invalidate_country_profile_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE f bigint;
BEGIN
 IF (OLD.mode,OLD.proxy_id) IS NOT DISTINCT FROM (NEW.mode,NEW.proxy_id) THEN RETURN NEW; END IF;
 SELECT fence INTO f FROM account_proxy_selection WHERE account_id=NEW.account_id;
 -- Fenced runtime adoption publishes its matching proof in this same transaction.
 IF current_setting('dob.country_writer',true)=NEW.account_id::text||':'||f::text OR EXISTS(
 SELECT 1 FROM account_proxy_selection s JOIN account_proxy_country_policies p USING(account_id)
 WHERE s.account_id=NEW.account_id AND s.active_stage='AUTO' AND s.work_revision=p.revision AND s.legacy_lease_until>clock_timestamp()) THEN RETURN NEW; END IF;
 PERFORM invalidate_explicit_country_selection(NEW.account_id);RETURN NEW;
END $$;
CREATE TRIGGER invalidate_country_profile_change AFTER UPDATE OF mode,proxy_id ON network_profiles
 FOR EACH ROW EXECUTE FUNCTION invalidate_country_profile_change();

CREATE FUNCTION invalidate_country_proxy_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE account uuid; proxy uuid;
BEGIN
 IF TG_TABLE_NAME='proxies' THEN
  IF (OLD.type,OLD.host,OLD.port,OLD.username,OLD.adapter,OLD.secret_ref) IS NOT DISTINCT FROM
     (NEW.type,NEW.host,NEW.port,NEW.username,NEW.adapter,NEW.secret_ref) THEN RETURN NEW; END IF;
  proxy=NEW.id;
 ELSE
  IF TG_OP='DELETE' THEN proxy=OLD.proxy_id; ELSE proxy=NEW.proxy_id; END IF;
 END IF;
 IF proxy IS NOT NULL THEN
  FOR account IN SELECT account_id FROM account_proxy_pool WHERE proxy_id=proxy AND enabled
     UNION SELECT account_id FROM network_profiles WHERE proxy_id=proxy ORDER BY 1 LOOP
   PERFORM invalidate_explicit_country_selection(account);
  END LOOP;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;RETURN NEW;
END $$;
CREATE TRIGGER invalidate_country_proxy_change AFTER UPDATE OF type,host,port,username,adapter,secret_ref ON proxies
 FOR EACH ROW EXECUTE FUNCTION invalidate_country_proxy_change();
CREATE TRIGGER invalidate_country_secret_change AFTER INSERT OR UPDATE OR DELETE ON secrets
 FOR EACH ROW EXECUTE FUNCTION invalidate_country_proxy_change();

CREATE FUNCTION bump_account_country_parameters_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.sticky_port,OLD.selected_target_country,OLD.preferred_country_code,OLD.fallback_active) IS DISTINCT FROM (NEW.sticky_port,NEW.selected_target_country,NEW.preferred_country_code,NEW.fallback_active) THEN
  INSERT INTO account_transport_state(account_id,provider,transport_epoch,active_proxy_id,transition_reason)
  SELECT NEW.account_id,a.provider,1,n.proxy_id,'country-route-parameters-change' FROM accounts a
   LEFT JOIN network_profiles n ON n.account_id=a.id WHERE a.id=NEW.account_id
  ON CONFLICT(account_id) DO UPDATE SET transport_epoch=account_transport_state.transport_epoch+1,
   active_proxy_id=EXCLUDED.active_proxy_id,transition_reason=EXCLUDED.transition_reason,updated_at=now();
 END IF;RETURN NEW;
END $$;
CREATE TRIGGER bump_account_country_parameters_epoch AFTER UPDATE OF sticky_port,selected_target_country,preferred_country_code,fallback_active
 ON account_network_identities FOR EACH ROW EXECUTE FUNCTION bump_account_country_parameters_epoch();
