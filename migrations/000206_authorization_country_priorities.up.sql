ALTER TABLE provider_authorizations
 ADD COLUMN proxy_countries jsonb CHECK(proxy_countries IS NULL OR (jsonb_typeof(proxy_countries)='array' AND jsonb_array_length(proxy_countries) BETWEEN 1 AND 3)),
 ADD COLUMN proxy_ids jsonb CHECK(proxy_ids IS NULL OR (jsonb_typeof(proxy_ids)='array' AND jsonb_array_length(proxy_ids)<=3)),
 ADD COLUMN route_proxy_id text,
 ADD COLUMN route_country text CHECK(route_country IS NULL OR route_country='AUTO' OR route_country ~ '^[A-Z]{2}$');
-- NULL preserves the original scalar binding of pre-existing flows. Never
-- rewrite an in-flight provider intent or assume a different route was used.
