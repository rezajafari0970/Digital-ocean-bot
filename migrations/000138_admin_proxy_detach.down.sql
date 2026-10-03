ALTER TABLE network_profiles ADD CONSTRAINT network_profiles_check CHECK(mode <> 'proxy_required' OR proxy_id IS NOT NULL);
