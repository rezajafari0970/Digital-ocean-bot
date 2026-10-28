-- Preserve existing authorization flows as AUTO; new requests bind the route
-- constraint before any provider authorization call is dispatched.
ALTER TABLE provider_authorizations ADD COLUMN proxy_country text NOT NULL DEFAULT 'AUTO'
 CHECK(proxy_country='AUTO' OR proxy_country ~ '^[A-Z]{2}$');
