CREATE TABLE account_security_profiles (
 account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
 profile_version BIGINT NOT NULL DEFAULT 1,
 tls_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
 http_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
 browser_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
 browser_namespace TEXT NOT NULL UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX account_security_profiles_version_idx ON account_security_profiles(profile_version);
