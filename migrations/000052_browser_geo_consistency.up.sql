ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS expected_country TEXT;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS expected_timezone TEXT;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS expected_locale TEXT;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS timezone_match BOOLEAN;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS locale_match BOOLEAN;

ALTER TABLE account_browser_identities
ADD COLUMN IF NOT EXISTS geo_consistency TEXT NOT NULL DEFAULT 'unknown';
