ALTER TABLE account_network_identities
    DROP COLUMN IF EXISTS country_code;

ALTER TABLE proxies
    DROP COLUMN IF EXISTS country_code;
