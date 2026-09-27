CREATE TABLE reality_target_observations(
id uuid PRIMARY KEY
DEFAULT gen_random_uuid(),

panel_id uuid NOT NULL
REFERENCES panel_instances(id)
ON DELETE CASCADE,

target text NOT NULL,

server_name text NOT NULL,

port integer NOT NULL
CHECK(
port >= 1
AND port <= 65535
),

reachable boolean NOT NULL,

tls_version text NOT NULL
DEFAULT '',

cert_valid boolean NOT NULL,

http2 boolean NOT NULL,

samples integer NOT NULL
CHECK(samples >= 0),

successes integer NOT NULL
CHECK(
successes >= 0
AND successes <= samples
),

median_latency_ms bigint NOT NULL
DEFAULT 0
CHECK(median_latency_ms >= 0),

success_ratio double precision NOT NULL
DEFAULT 0
CHECK(
success_ratio >= 0
AND success_ratio <= 1
),

eligible boolean NOT NULL,

score double precision NOT NULL
DEFAULT 0,

reason text NOT NULL
DEFAULT '',

observed_at timestamptz NOT NULL,

created_at timestamptz NOT NULL
DEFAULT now()
);

CREATE INDEX
reality_target_observations_panel_time_idx
ON reality_target_observations(
panel_id,
observed_at DESC
);

CREATE INDEX
reality_target_observations_candidate_idx
ON reality_target_observations(
panel_id,
target,
server_name,
port,
observed_at DESC
);

CREATE TABLE reality_target_selections(
panel_id uuid PRIMARY KEY
REFERENCES panel_instances(id)
ON DELETE CASCADE,

target text NOT NULL,

server_name text NOT NULL,

port integer NOT NULL
CHECK(
port >= 1
AND port <= 65535
),

score double precision NOT NULL,

selected_at timestamptz NOT NULL,

updated_at timestamptz NOT NULL
DEFAULT now()
);
