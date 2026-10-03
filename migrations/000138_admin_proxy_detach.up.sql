-- A deleted proxy leaves proxy_required accounts blocked until a replacement is selected.
-- network admission rejects NULL/missing/unhealthy proxies; it must never change mode to direct.
ALTER TABLE network_profiles DROP CONSTRAINT network_profiles_check;
