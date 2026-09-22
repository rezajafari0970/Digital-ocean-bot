DROP INDEX IF EXISTS proxies_status_idx;
ALTER TABLE proxies DROP COLUMN IF EXISTS expected_exit_ip;
ALTER TABLE proxies DROP COLUMN IF EXISTS health_error;
ALTER TABLE proxies DROP COLUMN IF EXISTS latency_ms;
ALTER TABLE proxies DROP COLUMN IF EXISTS consecutive_successes;
