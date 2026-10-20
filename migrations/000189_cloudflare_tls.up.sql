CREATE TABLE cloudflare_accounts (
 id uuid PRIMARY KEY, name text NOT NULL, enabled boolean NOT NULL DEFAULT true,
 revision bigint NOT NULL DEFAULT 1, manage_ssl boolean NOT NULL DEFAULT false,
 error_code text NOT NULL DEFAULT '', next_attempt_at timestamptz NOT NULL DEFAULT now(),
 refreshed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE cloudflare_secrets (
 account_id uuid PRIMARY KEY REFERENCES cloudflare_accounts(id) ON DELETE CASCADE,
 ciphertext bytea NOT NULL, nonce bytea NOT NULL, key_version integer NOT NULL CHECK(key_version>0)
);
CREATE TABLE cloudflare_zones (
 account_id uuid NOT NULL REFERENCES cloudflare_accounts(id), id text NOT NULL,
 name text NOT NULL, status text NOT NULL, enabled boolean NOT NULL DEFAULT false,
 present boolean NOT NULL DEFAULT true, ssl_strict boolean NOT NULL DEFAULT false,
 checked_at timestamptz, next_attempt_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(account_id,id), UNIQUE(account_id,name)
);
CREATE TABLE cloudflare_certificates (
 account_id uuid NOT NULL REFERENCES cloudflare_accounts(id), panel_id uuid NOT NULL,
 generation uuid NOT NULL, names text[] NOT NULL, csr text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'PREPARED' CHECK(state IN('PREPARED','SENT','ISSUED','INSTALLED')),
 provider_id text NOT NULL DEFAULT '', pem text NOT NULL DEFAULT '', not_after timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(account_id,panel_id)
);
CREATE TABLE cloudflare_schedules (
 profile_id uuid NOT NULL REFERENCES config_driver_profiles(id), panel_id uuid NOT NULL REFERENCES panel_instances(id) ON DELETE CASCADE,
 next_batch_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(profile_id,panel_id)
);
-- Retain DNS cleanup ownership after an ephemeral panel has been purged.
CREATE TABLE cloudflare_records (
 id uuid PRIMARY KEY, account_id uuid NOT NULL REFERENCES cloudflare_accounts(id),
 zone_id text NOT NULL, profile_id uuid NOT NULL REFERENCES config_driver_profiles(id),
 panel_id uuid NOT NULL, hostname text NOT NULL UNIQUE,
 origin_ip inet NOT NULL, provider_id text NOT NULL DEFAULT '',
 state text NOT NULL DEFAULT 'PREPARED' CHECK(state IN('PREPARED','SENT','DNS_READY','DELETING','DELETED','CONFLICT')),
 lifetime_seconds integer NOT NULL DEFAULT 2700 CHECK(lifetime_seconds BETWEEN 900 AND 86400), activated_at timestamptz,
 expires_at timestamptz NOT NULL, next_attempt_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(), observed_at timestamptz,
 probe_next_at timestamptz NOT NULL DEFAULT now(), verified_at timestamptz, verified_revision bigint, verified_classes text[] NOT NULL DEFAULT '{}',
 error_code text NOT NULL DEFAULT '', failures integer NOT NULL DEFAULT 0,
 FOREIGN KEY(account_id,zone_id) REFERENCES cloudflare_zones(account_id,id)
);
CREATE INDEX cloudflare_records_due ON cloudflare_records(next_attempt_at) WHERE state<>'DELETED';
CREATE INDEX cloudflare_records_scope ON cloudflare_records(profile_id,panel_id,expires_at) WHERE state<>'DELETED';
CREATE TABLE cloudflare_api_budget(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),next_at timestamptz NOT NULL DEFAULT now());
INSERT INTO cloudflare_api_budget(singleton) VALUES(true);
ALTER TABLE output_config_snapshots ADD COLUMN cloudflare_record_id uuid REFERENCES cloudflare_records(id);

CREATE TABLE cloudflare_ip_ranges(network cidr PRIMARY KEY,fetched_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE cloudflare_ip_refresh(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),attempted_at timestamptz,updated_at timestamptz);
INSERT INTO cloudflare_ip_refresh(singleton) VALUES(true);
CREATE TABLE cloudflare_ip_measurements(
 id uuid PRIMARY KEY, account_id uuid NOT NULL REFERENCES cloudflare_accounts(id),
 record_id uuid NOT NULL REFERENCES cloudflare_records(id), operator text NOT NULL, ip inet NOT NULL,
 latency_ms integer NOT NULL CHECK(latency_ms BETWEEN 1 AND 60000), measured_at timestamptz NOT NULL,
 source text NOT NULL CHECK(source='operator_probe'), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cloudflare_ip_measurements_lookup ON cloudflare_ip_measurements(account_id,operator,measured_at DESC);
ALTER TABLE cloudflare_records ADD COLUMN edge_address text NOT NULL DEFAULT '';
ALTER TABLE cloudflare_records ADD COLUMN operator text NOT NULL DEFAULT '';

ALTER TABLE cloudflare_certificates ADD COLUMN negative_observations integer NOT NULL DEFAULT 0;
ALTER TABLE cloudflare_certificates ADD COLUMN last_observed_at timestamptz;
CREATE TABLE cloudflare_certificate_history (
 generation uuid PRIMARY KEY, account_id uuid NOT NULL REFERENCES cloudflare_accounts(id), panel_id uuid NOT NULL,
 names text[] NOT NULL, csr text NOT NULL, state text NOT NULL, provider_id text NOT NULL,
 pem text NOT NULL, not_after timestamptz, archived_at timestamptz NOT NULL DEFAULT now()
);
