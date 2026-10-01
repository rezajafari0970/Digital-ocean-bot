ALTER TABLE proxy_runtime_state
    ADD COLUMN half_open_probe_in_flight BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN half_open_probe_lease_until TIMESTAMPTZ;
