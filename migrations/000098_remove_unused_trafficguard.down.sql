CREATE TABLE IF NOT EXISTS traffic_policies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
 min_samples integer NOT NULL DEFAULT 20, alpha double precision NOT NULL DEFAULT 0.2, sigma_multiplier double precision NOT NULL DEFAULT 4,
 min_bps bigint NOT NULL DEFAULT 0, hard_bps bigint NOT NULL DEFAULT 0, confirmations integer NOT NULL DEFAULT 3,
 action text NOT NULL DEFAULT 'LOG' CHECK(action IN ('LOG','ALERT','DISABLE_CLIENT')), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS xui_traffic_samples (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), client_id uuid NOT NULL REFERENCES xui_clients(id) ON DELETE CASCADE,
 up_bytes bigint NOT NULL, down_bytes bigint NOT NULL, sampled_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS xui_traffic_samples_client_time_idx ON xui_traffic_samples(client_id,sampled_at DESC);
CREATE TABLE IF NOT EXISTS traffic_baselines (
 client_id uuid PRIMARY KEY REFERENCES xui_clients(id) ON DELETE CASCADE, ewma_bps double precision NOT NULL DEFAULT 0,
 variance double precision NOT NULL DEFAULT 0, samples integer NOT NULL DEFAULT 0, consecutive_anomalies integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS traffic_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), client_id uuid NOT NULL REFERENCES xui_clients(id) ON DELETE CASCADE,
 suspicious boolean NOT NULL DEFAULT false, confirmed boolean NOT NULL DEFAULT false, rate_bps double precision NOT NULL DEFAULT 0,
 threshold_bps double precision NOT NULL DEFAULT 0, reason text NOT NULL DEFAULT '', action text NOT NULL DEFAULT 'LOG',
 created_at timestamptz NOT NULL DEFAULT now()
);
