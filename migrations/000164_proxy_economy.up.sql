CREATE TABLE proxy_economy_policy (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false,
    canary_account_id uuid,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO proxy_economy_policy(singleton) VALUES(true);

-- Application-observed socket bytes, not provider billing. No credentials,
-- URLs, destination paths or client payloads are stored here.
CREATE TABLE proxy_traffic_hourly (
    hour timestamptz NOT NULL,
    process_id text NOT NULL,
    role text NOT NULL,
    owner_id text NOT NULL,
    proxy_id text NOT NULL,
    purpose text NOT NULL,
    tx_bytes bigint NOT NULL CHECK (tx_bytes >= 0),
    rx_bytes bigint NOT NULL CHECK (rx_bytes >= 0),
    connections bigint NOT NULL CHECK (connections >= 0),
    requests bigint NOT NULL CHECK (requests >= 0),
    errors bigint NOT NULL CHECK (errors >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(hour, process_id, role, owner_id, proxy_id, purpose)
);
CREATE INDEX proxy_traffic_hourly_updated ON proxy_traffic_hourly(updated_at);

ALTER TABLE provider_catalog_cache ADD COLUMN scope_revision text NOT NULL DEFAULT '';
