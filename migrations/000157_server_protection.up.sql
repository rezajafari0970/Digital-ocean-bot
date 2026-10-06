CREATE TABLE server_protection_control (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT false,
 scope text NOT NULL DEFAULT 'fleet' CHECK(scope IN ('fleet','selected')),
 panel_ids uuid[] NOT NULL DEFAULT '{}',
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO server_protection_control(singleton) VALUES(true);
CREATE TABLE server_protection_nodes (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 desired_revision bigint NOT NULL,
 control_revision bigint NOT NULL,
 desired_enabled boolean NOT NULL,
 applied_revision bigint NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'PENDING',
 status jsonb NOT NULL DEFAULT '{}',
 last_error text NOT NULL DEFAULT '',
 checked_at timestamptz,
 next_check_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX server_protection_nodes_due ON server_protection_nodes(next_check_at);
CREATE TABLE server_protection_requests (
 request_id uuid PRIMARY KEY,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
