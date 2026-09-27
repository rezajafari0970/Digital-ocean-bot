CREATE TABLE reality_credentials(
id uuid PRIMARY KEY
DEFAULT gen_random_uuid(),

panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

managed_key text NOT NULL,

uuid_secret_ref text NOT NULL,

private_key_secret_ref text NOT NULL,

public_key text NOT NULL,

short_id text NOT NULL,

created_at timestamptz NOT NULL
DEFAULT now(),

updated_at timestamptz NOT NULL
DEFAULT now(),

UNIQUE(
panel_id,
managed_key
)
);

CREATE INDEX
reality_credentials_panel_idx
ON reality_credentials(
panel_id
);
