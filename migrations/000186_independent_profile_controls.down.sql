ALTER TABLE config_driver_listeners DROP COLUMN needs_reconcile;
DROP INDEX config_driver_listener_active_leases;
ALTER TABLE config_driver_listeners DROP COLUMN lease_until,DROP COLUMN claim_token;
DROP TABLE config_driver_sni_selections;
DROP TABLE config_driver_class_proofs;
ALTER TABLE config_driver_clients DROP COLUMN publish,DROP COLUMN expires_at;
ALTER TABLE config_driver_profiles DROP COLUMN last_scheduled_at;
ALTER TABLE config_driver_listeners DROP COLUMN stage_started_at;
ALTER TABLE config_driver_profiles DROP COLUMN class_settings;
