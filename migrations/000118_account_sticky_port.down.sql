DROP INDEX IF EXISTS account_network_identity_sticky_port_unique;
ALTER TABLE account_network_identities
  DROP CONSTRAINT IF EXISTS account_network_identities_sticky_port_range;
ALTER TABLE account_network_identities
  DROP COLUMN IF EXISTS sticky_port;