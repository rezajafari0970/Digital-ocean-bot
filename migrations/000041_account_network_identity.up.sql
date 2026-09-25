CREATE TABLE IF NOT EXISTS account_network_identities (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 timezone TEXT NOT NULL DEFAULT 'UTC',
 locale TEXT NOT NULL DEFAULT 'en-US',
 exit_ip INET,
 subnet_key TEXT,
 asn TEXT,
 country TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT,UPDATE,DELETE ON TABLE account_network_identities TO digitaloceanbot;
