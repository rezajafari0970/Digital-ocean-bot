CREATE TABLE panel_instances(
id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

account_id uuid NOT NULL
REFERENCES accounts(id)
ON DELETE CASCADE,

droplet_id uuid NOT NULL
REFERENCES droplets(id)
ON DELETE CASCADE,

driver text NOT NULL,

base_url text NOT NULL,

auth_secret_ref text NOT NULL,

version text NOT NULL DEFAULT '',

enabled boolean NOT NULL DEFAULT true,

last_seen_at timestamptz,

created_at timestamptz NOT NULL DEFAULT now(),

updated_at timestamptz NOT NULL DEFAULT now(),

UNIQUE(droplet_id,driver)
);

CREATE TABLE panel_capability_snapshots(
id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

driver text NOT NULL,

version text NOT NULL DEFAULT '',

capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,

evidence jsonb NOT NULL DEFAULT '{}'::jsonb,

observed_at timestamptz NOT NULL,

created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX
panel_capability_snapshots_panel_observed_idx
ON panel_capability_snapshots(
panel_id,
observed_at DESC
);
