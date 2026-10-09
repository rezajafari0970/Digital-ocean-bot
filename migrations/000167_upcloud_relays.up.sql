ALTER TABLE panel_routing_state ADD COLUMN relay_mode boolean NOT NULL DEFAULT false;
ALTER TABLE panel_routing_state ADD COLUMN category_digest text NOT NULL DEFAULT '';
CREATE TABLE panel_relay_endpoints (
 panel_id uuid PRIMARY KEY REFERENCES panel_instances(id) ON DELETE CASCADE,
 account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 host text NOT NULL,
 port integer NOT NULL CHECK(port IN(80,443,8080)),
 secret_ref text NOT NULL,
 transport_hash text NOT NULL,
 allowed_sources text[] NOT NULL DEFAULT '{}',
 enabled boolean NOT NULL DEFAULT true,
 state text NOT NULL CHECK(state IN('PREPARING','APPLIED','DISABLED')),
 plan_hash text NOT NULL DEFAULT '',
 verified_at timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT,UPDATE,DELETE ON panel_relay_endpoints TO digitaloceanbot;
CREATE TABLE panel_relay_assignments (
 receiver_panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 donor_panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 donor_droplet_id uuid NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 donor_account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 secret_ref text NOT NULL,
 transport_hash text NOT NULL,
 donor_plan_hash text NOT NULL,
 receiver_plan_hash text NOT NULL,
 outbound_tag text NOT NULL,
 selected boolean NOT NULL DEFAULT true,
 applied boolean NOT NULL DEFAULT false,
 alive boolean NOT NULL DEFAULT false,
 failed_since timestamptz,
 failures integer NOT NULL DEFAULT 0 CHECK(failures>=0),
 last_try bigint NOT NULL DEFAULT 0,
 last_success_at timestamptz,
 observed_at timestamptz,
 cooldown_until timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(receiver_panel_id,donor_panel_id),
 CHECK(receiver_panel_id<>donor_panel_id)
);
CREATE UNIQUE INDEX panel_relay_distinct_servers ON panel_relay_assignments(receiver_panel_id,donor_droplet_id) WHERE selected;
CREATE INDEX panel_relay_donor_dependents ON panel_relay_assignments(donor_panel_id) WHERE selected;
GRANT SELECT,INSERT,UPDATE,DELETE ON panel_relay_assignments TO digitaloceanbot;
