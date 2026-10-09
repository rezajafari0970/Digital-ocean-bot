DROP TABLE IF EXISTS panel_relay_assignments;
DROP TABLE IF EXISTS panel_relay_endpoints;
ALTER TABLE panel_routing_state DROP COLUMN IF EXISTS relay_mode;
ALTER TABLE panel_routing_state DROP COLUMN IF EXISTS category_digest;
