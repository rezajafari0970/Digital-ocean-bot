CREATE TABLE reality_probe_nodes(
id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
name text NOT NULL UNIQUE,
host inet NOT NULL UNIQUE,
port integer NOT NULL DEFAULT 22 CHECK(port BETWEEN 1 AND 65535),
ssh_user text NOT NULL,
account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
ssh_key_secret_ref text NOT NULL,
enabled boolean NOT NULL DEFAULT true,
status text NOT NULL DEFAULT 'UNKNOWN' CHECK(status IN ('UNKNOWN','HEALTHY','DEGRADED','DOWN')),
consecutive_successes integer NOT NULL DEFAULT 0 CHECK(consecutive_successes>=0),
consecutive_failures integer NOT NULL DEFAULT 0 CHECK(consecutive_failures>=0),
last_latency_ms bigint NOT NULL DEFAULT 0 CHECK(last_latency_ms>=0),
last_exit_ip inet,
last_checked_at timestamptz,
last_success_at timestamptz,
created_at timestamptz NOT NULL DEFAULT now(),
updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reality_probe_nodes_select_idx ON reality_probe_nodes(enabled,status,last_checked_at);
