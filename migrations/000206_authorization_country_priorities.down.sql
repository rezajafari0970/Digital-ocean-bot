DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM provider_authorizations WHERE jsonb_array_length(proxy_countries)>1 OR jsonb_array_length(proxy_ids)>1 OR route_proxy_id IS DISTINCT FROM proxy_id AND route_proxy_id IS NOT NULL OR route_country IS DISTINCT FROM proxy_country AND route_country IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot erase an authorization priority or selected route';
 END IF;
END $$;
ALTER TABLE provider_authorizations DROP COLUMN route_country, DROP COLUMN route_proxy_id, DROP COLUMN proxy_ids, DROP COLUMN proxy_countries;
