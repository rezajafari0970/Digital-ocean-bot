DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM panel_protocol_health WHERE driver='managed_tls_xudp') THEN
  RAISE EXCEPTION 'Retained managed-relay evidence prevents protocol-driver downgrade';
 END IF;
END $$;
ALTER TABLE panel_protocol_health DROP CONSTRAINT panel_protocol_health_result_check;
ALTER TABLE panel_protocol_health DROP COLUMN driver;
ALTER TABLE panel_protocol_health ADD CONSTRAINT panel_protocol_health_check
 CHECK ((code='ok')=(dns_ok AND quic_ok));
