ALTER TABLE proxy_runtime_state
    DROP COLUMN IF EXISTS half_open_probe_lease_until,
    DROP COLUMN IF EXISTS half_open_probe_in_flight;
