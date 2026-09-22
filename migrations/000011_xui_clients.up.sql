CREATE TABLE xui_clients (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 droplet_id UUID NOT NULL REFERENCES droplets(id) ON DELETE CASCADE,
 inbound_id INTEGER NOT NULL,
 email TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(droplet_id,inbound_id,email)
);

CREATE TABLE xui_traffic_samples (
 id BIGSERIAL PRIMARY KEY,
 client_id UUID NOT NULL REFERENCES xui_clients(id) ON DELETE CASCADE,
 up_bytes BIGINT NOT NULL,
 down_bytes BIGINT NOT NULL,
 sampled_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX xui_clients_account_enabled_idx ON xui_clients(account_id,enabled);
CREATE INDEX xui_traffic_samples_client_time_idx ON xui_traffic_samples(client_id,sampled_at DESC);
