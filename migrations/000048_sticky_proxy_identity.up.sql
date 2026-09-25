ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS sticky_session TEXT;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS preferred_country_code TEXT;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS preferred_country TEXT;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS rotation_started_at TIMESTAMPTZ;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS fallback_active BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS last_health_at TIMESTAMPTZ;
ALTER TABLE account_network_identities ADD COLUMN IF NOT EXISTS last_health_ok BOOLEAN;
UPDATE account_network_identities i SET sticky_session=COALESCE(sticky_session,replace(gen_random_uuid()::text,'-','')), preferred_country=COALESCE(NULLIF(preferred_country,''),NULLIF(country,''));
UPDATE account_network_identities SET preferred_country_code='ve' WHERE preferred_country_code IS NULL AND lower(preferred_country)='venezuela';
