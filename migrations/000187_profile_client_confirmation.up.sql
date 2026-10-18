-- Ownership reservations are not proof of native creation.
ALTER TABLE config_driver_clients ADD COLUMN confirmed boolean NOT NULL DEFAULT true;
UPDATE config_driver_clients c SET confirmed=false
WHERE NOT EXISTS(SELECT 1 FROM panel_client_routes r WHERE r.panel_id=c.panel_id AND r.client_id=c.client_id AND r.email=c.email);
