CREATE TABLE traffic_baselines (
 client_id UUID PRIMARY KEY REFERENCES xui_clients(id) ON DELETE CASCADE,
 ewma_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
 variance DOUBLE PRECISION NOT NULL DEFAULT 0,
 samples INTEGER NOT NULL DEFAULT 0,
 consecutive_anomalies INTEGER NOT NULL DEFAULT 0,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE traffic_events (
 id BIGSERIAL PRIMARY KEY,
 client_id UUID NOT NULL REFERENCES xui_clients(id) ON DELETE CASCADE,
 suspicious BOOLEAN NOT NULL,
 confirmed BOOLEAN NOT NULL,
 rate_bps DOUBLE PRECISION NOT NULL,
 threshold_bps DOUBLE PRECISION NOT NULL,
 reason TEXT NOT NULL,
 action TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX traffic_events_client_time_idx ON traffic_events(client_id,created_at DESC);
CREATE INDEX traffic_events_confirmed_idx ON traffic_events(confirmed,created_at DESC) WHERE confirmed=true;
