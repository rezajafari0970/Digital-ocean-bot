ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS geo_consistency;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS locale_match;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS timezone_match;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS expected_locale;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS expected_timezone;

ALTER TABLE account_browser_identities
DROP COLUMN IF EXISTS expected_country;
