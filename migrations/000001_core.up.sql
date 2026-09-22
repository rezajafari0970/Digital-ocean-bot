CREATE TABLE proxies (
 id UUID PRIMARY KEY,
 name TEXT NOT NULL UNIQUE,
 type TEXT NOT NULL CHECK (type IN ('http','https','socks5')),
 host TEXT NOT NULL,
 port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
 username TEXT,
 secret_ref TEXT,
 status TEXT NOT NULL DEFAULT 'unknown',
 exit_ip INET,
 country TEXT,
 asn TEXT,
 failure_count INTEGER NOT NULL DEFAULT 0,
 last_checked_at TIMESTAMPTZ,
 last_success_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
 id UUID PRIMARY KEY,
 provider TEXT NOT NULL,
 name TEXT NOT NULL,
 external_id TEXT,
 email TEXT,
 secret_ref TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE network_profiles (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
 mode TEXT NOT NULL CHECK (mode IN ('direct','proxy_required')),
 proxy_id UUID REFERENCES proxies(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK (mode <> 'proxy_required' OR proxy_id IS NOT NULL)
);
