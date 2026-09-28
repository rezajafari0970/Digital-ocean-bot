ALTER TABLE panel_inbound_policies
 ADD COLUMN user_quota_bytes bigint NOT NULL DEFAULT 0 CHECK(user_quota_bytes >= 0),
 ADD COLUMN user_lifetime_seconds integer NOT NULL DEFAULT 0 CHECK(user_lifetime_seconds >= 0),
 ADD COLUMN device_limit integer NOT NULL DEFAULT 0 CHECK(device_limit >= 0),
 ADD COLUMN bulk_user_count integer NOT NULL DEFAULT 0 CHECK(bulk_user_count >= 0),
 ADD COLUMN users_per_second integer NOT NULL DEFAULT 1 CHECK(users_per_second >= 1),
 ADD COLUMN sni_selection_mode text NOT NULL DEFAULT 'scored'
   CHECK(sni_selection_mode IN ('scored','manual')),
 ADD COLUMN manual_snis jsonb NOT NULL DEFAULT '[]'::jsonb;
