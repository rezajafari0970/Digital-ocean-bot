ALTER TABLE accounts ADD COLUMN IF NOT EXISTS preferred_images JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS server_lifetime_min_seconds INTEGER;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS server_lifetime_max_seconds INTEGER;
UPDATE accounts SET preferred_images=CASE WHEN COALESCE(preferred_image,'')<>'' THEN jsonb_build_array(preferred_image) ELSE '[]'::jsonb END WHERE preferred_images='[]'::jsonb;
UPDATE accounts SET server_lifetime_min_seconds=server_lifetime_seconds,server_lifetime_max_seconds=server_lifetime_seconds WHERE server_lifetime_min_seconds IS NULL OR server_lifetime_max_seconds IS NULL;
