CREATE TABLE panel_inventory_syncs(
id uuid PRIMARY KEY
DEFAULT gen_random_uuid(),

panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

state text NOT NULL
CHECK(
state IN (
'RUNNING',
'COMPLETED',
'FAILED'
)
),

observed_count integer NOT NULL
DEFAULT 0
CHECK(observed_count >= 0),

changed_count integer NOT NULL
DEFAULT 0
CHECK(changed_count >= 0),

missing_count integer NOT NULL
DEFAULT 0
CHECK(missing_count >= 0),

started_at timestamptz NOT NULL
DEFAULT now(),

finished_at timestamptz,

error_code text NOT NULL
DEFAULT ''
);

CREATE TABLE panel_inbound_inventory(
panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

remote_id bigint NOT NULL,

remark text NOT NULL DEFAULT '',

protocol text NOT NULL DEFAULT '',

port integer NOT NULL DEFAULT 0
CHECK(
port >= 0
AND port <= 65535
),

listen text NOT NULL DEFAULT '',

enabled boolean NOT NULL DEFAULT false,

transport text NOT NULL DEFAULT '',

security text NOT NULL DEFAULT '',

client_count integer NOT NULL DEFAULT 0
CHECK(client_count >= 0),

upload_bytes bigint NOT NULL DEFAULT 0
CHECK(upload_bytes >= 0),

download_bytes bigint NOT NULL DEFAULT 0
CHECK(download_bytes >= 0),

total_bytes bigint NOT NULL DEFAULT 0
CHECK(total_bytes >= 0),

raw_hash text NOT NULL,

present boolean NOT NULL DEFAULT true,

first_seen_at timestamptz NOT NULL,

last_seen_at timestamptz NOT NULL,

missing_since timestamptz,

PRIMARY KEY(
panel_id,
remote_id
)
);

CREATE INDEX
panel_inbound_inventory_panel_present_idx
ON panel_inbound_inventory(
panel_id,
present
);

CREATE INDEX
panel_inbound_inventory_protocol_idx
ON panel_inbound_inventory(
protocol
);

CREATE INDEX
panel_inbound_inventory_transport_security_idx
ON panel_inbound_inventory(
transport,
security
);
