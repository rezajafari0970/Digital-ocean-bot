ALTER TABLE reality_target_selections
ADD COLUMN server_names jsonb NOT NULL DEFAULT '[]'::jsonb,
ADD COLUMN tls_version text NOT NULL DEFAULT '',
ADD COLUMN alpn text NOT NULL DEFAULT '',
ADD COLUMN curve_id text NOT NULL DEFAULT '',
ADD COLUMN cert_valid boolean NOT NULL DEFAULT false,
ADD COLUMN cert_chain_valid boolean NOT NULL DEFAULT false,
ADD COLUMN latency_ms bigint NOT NULL DEFAULT 0,
ADD COLUMN scanned_at timestamptz;
