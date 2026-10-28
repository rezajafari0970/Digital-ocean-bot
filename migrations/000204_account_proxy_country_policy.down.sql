-- Never discard explicit operator intent to make an old binary start.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM account_proxy_country_policies WHERE countries<>'["AUTO"]'::jsonb) THEN
  RAISE EXCEPTION 'explicit country policies must be reset through a compatible version before downgrade';
 END IF;
 IF EXISTS(SELECT 1 FROM proxies WHERE adapter NOT IN ('generic','suffix-session')) THEN
  RAISE EXCEPTION 'registered proxy drivers require a compatible version';
 END IF;
END $$;
DROP TRIGGER bump_account_country_parameters_epoch ON account_network_identities;
DROP FUNCTION bump_account_country_parameters_epoch();
DROP TRIGGER invalidate_country_secret_change ON secrets;
DROP TRIGGER invalidate_country_proxy_change ON proxies;
DROP FUNCTION invalidate_country_proxy_change();
DROP TRIGGER invalidate_country_profile_change ON network_profiles;
DROP FUNCTION invalidate_country_profile_change();
DROP TRIGGER invalidate_country_pool_change ON account_proxy_pool;
DROP FUNCTION invalidate_country_pool_change();
DROP FUNCTION invalidate_explicit_country_selection(uuid);
DROP TRIGGER guard_explicit_proxy_country_writer ON account_network_identities;
DROP FUNCTION guard_explicit_proxy_country_writer();
ALTER TABLE account_network_identities DROP COLUMN selected_target_country;
DROP TABLE account_proxy_selection;
DROP TABLE account_proxy_country_policies;
ALTER TABLE proxies DROP CONSTRAINT proxies_adapter_check;
ALTER TABLE proxies ADD CONSTRAINT proxies_adapter_check CHECK(adapter IN ('generic','suffix-session'));
