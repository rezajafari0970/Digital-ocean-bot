-- Retire client transport overlays. Migration155 remains immutable history.
DROP TABLE IF EXISTS output_client_transport_operations;
DROP TABLE IF EXISTS output_client_transport_profiles;
