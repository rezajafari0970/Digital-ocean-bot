ALTER TABLE panel_inbound_policies
 DROP COLUMN IF EXISTS manual_snis,
 DROP COLUMN IF EXISTS sni_selection_mode,
 DROP COLUMN IF EXISTS users_per_second,
 DROP COLUMN IF EXISTS bulk_user_count,
 DROP COLUMN IF EXISTS device_limit,
 DROP COLUMN IF EXISTS user_lifetime_seconds,
 DROP COLUMN IF EXISTS user_quota_bytes;
