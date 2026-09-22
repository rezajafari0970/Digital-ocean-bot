CREATE TABLE deployment_profiles (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 version INTEGER NOT NULL DEFAULT 1,
 config JSONB NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(account_id,name,version)
);

ALTER TABLE deployments ADD COLUMN profile_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE deployments ADD CONSTRAINT deployments_profile_fk FOREIGN KEY(profile_id) REFERENCES deployment_profiles(id);
CREATE INDEX deployment_profiles_account_enabled_idx ON deployment_profiles(account_id,enabled);
