CREATE TABLE lifecycle_deficit_claims (
 droplet_id uuid PRIMARY KEY REFERENCES droplets(id) ON DELETE CASCADE,
 claim_id uuid NOT NULL DEFAULT gen_random_uuid(),
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 provider_resource_id text NOT NULL,
 started_at timestamptz,
 recovered_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX lifecycle_deficit_claims_account_idx ON lifecycle_deficit_claims(account_id);
