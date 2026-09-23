-- Prevent duplicate provider accounts while allowing legacy NULL identities.
CREATE UNIQUE INDEX IF NOT EXISTS accounts_provider_external_id_uidx
ON accounts(provider, external_id)
WHERE external_id IS NOT NULL;
