CREATE TABLE panel_inbound_policies(
id uuid PRIMARY KEY
DEFAULT gen_random_uuid(),

panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

policy_key text NOT NULL,

revision bigint NOT NULL
DEFAULT 1
CHECK(revision >= 1),

enabled boolean NOT NULL
DEFAULT true,

desired_count integer NOT NULL
DEFAULT 0
CHECK(desired_count >= 0),

protocol text NOT NULL,

transport text NOT NULL
DEFAULT '',

security text NOT NULL
DEFAULT '',

listen text NOT NULL
DEFAULT '',

preferred_ports jsonb NOT NULL
DEFAULT '[]'::jsonb,

dynamic_port_start integer NOT NULL
CHECK(
dynamic_port_start >= 1
AND dynamic_port_start <= 65535
),

dynamic_port_end integer NOT NULL
CHECK(
dynamic_port_end >= 1
AND dynamic_port_end <= 65535
),

reserved_ports jsonb NOT NULL
DEFAULT '[]'::jsonb,

clients_per_inbound integer NOT NULL
DEFAULT 1
CHECK(clients_per_inbound >= 0),

allow_delete boolean NOT NULL
DEFAULT false,

created_at timestamptz NOT NULL
DEFAULT now(),

updated_at timestamptz NOT NULL
DEFAULT now(),

UNIQUE(
panel_id,
policy_key
),

CHECK(
dynamic_port_start <=
dynamic_port_end
)
);

CREATE INDEX
panel_inbound_policies_panel_enabled_idx
ON panel_inbound_policies(
panel_id,
enabled
);
