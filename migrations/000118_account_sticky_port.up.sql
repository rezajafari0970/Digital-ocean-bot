ALTER TABLE account_network_identities
  ADD COLUMN IF NOT EXISTS sticky_port integer;

DO $$
DECLARE n integer;
BEGIN
  SELECT count(*) INTO n
  FROM account_network_identities i
  JOIN network_profiles np ON np.account_id=i.account_id
  JOIN proxies p ON p.id=np.proxy_id
  WHERE COALESCE(p.adapter,'generic')='suffix-session';

  IF n > 10001 THEN
    RAISE EXCEPTION 'too many sticky accounts for ports 10000-20000';
  END IF;
END $$;

WITH ranked AS (
  SELECT i.account_id,
         9999 + row_number() OVER (ORDER BY i.account_id) AS port
  FROM account_network_identities i
  JOIN network_profiles np ON np.account_id=i.account_id
  JOIN proxies p ON p.id=np.proxy_id
  WHERE COALESCE(p.adapter,'generic')='suffix-session'
)
UPDATE account_network_identities i
SET sticky_port=r.port
FROM ranked r
WHERE i.account_id=r.account_id
  AND i.sticky_port IS NULL;

ALTER TABLE account_network_identities
  DROP CONSTRAINT IF EXISTS account_network_identities_sticky_port_range;
ALTER TABLE account_network_identities
  ADD CONSTRAINT account_network_identities_sticky_port_range
  CHECK (sticky_port IS NULL OR sticky_port BETWEEN 10000 AND 20000);

CREATE UNIQUE INDEX IF NOT EXISTS account_network_identity_sticky_port_unique
  ON account_network_identities(sticky_port)
  WHERE sticky_port IS NOT NULL;