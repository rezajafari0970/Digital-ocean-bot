DROP TRIGGER IF EXISTS global_reality_trojan_port_guard ON global_config_policies;
DROP TRIGGER IF EXISTS reality_trojan_port_guard ON reality_config_profiles;
DROP TRIGGER IF EXISTS trojan_tcp_port_guard ON trojan_tcp_policy;
DROP FUNCTION IF EXISTS guard_trojan_tcp_ports();
DROP TABLE IF EXISTS trojan_tcp_clients;
DROP TABLE IF EXISTS trojan_tcp_listeners;
DROP TABLE IF EXISTS trojan_tcp_policy;
