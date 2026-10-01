ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS country_code TEXT;

ALTER TABLE account_network_identities
    ADD COLUMN IF NOT EXISTS country_code TEXT;
