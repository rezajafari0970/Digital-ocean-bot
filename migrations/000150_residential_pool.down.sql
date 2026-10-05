ALTER TABLE panel_routing_state DROP COLUMN pool_enabled;
ALTER TABLE residential_proxies DROP COLUMN check_count, DROP COLUMN latency_ewma_ms, DROP COLUMN success_ewma;
DROP TABLE residential_imports;
DROP INDEX residential_name_unique;
