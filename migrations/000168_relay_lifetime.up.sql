ALTER TABLE panel_relay_endpoints ADD COLUMN valid_until timestamptz;
COMMENT ON COLUMN panel_relay_endpoints.valid_until IS 'Earlier of managed SOCKS credential expiry and donor server expiry; unknown is not eligible.';
