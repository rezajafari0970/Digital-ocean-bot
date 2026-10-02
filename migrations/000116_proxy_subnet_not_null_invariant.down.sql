ALTER TABLE account_network_identities
  DROP CONSTRAINT IF EXISTS account_network_identities_subnet_canonical;

ALTER TABLE account_network_identities
  ADD CONSTRAINT account_network_identities_subnet_canonical
  CHECK (
    exit_ip IS NULL
    OR subnet_key=(host(network(set_masklen(exit_ip,24)))||'/24')
  );
