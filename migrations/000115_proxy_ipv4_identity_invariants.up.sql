ALTER TABLE account_network_identities
  DROP CONSTRAINT IF EXISTS account_network_identities_exit_ipv4;
ALTER TABLE account_network_identities
  ADD CONSTRAINT account_network_identities_exit_ipv4
  CHECK (exit_ip IS NULL OR family(exit_ip)=4);

ALTER TABLE account_network_identities
  DROP CONSTRAINT IF EXISTS account_network_identities_subnet_canonical;
ALTER TABLE account_network_identities
  ADD CONSTRAINT account_network_identities_subnet_canonical
  CHECK (
    exit_ip IS NULL OR
    subnet_key=(host(network(set_masklen(exit_ip,24)))||'/24')
  );

ALTER TABLE proxies
  DROP CONSTRAINT IF EXISTS proxies_exit_ipv4;
ALTER TABLE proxies
  ADD CONSTRAINT proxies_exit_ipv4
  CHECK (exit_ip IS NULL OR family(exit_ip)=4);

CREATE UNIQUE INDEX IF NOT EXISTS account_network_identity_exit_ip_unique
  ON account_network_identities(exit_ip)
  WHERE exit_ip IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS account_network_identity_ipv4_24_unique
  ON account_network_identities((network(set_masklen(exit_ip,24))))
  WHERE exit_ip IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS account_network_identity_sticky_unique
  ON account_network_identities(sticky_session)
  WHERE sticky_session IS NOT NULL AND sticky_session<>'';