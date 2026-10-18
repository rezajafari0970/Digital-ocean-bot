-- Refuse rollback once independent profiles exist: silently discarding ownership is unsafe.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM config_driver_profiles) THEN RAISE EXCEPTION 'Config profiles require an explicit verified rollback before dropping ownership'; END IF;
END $$;
DROP TRIGGER trojan_driver_port_guard ON trojan_tcp_policy;
DROP TRIGGER global_driver_port_guard ON global_config_policies;
DROP TRIGGER reality_driver_port_guard ON reality_config_profiles;
DROP TRIGGER config_driver_port_guard ON config_driver_profiles;
DROP FUNCTION guard_config_driver_ports();
ALTER TABLE output_config_snapshots DROP COLUMN config_profile_id, DROP COLUMN config_profile_revision;
DROP TABLE config_driver_events,config_driver_clients,config_driver_listeners,config_driver_profiles;
